// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package protomux splits a stream of incoming connections into HTTP/2 (gRPC) and HTTP/1.1 (web) connections.
//
// The discovery service serves gRPC traffic and a small set of static HTML assets on the same port.
// Running both on top of Go's HTTP/2 server (via grpc.Server.ServeHTTP) costs three goroutines and
// a set of per-connection buffers for every gRPC stream, which is expensive with hundreds of
// thousands of long-lived Watch calls. Instead, connections are classified once, right after they
// are accepted, and handed over to either the native gRPC server or the HTTP server.
//
// Classification is done via TLS ALPN: clients which don't offer "http/1.1" are assumed to be gRPC
// clients (all gRPC implementations offer "h2" only), everything else (browsers, curl) is served
// over HTTP/1.1. For plaintext listeners the HTTP/2 client connection preface is used instead.
package protomux

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/http2"
)

// handshakeTimeout limits the time a client may take to reveal which protocol it speaks.
const handshakeTimeout = 10 * time.Second

// Accept retry backoff on temporary errors (e.g. running out of file descriptors), same as
// net/http and grpc-go use.
const (
	acceptBackoffMin = 5 * time.Millisecond
	acceptBackoffMax = time.Second
)

// Mux accepts connections from a listener and splits them between two virtual listeners.
type Mux struct {
	inner  net.Listener
	logger *zap.Logger

	// tlsConfig is nil for plaintext listeners.
	tlsConfig *tls.Config

	metrics *Metrics

	h2   *listener
	http *listener
}

// New creates a new Mux on top of the listener.
//
// If tlsConfig is not nil, the Mux terminates TLS, and the connections handed over to the
// listeners are already decrypted.
func New(inner net.Listener, tlsConfig *tls.Config, metrics *Metrics, logger *zap.Logger) *Mux {
	mux := &Mux{
		inner:   inner,
		logger:  logger,
		metrics: metrics,
		h2:      newListener(inner.Addr()),
		http:    newListener(inner.Addr()),
	}

	if tlsConfig != nil {
		mux.tlsConfig = alpnConfig(tlsConfig)
	}

	return mux
}

// H2Listener returns the listener yielding HTTP/2 (gRPC) connections.
func (mux *Mux) H2Listener() net.Listener {
	return mux.h2
}

// HTTPListener returns the listener yielding HTTP/1.1 connections.
func (mux *Mux) HTTPListener() net.Listener {
	return mux.http
}

// Run the accept loop until the context is canceled or the underlying listener fails.
//
// Both virtual listeners are closed on return, which shuts down the servers running on top of them.
func (mux *Mux) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	// deferred in reverse order: close the virtual listeners first, so that in-flight
	// dispatch goroutines stop waiting for a server to pick the connection up
	defer wg.Wait()

	defer mux.h2.Close()   //nolint:errcheck
	defer mux.http.Close() //nolint:errcheck

	// unblock Accept on shutdown
	stop := context.AfterFunc(ctx, func() { mux.inner.Close() }) //nolint:errcheck
	defer stop()

	var backoff time.Duration

	for {
		conn, err := mux.inner.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}

			// a temporary error (e.g. EMFILE under a connection storm) must not bring the
			// whole service down: back off and retry, like the servers this loop replaces do
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() { //nolint:staticcheck // no better signal is available
				backoff = min(max(backoff*2, acceptBackoffMin), acceptBackoffMax)

				mux.logger.Warn("accept failed, retrying", zap.Error(err), zap.Duration("backoff", backoff))

				select {
				case <-time.After(backoff):
					continue
				case <-ctx.Done():
					return nil
				}
			}

			return err
		}

		backoff = 0

		// classification blocks on the client (TLS handshake or first bytes), so it can't
		// happen in the accept loop
		wg.Go(func() {
			mux.dispatch(ctx, conn)
		})
	}
}

func (mux *Mux) dispatch(ctx context.Context, conn net.Conn) {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		mux.metrics.connRejected()

		conn.Close() //nolint:errcheck

		return
	}

	target, conn, err := mux.classify(ctx, conn)
	if err != nil {
		mux.logger.Debug("failed to classify connection", zap.Error(err), zap.Stringer("remote_addr", conn.RemoteAddr()))

		mux.metrics.connRejected()

		conn.Close() //nolint:errcheck

		return
	}

	if err = conn.SetDeadline(time.Time{}); err != nil {
		mux.metrics.connRejected()

		conn.Close() //nolint:errcheck

		return
	}

	// The HTTP server counts its own connections through http.Server.ConnState, but the gRPC
	// server has no equivalent hook: the only one it offers is grpc.StatsHandler, and
	// registering one makes gRPC build per-RPC and per-message stats objects it otherwise
	// skips entirely. Counting the connection here instead costs nothing per request.
	//
	// The connection is counted before it is handed over, because the server may close it at
	// any moment afterwards, and the close must never be recorded before the open.
	if target == mux.h2 {
		mux.metrics.connOpened(ProtocolGRPC)

		// push closes the connection when the server is gone, and trackedConn records that
		target.push(&trackedConn{Conn: conn, onClose: func() { mux.metrics.connClosed(ProtocolGRPC) }})

		return
	}

	if !target.push(conn) {
		// the server is gone (shutdown in progress)
		mux.metrics.connRejected()
	}
}

