// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"golang.org/x/net/http2"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/siderolabs/discovery-service/internal/limiter"
	"github.com/siderolabs/discovery-service/internal/state"
	"github.com/siderolabs/discovery-service/pkg/limits"
	"github.com/siderolabs/discovery-service/pkg/server"
)

func checkMetrics(t testing.TB, c prom.Collector) {
	problems, err := promtestutil.CollectAndLint(c)
	require.NoError(t, err)
	require.Empty(t, problems)

	assert.NotZero(t, promtestutil.CollectAndCount(c), "collector should not be unchecked")
}

type testServer struct { //nolint:govet
	lis           net.Listener
	s             *grpc.Server
	httpServer    *http.Server
	state         *state.State
	stopCh        <-chan struct{}
	serverOptions []grpc.ServerOption

	address string
}

func setupServer(t testing.TB, rateLimit rate.Limit, redirectEndpoint string) *testServer {
	t.Helper()

	return setupServerWithLogger(t, rateLimit, redirectEndpoint, zaptest.NewLogger(t))
}

func setupServerWithLogger(t testing.TB, rateLimit rate.Limit, redirectEndpoint string, logger *zap.Logger) *testServer {
	t.Helper()

	testServer := &testServer{}

	testServer.state = state.NewState(logger)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	testServer.stopCh = ctx.Done()

	go func() {
		testServer.state.RunGC(ctx, logger, time.Second)
	}()

	srv := server.NewClusterServer(testServer.state, testServer.stopCh, redirectEndpoint)

	// Check metrics before and after the test
	// to ensure that collector does not switch from being unchecked to checked and invalid.
	checkMetrics(t, srv)
	t.Cleanup(func() { checkMetrics(t, srv) })

	var err error

	testServer.lis, err = net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	testServer.address = testServer.lis.Addr().String()

	limiter := limiter.NewIPRateLimiter(rateLimit, limits.IPRateBurstSizeMax)

	testServer.serverOptions = []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			server.UnaryRequestLogger(logger),
			server.RateLimitUnaryServerInterceptor(limiter),
		),
		grpc.ChainStreamInterceptor(
			server.StreamRequestLogger(logger),
			server.RateLimitStreamServerInterceptor(limiter),
		),
		grpc.SharedWriteBuffer(true),
		grpc.ReadBufferSize(16 * 1024),
		grpc.WriteBufferSize(16 * 1024),
	}

	testServer.s = grpc.NewServer(testServer.serverOptions...)
	pb.RegisterClusterServer(testServer.s, srv)

	testServer.httpServer = &http.Server{
		Handler:   testServer.s,
		TLSConfig: GetServerTLSConfig(t),
	}

	require.NoError(t, http2.ConfigureServer(testServer.httpServer, nil))

	go func() {
		if stopErr := testServer.httpServer.ServeTLS(testServer.lis, "", ""); stopErr != nil && !errors.Is(stopErr, http.ErrServerClosed) {
			assert.NoError(t, stopErr)
		}

		t.Logf("server stopped")
	}()

	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:usetesting
		defer shutdownCancel()

		assert.NoError(t, testServer.httpServer.Shutdown(shutdownCtx))
	})

	return testServer
}

func (testServer *testServer) restartWithRedirect(t *testing.T, redirectEndpoint string) {
	t.Logf("restarting server with redirect to %s", redirectEndpoint)

	assert.NoError(t, testServer.httpServer.Close())

	srv := server.NewClusterServer(testServer.state, testServer.stopCh, redirectEndpoint)

	testServer.s = grpc.NewServer(testServer.serverOptions...)
	pb.RegisterClusterServer(testServer.s, srv)

	var err error

	testServer.lis, err = net.Listen("tcp", testServer.address)
	require.NoError(t, err)

	testServer.httpServer = &http.Server{
		Handler:   testServer.s,
		TLSConfig: GetServerTLSConfig(t),
	}

	require.NoError(t, http2.ConfigureServer(testServer.httpServer, nil))

	go func() {
		if stopErr := testServer.httpServer.ServeTLS(testServer.lis, "", ""); stopErr != nil && !errors.Is(stopErr, http.ErrServerClosed) {
			assert.NoError(t, stopErr)
		}

		t.Logf("restarted server stopped")
	}()

	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer shutdownCancel()

		assert.NoError(t, testServer.httpServer.Shutdown(shutdownCtx))
	})
}

