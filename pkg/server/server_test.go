// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package server_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	prom "github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/talos-systems/discovery-api/api/v1alpha1/server/pb"
	"go.uber.org/zap/zaptest"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	_ "github.com/siderolabs/discovery-service/internal/proto"
	"github.com/siderolabs/discovery-service/internal/state"
	"github.com/siderolabs/discovery-service/pkg/limits"
	"github.com/siderolabs/discovery-service/pkg/server"
)

func checkMetrics(t *testing.T, c prom.Collector) {
	problems, err := promtestutil.CollectAndLint(c)
	require.NoError(t, err)
	require.Empty(t, problems)

	assert.NotZero(t, promtestutil.CollectAndCount(c), "collector should not be unchecked")
}

func setupServer(t *testing.T, rateLimit rate.Limit) (address string) {
	t.Helper()

	logger := zaptest.NewLogger(t)

	state := state.NewState(logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		state.RunGC(ctx, logger, time.Second)
	}()

	srv := server.NewClusterServer(state, ctx.Done())

	// Check metrics before and after the test
	// to ensure that collector does not switch from being unchecked to checked and invalid.
	checkMetrics(t, srv)
	t.Cleanup(func() { checkMetrics(t, srv) })

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	limiter := limits.NewIPRateLimiter(rateLimit, limits.BurstSizeMax)

	serverOptions := []grpc.ServerOption{
		grpc_middleware.WithUnaryServerChain(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(server.FieldExtractor)),
			server.AddPeerAddressUnaryServerInterceptor(),
			server.RateLimitUnaryServerInterceptor(limiter),
		),
		grpc_middleware.WithStreamServerChain(
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(server.FieldExtractor)),
			server.AddPeerAddressStreamServerInterceptor(),
			server.RateLimitStreamServerInterceptor(limiter),
		),
	}

	s := grpc.NewServer(serverOptions...)
	pb.RegisterClusterServer(s, srv)

	go func() {
		require.NoError(t, s.Serve(lis))
	}()

	t.Cleanup(s.Stop)

	return lis.Addr().String()
}

func TestServerAPI(t *testing.T) {
	t.Parallel()

	addr := setupServer(t, 5000)

	conn, e := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, e)

	client := pb.NewClusterClient(conn)

	t.Run("Hello", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		resp, err := client.Hello(ctx, &pb.HelloRequest{
			ClusterId:     "fake",
			ClientVersion: "v0.12.0",
		})
		require.NoError(t, err)

		assert.Equal(t, []byte{0x7f, 0x0, 0x0, 0x1}, resp.ClientIp) // 127.0.0.1
	})

	t.Run("HelloWithRealIP", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
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

		ctx, cancel := context.WithCancel(context.Background())
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

		ctx, cancel := context.WithCancel(context.Background())
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	addr := setupServer(t, 5000)

	conn, e := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, e)

	client := pb.NewClusterClient(conn)

	ctx := context.Background()

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

		for i := 0; i < limits.ClusterAffiliatesMax; i++ {
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

		for i := 0; i < limits.AffiliateEndpointsMax; i++ {
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

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		ctx = metadata.AppendToOutgoingContext(ctx, "X-Real-IP", ip)

		for i := 0; i < limits.BurstSizeMax; i++ {
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

	addr := setupServer(t, 1)

	conn, e := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, e)

	client := pb.NewClusterClient(conn)

	t.Run("HitRateLimitIP1", testHitRateLimit(client, "1.2.3.4"))
	t.Run("HitRateLimitIP2", testHitRateLimit(client, "5.6.7.8"))
}
