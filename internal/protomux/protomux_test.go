// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package protomux_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/net/http2"

	"github.com/siderolabs/discovery-service/internal/protomux"
	"github.com/siderolabs/discovery-service/internal/testcert"
)

const clientPreface = http2.ClientPreface

// setupMux starts a Mux and a reader for each of its listeners; the returned channels
// receive the first line of data read off a connection routed to that listener.
func setupMux(t *testing.T, tlsConfig *tls.Config) (addr string, h2, web <-chan string) {
	t.Helper()

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	mux := protomux.New(lis, tlsConfig, protomux.NewMetrics(), zaptest.NewLogger(t))

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- mux.Run(ctx) }()

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Error("mux did not shut down")
		}
	})

	accept := func(l net.Listener) <-chan string {
		ch := make(chan string, 8)

		go func() {
			for {
				conn, acceptErr := l.Accept()
				if acceptErr != nil {
					return
				}

				go func() {
					defer conn.Close() //nolint:errcheck

					buf := make([]byte, len(clientPreface))

					n, _ := io.ReadFull(conn, buf) //nolint:errcheck

					ch <- string(buf[:n])
				}()
			}
		}()

		return ch
	}

	return lis.Addr().String(), accept(mux.H2Listener()), accept(mux.HTTPListener())
}

func expect(t *testing.T, ch <-chan string, want string) {
	t.Helper()

	select {
	case got := <-ch:
		assert.Equal(t, want, got)
	case <-time.After(10 * time.Second):
		t.Fatal("connection was not routed to the expected listener")
	}
}

func TestTLSRouting(t *testing.T) {
	t.Parallel()

	addr, h2, web := setupMux(t, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{testcert.Generate(t)},
	})

	dial := func(nextProtos []string, payload string) {
		dialer := &tls.Dialer{Config: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
			NextProtos:         nextProtos,
		}}

		conn, err := dialer.DialContext(t.Context(), "tcp", addr)
		require.NoError(t, err)

		t.Cleanup(func() { conn.Close() }) //nolint:errcheck

		_, err = io.WriteString(conn, payload)
		require.NoError(t, err)
	}

	// gRPC clients advertise "h2" only
	dial([]string{"h2"}, clientPreface)
	expect(t, h2, clientPreface)

	// browsers advertise both, and are served over HTTP/1.1
	dial([]string{"h2", "http/1.1"}, "GET / HTTP/1.1\r\n\r\n\r\n\r\n\r\n")
	expect(t, web, "GET / HTTP/1.1\r\n\r\n\r\n\r\n\r\n")

	// clients with no ALPN at all fall back to the web listener
	dial(nil, "GET / HTTP/1.1\r\n\r\n\r\n\r\n\r\n")
	expect(t, web, "GET / HTTP/1.1\r\n\r\n\r\n\r\n\r\n")
}

func TestPlaintextRouting(t *testing.T) {
	t.Parallel()

	addr, h2, web := setupMux(t, nil)

	dial := func(payload string) {
		conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
		require.NoError(t, err)

		// the connection stays open until the test is done, so the mux never races with
		// a client-side close while sniffing the payload
		t.Cleanup(func() { conn.Close() }) //nolint:errcheck

		_, err = io.WriteString(conn, payload)
		require.NoError(t, err)
	}

	// the HTTP/2 client connection preface is consumed by the sniffer and replayed
	dial(clientPreface)
	expect(t, h2, clientPreface)

	// anything else is HTTP/1.1, and the bytes are handed over untouched
	dial("GET / HTTP/1.1\r\n\r\n\r\n\r\n\r\n")
	expect(t, web, "GET / HTTP/1.1\r\n\r\n\r\n\r\n\r\n")

	// a payload which only shares a prefix with the preface is still HTTP/1.1
	dial("PRI * HTTP/1.1\r\n\r\n\r\n\r\n\r\n")
	expect(t, web, "PRI * HTTP/1.1\r\n\r\n\r\n\r\n\r\n")
}

func TestShutdown(t *testing.T) {
	t.Parallel()

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	mux := protomux.New(lis, nil, protomux.NewMetrics(), zaptest.NewLogger(t))

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- mux.Run(ctx) }()

	cancel()

	select {
	case err = <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("mux did not shut down")
	}

	// both listeners are closed, so the servers running on top of them stop
	_, err = mux.H2Listener().Accept()
	assert.ErrorIs(t, err, net.ErrClosed)

	_, err = mux.HTTPListener().Accept()
	assert.ErrorIs(t, err, net.ErrClosed)
}

