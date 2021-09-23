// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package server_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/talos-systems/discovery-service/api/v1alpha1/pb"
	_ "github.com/talos-systems/discovery-service/internal/proto"
	"github.com/talos-systems/discovery-service/internal/state"
	"github.com/talos-systems/discovery-service/pkg/limits"
	"github.com/talos-systems/discovery-service/pkg/server"
)

func setupServer(t *testing.T) (address string) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	pb.RegisterClusterServer(s, server.NewClusterServer(state.NewState()))

	go func() {
		require.NoError(t, s.Serve(lis))
	}()

	t.Cleanup(s.Stop)

	return lis.Addr().String()
}

func TestServerAPI(t *testing.T) {
	t.Parallel()

	addr := setupServer(t)

	conn, e := grpc.Dial(addr, grpc.WithInsecure())
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

	addr := setupServer(t)

	conn, e := grpc.Dial(addr, grpc.WithInsecure())
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
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:   "fake",
			AffiliateId: strings.Repeat("A", limits.AffiliateIDMax+1),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:     "fake",
			AffiliateId:   "fake",
			AffiliateData: bytes.Repeat([]byte{0}, limits.AffiliateDataMax+1),
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:          "fake",
			AffiliateId:        "fake",
			AffiliateEndpoints: [][]byte{bytes.Repeat([]byte{0}, limits.AffiliateEndpointMax+1)},
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
			})
			require.NoError(t, err)
		}

		_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:   "fatcluster",
			AffiliateId: "af",
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
			})
			require.NoError(t, err)
		}

		_, err := client.AffiliateUpdate(ctx, &pb.AffiliateUpdateRequest{
			ClusterId:          "smallcluster",
			AffiliateId:        "af",
			AffiliateEndpoints: [][]byte{[]byte("endpoin")},
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