func (mux *Mux) classify(ctx context.Context, conn net.Conn) (*listener, net.Conn, error) {
	if mux.tlsConfig != nil {
		tlsConn := tls.Server(conn, mux.tlsConfig)

		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, conn, err
		}

		if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
			return mux.h2, tlsConn, nil
		}

		return mux.http, tlsConn, nil
	}

	// plaintext: peek at the HTTP/2 client connection preface
	//
	// unlike tls.Conn.HandshakeContext, a raw Read is not context-aware, so a client which
	// connects and stays silent would otherwise hold up shutdown for the whole handshakeTimeout
	stop := context.AfterFunc(ctx, func() { conn.Close() }) //nolint:errcheck
	defer stop()

	var (
		buf [len(http2.ClientPreface)]byte
		n   int
	)

	for n < len(http2.ClientPreface) {
		read, err := conn.Read(buf[n:])
		n += read

		if string(buf[:n]) != http2.ClientPreface[:n] {
			return mux.http, &prefixConn{Conn: conn, prefix: buf[:n]}, nil
		}

		if err != nil {
			return nil, conn, err
		}
	}

	return mux.h2, &prefixConn{Conn: conn, prefix: buf[:n]}, nil
}

// alpnConfig makes the TLS handshake pick the protocol the connection will be routed by.
//
// gRPC clients advertise "h2" only, while browsers and other HTTP clients also advertise
// "http/1.1"; the latter get HTTP/1.1, which is plenty for the static assets served here.
func alpnConfig(config *tls.Config) *tls.Config {
	h2Config := config.Clone()
	h2Config.NextProtos = []string{"h2"}

	httpConfig := config.Clone()
	httpConfig.NextProtos = []string{"http/1.1"}

	config = config.Clone()
	config.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		if slices.Contains(hello.SupportedProtos, "http/1.1") {
			return httpConfig, nil
		}

		return h2Config, nil
	}

	return config
}

// trackedConn reports the connection as closed exactly once, so that the active connection
// gauge stays balanced however many times the server closes it.
type trackedConn struct {
	net.Conn

	onClose func()
	once    sync.Once
}

// Close implements net.Conn.
func (conn *trackedConn) Close() error {
	err := conn.Conn.Close()

	conn.once.Do(conn.onClose)

	return err
}

// prefixConn re-injects the bytes consumed while sniffing the protocol.
type prefixConn struct {
	net.Conn

	prefix []byte
}

func (conn *prefixConn) Read(p []byte) (int, error) {
	if len(conn.prefix) > 0 {
		n := copy(p, conn.prefix)
		conn.prefix = conn.prefix[n:]

		return n, nil
	}

	return conn.Conn.Read(p)
}

// listener is a virtual listener fed by the Mux accept loop.
type listener struct {
	addr  net.Addr
	conns chan net.Conn
	done  chan struct{}

	closeOnce sync.Once
}

func newListener(addr net.Addr) *listener {
	return &listener{
		addr:  addr,
		conns: make(chan net.Conn),
		done:  make(chan struct{}),
	}
}

// push hands the connection over to the server running on this listener.
//
// Returns false if the listener is closed, in which case the connection is closed as well.
func (l *listener) push(conn net.Conn) bool {
	select {
	case l.conns <- conn:
		return true
	case <-l.done:
		conn.Close() //nolint:errcheck

		return false
	}
}

// Accept implements net.Listener.
func (l *listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

// Close implements net.Listener.
func (l *listener) Close() error {
	l.closeOnce.Do(func() { close(l.done) })

	return nil
}

// Addr implements net.Listener.
func (l *listener) Addr() net.Addr {
	return l.addr
}

// Check interfaces.
var (
	_ net.Listener = (*listener)(nil)
	_ net.Conn     = (*prefixConn)(nil)
	_ net.Conn     = (*trackedConn)(nil)
)