// readSignalListener wraps a listener to report when the first Read on an accepted connection
// starts, i.e. when the Mux is blocked in the protocol sniffer.
type readSignalListener struct {
	net.Listener

	reading chan struct{}
}

func (l *readSignalListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	return &readSignalConn{Conn: conn, reading: l.reading}, nil
}

type readSignalConn struct {
	net.Conn

	reading chan struct{}
	once    sync.Once
}

func (conn *readSignalConn) Read(p []byte) (int, error) {
	conn.once.Do(func() { close(conn.reading) })

	return conn.Conn.Read(p)
}

func TestShutdownWithSilentClient(t *testing.T) {
	t.Parallel()

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	reading := make(chan struct{})

	mux := protomux.New(&readSignalListener{Listener: lis, reading: reading}, nil, protomux.NewMetrics(), zaptest.NewLogger(t))

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- mux.Run(ctx) }()

	// a plaintext client which connects and never sends anything keeps the sniffer blocked
	// in Read; shutdown must not wait for the handshake timeout to kick it out
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", lis.Addr().String())
	require.NoError(t, err)

	t.Cleanup(func() { conn.Close() }) //nolint:errcheck

	select {
	case <-reading:
	case <-time.After(10 * time.Second):
		t.Fatal("connection was not picked up by the sniffer")
	}

	start := time.Now()

	cancel()

	select {
	case err = <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("mux did not shut down")
	}

	assert.Less(t, time.Since(start), 5*time.Second, "shutdown waited for the handshake timeout")

	// the mux closed the connection while shutting down
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))

	_, err = conn.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
}

func TestConnectionMetrics(t *testing.T) {
	t.Parallel()

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	metrics := protomux.NewMetrics()
	mux := protomux.New(lis, nil, metrics, zaptest.NewLogger(t))

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- mux.Run(ctx) }()

	t.Cleanup(func() {
		cancel()

		select {
		case runErr := <-done:
			assert.NoError(t, runErr)
		case <-time.After(10 * time.Second):
			t.Error("mux did not shut down")
		}
	})

	// accept the connection but hold it open, so that the open and the close can be observed
	// separately
	accepted := make(chan net.Conn, 1)

	go func() {
		for {
			conn, acceptErr := mux.H2Listener().Accept()
			if acceptErr != nil {
				return
			}

			accepted <- conn
		}
	}()

	client, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", lis.Addr().String())
	require.NoError(t, err)

	defer client.Close() //nolint:errcheck

	_, err = io.WriteString(client, clientPreface)
	require.NoError(t, err)

	var served net.Conn

	select {
	case served = <-accepted:
	case <-time.After(10 * time.Second):
		t.Fatal("connection was not routed to the gRPC listener")
	}

	assertConnMetrics(t, metrics, 1, 0, 1)

	// the server closing the connection is what balances the gauge, and closing twice (which
	// gRPC may well do) must not count twice
	require.NoError(t, served.Close())
	served.Close() //nolint:errcheck

	assertConnMetrics(t, metrics, 1, 1, 0)
}

// assertConnMetrics checks the gRPC connection counters.
func assertConnMetrics(t *testing.T, metrics *protomux.Metrics, opened, closed, active int) {
	t.Helper()

	expected := fmt.Sprintf(`
# HELP discovery_connections_active The current number of connections on the main listener by protocol.
# TYPE discovery_connections_active gauge
discovery_connections_active{protocol="grpc"} %d
discovery_connections_active{protocol="http"} 0
# HELP discovery_connections_opened_total Number of connections accepted on the main listener by protocol.
# TYPE discovery_connections_opened_total counter
discovery_connections_opened_total{protocol="grpc"} %d
discovery_connections_opened_total{protocol="http"} 0
# HELP discovery_connections_closed_total Number of connections closed on the main listener by protocol.
# TYPE discovery_connections_closed_total counter
discovery_connections_closed_total{protocol="grpc"} %d
discovery_connections_closed_total{protocol="http"} 0
`, active, opened, closed)

	require.NoError(t, promtestutil.CollectAndCompare(metrics, strings.NewReader(expected),
		"discovery_connections_opened_total", "discovery_connections_closed_total", "discovery_connections_active"))
}