func TestServerAPI(t *testing.T) {
	t.Parallel()

	addr := setupServer(t, 5000, "").address

	conn, e := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(GetClientTLSConfig(t))))
	require.NoError(t, e)

	t.Cleanup(func() {
		assert.NoError(t, conn.Close())
	})

	client := pb.NewClusterClient(conn)

	t.Run("Hello", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		resp, err := client.Hello(ctx, &pb.HelloRequest{
			ClusterId:     "fake",
			ClientVersion: "v0.12.0",
		})
		require.NoError(t, err)

		assert.Equal(t, []byte{0x7f, 0x0, 0x0, 0x1}, resp.ClientIp) // 127.0.0.1
		assert.Nil(t, resp.Redirect)
	})

	t.Run("HelloWithRealIP", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		ctx = metadata.AppendToOutgoingContext(ctx, "X-Real-IP", "1.2.3.4") // with real IP of client

		resp, err := client.Hello(ctx, &pb.HelloRequest{
			ClusterId:     "fake",
			ClientVersion: "v0.12.0",
		})
		require.NoError(t, err)

		assert.Equal(t, []byte{0x1, 0x2, 0x3, 0x4}, resp.ClientIp)
	})

	t.Run("AffiliateUpdate", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:     "fake1",
			AffiliateId:   "af1",
			AffiliateData: []byte("data1"),
			AffiliateEndpoints: [][]byte{
				[]byte("e1"),
				[]byte("e2"),
			},
			Ttl: durationpb.New(time.Minute),
		})
		require.NoError(t, err)

		resp, err := client.List(ctx, &pb.ListRequest{
			ClusterId: "fake1",
		})
		require.NoError(t, err)

		require.Len(t, resp.Affiliates, 1)
		assert.Equal(t, "af1", resp.Affiliates[0].Id)
		assert.Equal(t, []byte("data1"), resp.Affiliates[0].Data)
		assert.Equal(t, [][]byte{[]byte("e1"), []byte("e2")}, resp.Affiliates[0].Endpoints)

		// add more endpoints
		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:   "fake1",
			AffiliateId: "af1",
			AffiliateEndpoints: [][]byte{
				[]byte("e3"),
				[]byte("e2"),
			},
			Ttl: durationpb.New(time.Minute),
		})
		require.NoError(t, err)

		resp, err = client.List(ctx, &pb.ListRequest{
			ClusterId: "fake1",
		})
		require.NoError(t, err)

		require.Len(t, resp.Affiliates, 1)
		assert.Equal(t, "af1", resp.Affiliates[0].Id)
		assert.Equal(t, []byte("data1"), resp.Affiliates[0].Data)
		assert.Equal(t, [][]byte{[]byte("e1"), []byte("e2"), []byte("e3")}, resp.Affiliates[0].Endpoints)
	})

	t.Run("AffiliateDelete", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		_, err := client.AffiliateDelete(ctx, &pb.AffiliateDeleteRequest{
			ClusterId:   "fake2",
			AffiliateId: "af1",
		})
		require.NoError(t, err)

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:     "fake2",
			AffiliateId:   "af1",
			AffiliateData: []byte("data1"),
		})
		require.NoError(t, err)

		_, err = client.AffiliateDelete(ctx, &pb.AffiliateDeleteRequest{
			ClusterId:   "fake2",
			AffiliateId: "af1",
		})
		require.NoError(t, err)

		resp, err := client.List(ctx, &pb.ListRequest{
			ClusterId: "fake2",
		})
		require.NoError(t, err)

		assert.Len(t, resp.Affiliates, 0)
	})

	t.Run("Watch", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:     "fake3",
			AffiliateId:   "af1",
			AffiliateData: []byte("data1"),
		})
		require.NoError(t, err)

		cli, err := client.Watch(ctx, &pb.WatchRequest{
			ClusterId: "fake3",
		})
		require.NoError(t, err)

		msg, err := cli.Recv()
		require.NoError(t, err)

		assert.True(t, proto.Equal(&pb.WatchResponse{
			Deleted: false,
			Affiliates: []*pb.Affiliate{
				{
					Id:        "af1",
					Data:      []byte("data1"),
					Endpoints: [][]byte{},
				},
			},
		}, msg))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:     "fake3",
			AffiliateId:   "af2",
			AffiliateData: []byte("data2"),
		})
		require.NoError(t, err)

		msg, err = cli.Recv()
		require.NoError(t, err)

		assert.True(t, proto.Equal(&pb.WatchResponse{
			Deleted: false,
			Affiliates: []*pb.Affiliate{
				{
					Id:        "af2",
					Data:      []byte("data2"),
					Endpoints: [][]byte{},
				},
			},
		}, msg))

		_, err = client.AffiliateDelete(ctx, &pb.AffiliateDeleteRequest{
			ClusterId:   "fake3",
			AffiliateId: "af1",
		})
		require.NoError(t, err)

		msg, err = cli.Recv()
		require.NoError(t, err)

		assert.True(t, proto.Equal(&pb.WatchResponse{
			Deleted: true,
			Affiliates: []*pb.Affiliate{
				{
					Id: "af1",
				},
			},
		}, msg))
	})
}

