// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package service_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/siderolabs/discovery-api/api/v1alpha1/server/pb"
	_ "github.com/siderolabs/proto-codec/codec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/siderolabs/discovery-service/internal/testcert"
	"github.com/siderolabs/discovery-service/pkg/service"
)

// freePort grabs a port and releases it, so that the service can bind it.
func freePort(t *testing.T) string {
	t.Helper()

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := lis.Addr().String()

	require.NoError(t, lis.Close())

	return addr
}

// runService starts the service in the background and returns its address, the address of its
// metrics server, and the registry its collectors are registered with.
func runService(t *testing.T, certPath, keyPath string) (addr, metricsAddr string, registry *prom.Registry) {
	t.Helper()

	addr = freePort(t)
	metricsAddr = freePort(t)
	registry = prom.NewRegistry()
	logger := zaptest.NewLogger(t)

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)

	go func() {
		errCh <- service.Run(ctx, service.Options{
			ListenAddr:           addr,
			CertificatePath:      certPath,
			KeyPath:              keyPath,
			GCInterval:           time.Minute,
			MetricsRegisterer:    registry,
			MetricsServerEnabled: true,
			MetricsAddr:          metricsAddr,
		}, logger)
	}()

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-errCh:
			assert.NoError(t, err)
		case <-time.After(30 * time.Second):
			t.Error("service did not shut down")
		}
	})

	// wait for the listener to come up
	require.Eventually(t, func() bool {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return false
		}

		return conn.Close() == nil
	}, 10*time.Second, 50*time.Millisecond)

	return addr, metricsAddr, registry
}

// checkGRPC verifies that a gRPC client can talk to the service.
func checkGRPC(ctx context.Context, t *testing.T, addr string, creds credentials.TransportCredentials) {
	t.Helper()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, conn.Close()) })

	client := pb.NewClusterClient(conn)

	_, err = client.Hello(ctx, &pb.HelloRequest{ClusterId: "cluster-1", ClientVersion: "v1.0.0"})
	require.NoError(t, err)

	// a Watch stream should receive the initial (empty) snapshot
	watch, err := client.Watch(ctx, &pb.WatchRequest{ClusterId: "cluster-1"})
	require.NoError(t, err)

	resp, err := watch.Recv()
	require.NoError(t, err)
	assert.Empty(t, resp.Affiliates)
}

// checkHTTP verifies that the static assets are served on the very same port.
func checkHTTP(ctx context.Context, t *testing.T, url string, transport *http.Transport) {
	t.Helper()

	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "<html")
}

func TestRunTLS(t *testing.T) {
	t.Parallel()

	certPath, keyPath := testcert.WriteFiles(t)
	addr, metricsAddr, registry := runService(t, certPath, keyPath)

	//nolint:gosec
	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	// gRPC clients advertise "h2" only and are routed to the gRPC server
	checkGRPC(t.Context(), t, addr, credentials.NewTLS(tlsConfig))

	// HTTP clients advertising "http/1.1" get the static assets from the same port
	checkHTTP(t.Context(), t, "https://"+addr+"/", &http.Transport{TLSClientConfig: tlsConfig})

	// connections are counted per protocol, which is what tells a reconnect apart from a
	// request arriving on an existing connection: the gRPC client made a single connection and
	// used it for both Hello and Watch
	expected := `
# HELP discovery_connections_active The current number of connections on the main listener by protocol.
# TYPE discovery_connections_active gauge
discovery_connections_active{protocol="grpc"} 1
discovery_connections_active{protocol="http"} 1
# HELP discovery_connections_opened_total Number of connections accepted on the main listener by protocol.
# TYPE discovery_connections_opened_total counter
discovery_connections_opened_total{protocol="grpc"} 1
discovery_connections_opened_total{protocol="http"} 1
# HELP discovery_connections_rejected_total Number of connections which never reached a server: failed TLS handshake or protocol detection, or accepted during shutdown.
# TYPE discovery_connections_rejected_total counter
discovery_connections_rejected_total 1
`

	// the single rejected connection is the readiness probe above: it connects and hangs up
	// without a TLS handshake, which is exactly what that counter is meant to catch
	require.NoError(t, promtestutil.GatherAndCompare(registry, strings.NewReader(expected),
		"discovery_connections_opened_total", "discovery_connections_active", "discovery_connections_rejected_total"))

	// the metrics server serves the registry the collectors were registered with, not the
	// global default one
	assert.Contains(t, scrape(t, metricsAddr), `discovery_connections_opened_total{protocol="grpc"} 1`)
}

// scrape fetches the metrics endpoint.
func scrape(t *testing.T, metricsAddr string) string {
	t.Helper()

	var body []byte

	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+metricsAddr+"/metrics", nil)
		require.NoError(t, err)

		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			return false
		}

		defer resp.Body.Close() //nolint:errcheck

		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond)

	return string(body)
}

func TestRunInsecure(t *testing.T) {
	t.Parallel()

	addr, _, _ := runService(t, "", "")

	// gRPC over cleartext HTTP/2, detected by the client connection preface
	checkGRPC(t.Context(), t, addr, insecure.NewCredentials())

	// plain HTTP/1.1 on the same port
	checkHTTP(t.Context(), t, "http://"+addr+"/", &http.Transport{})
}

func TestStatsEndpoint(t *testing.T) {
	t.Parallel()

	addr, _, _ := runService(t, "", "")

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://%s/stats", addr), nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "affiliates")
}