func TestValidation(t *testing.T) {
	t.Parallel()

	addr := setupServer(t, 5000, "").address

	conn, e := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(GetClientTLSConfig(t))))
	require.NoError(t, e)

	client := pb.NewClusterClient(conn)

	ctx := t.Context()

	t.Run("Hello", func(t *testing.T) {
		t.Parallel()

		_, err := client.Hello(ctx, &pb.HelloRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.Hello(ctx, &pb.HelloRequest{
			ClusterId: strings.Repeat("A", 1024),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("AffiliateUpdate", func(t *testing.T) {
		t.Parallel()

		_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:   strings.Repeat("A", limits.ClusterIDMax+1),
			AffiliateId: "fake",
			Ttl:         durationpb.New(time.Minute),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:   "fake",
			AffiliateId: strings.Repeat("A", limits.AffiliateIDMax+1),
			Ttl:         durationpb.New(time.Minute),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:     "fake",
			AffiliateId:   "fake",
			AffiliateData: bytes.Repeat([]byte{0}, limits.AffiliateDataMax+1),
			Ttl:           durationpb.New(time.Minute),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:          "fake",
			AffiliateId:        "fake",
			AffiliateEndpoints: [][]byte{bytes.Repeat([]byte{0}, limits.AffiliateEndpointMax+1)},
			Ttl:                durationpb.New(time.Minute),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("AffiliateUpdateTooMany", func(t *testing.T) {
		t.Parallel()

		for i := range limits.ClusterAffiliatesMax {
			_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
				ClusterId:   "fatcluster",
				AffiliateId: fmt.Sprintf("af%d", i),
				Ttl:         durationpb.New(time.Minute),
			})
			require.NoError(t, err)
		}

		_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:   "fatcluster",
			AffiliateId: "af",
			Ttl:         durationpb.New(time.Minute),
		})
		require.Error(t, err)
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})

	t.Run("AffiliateUpdateTooManyEndpoints", func(t *testing.T) {
		t.Parallel()

		for i := range limits.AffiliateEndpointsMax {
			_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
				ClusterId:          "smallcluster",
				AffiliateId:        "af",
				AffiliateEndpoints: [][]byte{[]byte(fmt.Sprintf("endpoint%d", i))},
				Ttl:                durationpb.New(time.Minute),
			})
			require.NoError(t, err)
		}

		_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:          "smallcluster",
			AffiliateId:        "af",
			AffiliateEndpoints: [][]byte{[]byte("endpoin")},
			Ttl:                durationpb.New(time.Minute),
		})
		require.Error(t, err)
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})

	t.Run("AffiliateDelete", func(t *testing.T) {
		t.Parallel()

		_, err := client.AffiliateDelete(ctx, &pb.AffiliateDeleteRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateDelete(ctx, &pb.AffiliateDeleteRequest{
			ClusterId:   strings.Repeat("A", 1024),
			AffiliateId: "fake",
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateDelete(ctx, &pb.AffiliateDeleteRequest{
			ClusterId:   "fake",
			AffiliateId: strings.Repeat("A", 1024),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()

		_, err := client.List(ctx, &pb.ListRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.List(ctx, &pb.ListRequest{
			ClusterId: strings.Repeat("A", 1024),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("Watch", func(t *testing.T) {
		t.Parallel()

		cli, err := client.Watch(ctx, &pb.WatchRequest{})
		require.NoError(t, err)

		_, err = cli.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		cli, err = client.Watch(ctx, &pb.WatchRequest{
			ClusterId: strings.Repeat("A", 1024),
		})
		require.NoError(t, err)

		_, err = cli.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func testHitRateLimit(client pb.ClusterClient, ip string) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()

		ctx = metadata.AppendToOutgoingContext(ctx, "X-Real-IP", ip)

		for range limits.IPRateBurstSizeMax {
			_, err := client.Hello(ctx, &pb.HelloRequest{
				ClusterId:     "fake",
				ClientVersion: "v0.12.0",
			})
			require.NoError(t, err)
		}

		_, err := client.Hello(ctx, &pb.HelloRequest{
			ClusterId:     "fake",
			ClientVersion: "v0.12.0",
		})
		require.Error(t, err)
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	}
}

func TestServerRateLimit(t *testing.T) {
	t.Parallel()

	addr := setupServer(t, 1, "").address

	conn, e := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(GetClientTLSConfig(t))))
	require.NoError(t, e)

	client := pb.NewClusterClient(conn)

	t.Run("HitRateLimitIP1", testHitRateLimit(client, "1.2.3.4"))
	t.Run("HitRateLimitIP2", testHitRateLimit(client, "5.6.7.8"))
}

func TestServerRedirect(t *testing.T) {
	t.Parallel()

	addr := setupServer(t, 1, "new.example.com:443").address

	conn, e := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(GetClientTLSConfig(t))))
	require.NoError(t, e)

	client := pb.NewClusterClient(conn)

	ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
	defer cancel()

	resp, err := client.Hello(ctx, &pb.HelloRequest{
		ClusterId:     "fake",
		ClientVersion: "v0.12.0",
	})
	require.NoError(t, err)

	assert.Equal(t, "new.example.com:443", resp.GetRedirect().GetEndpoint())
}

func BenchmarkViaClient(b *testing.B) {
	endpoint := setupServerWithLogger(b, 500000, "", zap.NewNop()).address

	conn, e := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(GetClientTLSConfig(b))),
	)
	require.NoError(b, e)

	b.Cleanup(func() {
		assert.NoError(b, conn.Close())
	})

	client := pb.NewClusterClient(conn)

	ctx, cancel := context.WithTimeout(b.Context(), 10*time.Second)
	b.Cleanup(cancel)

	helloReq := &pb.HelloRequest{
		ClusterId:     "fake",
		ClientVersion: "v0.12.0",
	}

	affiliateReq := &pb.AffiliateUpdateRequest{
		ClusterId:     "fake1",
		AffiliateId:   "af1",
		AffiliateData: bytes.Repeat([]byte("a"), 1024),
		AffiliateEndpoints: [][]byte{
			[]byte("e1"),
			[]byte("e2"),
		},
		Ttl: durationpb.New(time.Minute),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, err := client.Hello(ctx, helloReq)
		require.NoError(b, err)

		_, err = client.AffiliateUpdate(ctx, affiliateReq)
		require.NoError(b, err)
	}
}

func init() {
	server.TrustXRealIP(true)
}
