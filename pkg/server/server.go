// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package server implements server-side part of gRPC API.
package server

import (
	"context"
	"errors"
	"net"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"inet.af/netaddr"

	"github.com/talos-systems/discovery-service/api/v1alpha1/server/pb"
	"github.com/talos-systems/discovery-service/internal/state"
)

const updateBuffer = 32

// ClusterServer implements discovery cluster gRPC API.
type ClusterServer struct {
	pb.UnimplementedClusterServer

	state  *state.State
	stopCh <-chan struct{}

	mHello *prom.CounterVec
}

// NewClusterServer builds new ClusterServer.
func NewClusterServer(state *state.State, stopCh <-chan struct{}) *ClusterServer {
	srv := &ClusterServer{
		state:  state,
		stopCh: stopCh,
		mHello: prom.NewCounterVec(prom.CounterOpts{
			Name: "discovery_server_hello_requests_total",
			Help: "Number of hello requests by client version.",
		}, []string{"client_version"}),
	}

	// initialize vectors to set correct descriptors
	srv.mHello.WithLabelValues("unknown")

	return srv
}

// NewTestClusterServer builds cluster server for testing code.
func NewTestClusterServer() *ClusterServer {
	return NewClusterServer(state.NewState(), nil)
}

// Hello implements cluster API.
func (srv *ClusterServer) Hello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	clientVersion := req.ClientVersion
	if clientVersion == "" {
		clientVersion = "unknown"
	}

	srv.mHello.WithLabelValues(clientVersion).Inc()

	if err := validateClusterID(req.ClusterId); err != nil {
		return nil, err
	}

	resp := &pb.HelloResponse{}

	if peer, ok := peer.FromContext(ctx); ok {
		if addr, ok := peer.Addr.(*net.TCPAddr); ok {
			if ip, ok := netaddr.FromStdIP(addr.IP); ok {
				var err error

				resp.ClientIp, err = ip.MarshalBinary()
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return resp, nil
}

// AffiliateUpdate implements cluster API.
func (srv *ClusterServer) AffiliateUpdate(ctx context.Context, req *pb.AffiliateUpdateRequest) (*pb.AffiliateUpdateResponse, error) {
	if err := validateClusterID(req.ClusterId); err != nil {
		return nil, err
	}

	if err := validateAffiliateID(req.AffiliateId); err != nil {
		return nil, err
	}

	if err := validateAffiliateData(req.AffiliateData); err != nil {
		return nil, err
	}

	if err := validateAffiliateEndpoints(req.AffiliateEndpoints); err != nil {
		return nil, err
	}

	if err := validateTTL(req.Ttl.AsDuration()); err != nil {
		return nil, err
	}

	if err := srv.state.GetCluster(req.ClusterId).WithAffiliate(req.AffiliateId, func(affiliate *state.Affiliate) error {
		expiration := time.Now().Add(req.Ttl.AsDuration())

		if len(req.AffiliateData) > 0 {
			affiliate.Update(req.AffiliateData, expiration)
		}

		return affiliate.MergeEndpoints(req.AffiliateEndpoints, expiration)
	}); err != nil {
		switch {
		case errors.Is(err, state.ErrTooManyEndpoints):
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		case errors.Is(err, state.ErrTooManyAffiliates):
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		default:
			return nil, err
		}
	}

	return &pb.AffiliateUpdateResponse{}, nil
}

// AffiliateDelete implements cluster API.
func (srv *ClusterServer) AffiliateDelete(ctx context.Context, req *pb.AffiliateDeleteRequest) (*pb.AffiliateDeleteResponse, error) {
	if err := validateClusterID(req.ClusterId); err != nil {
		return nil, err
	}

	if err := validateAffiliateID(req.AffiliateId); err != nil {
		return nil, err
	}

	srv.state.GetCluster(req.ClusterId).DeleteAffiliate(req.AffiliateId)

	return &pb.AffiliateDeleteResponse{}, nil
}

// List implements cluster API.
func (srv *ClusterServer) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	if err := validateClusterID(req.ClusterId); err != nil {
		return nil, err
	}

	affiliates := srv.state.GetCluster(req.ClusterId).List()
	resp := &pb.ListResponse{
		Affiliates: make([]*pb.Affiliate, 0, len(affiliates)),
	}

	for _, affiliate := range affiliates {
		resp.Affiliates = append(resp.Affiliates, &pb.Affiliate{
			Id:        affiliate.ID,
			Data:      affiliate.Data,
			Endpoints: affiliate.Endpoints,
		})
	}

	return resp, nil
}

// Watch implements cluster API.
func (srv *ClusterServer) Watch(req *pb.WatchRequest, server pb.Cluster_WatchServer) error {
	if err := validateClusterID(req.ClusterId); err != nil {
		return err
	}

	// make enough room to handle connection issues
	updates := make(chan *state.Notification, updateBuffer)

	snapshot, subscription := srv.state.GetCluster(req.ClusterId).Subscribe(updates)
	defer subscription.Close()

	snapshotResp := &pb.WatchResponse{}

	for _, affiliate := range snapshot {
		snapshotResp.Affiliates = append(snapshotResp.Affiliates,
			&pb.Affiliate{
				Id:        affiliate.ID,
				Data:      affiliate.Data,
				Endpoints: affiliate.Endpoints,
			})
	}

	if err := server.Send(snapshotResp); err != nil {
		if status.Code(err) == codes.Canceled {
			return nil
		}

		return err
	}

	for {
		select {
		case <-server.Context().Done():
			return nil
		case <-srv.stopCh:
			return nil
		case err := <-subscription.ErrCh():
			return status.Errorf(codes.Aborted, "subscription canceled: %s", err)
		case notification := <-updates:
			resp := &pb.WatchResponse{}

			if notification.Affiliate == nil {
				resp.Deleted = true
				resp.Affiliates = []*pb.Affiliate{
					{
						Id: notification.AffiliateID,
					},
				}
			} else {
				resp.Affiliates = []*pb.Affiliate{
					{
						Id:        notification.Affiliate.ID,
						Data:      notification.Affiliate.Data,
						Endpoints: notification.Affiliate.Endpoints,
					},
				}
			}

			if err := server.Send(resp); err != nil {
				if status.Code(err) == codes.Canceled {
					return nil
				}

				return err
			}
		}
	}
}

// Describe implements prom.Collector interface.
func (srv *ClusterServer) Describe(ch chan<- *prom.Desc) {
	prom.DescribeByCollect(srv, ch)
}

// Collect implements prom.Collector interface.
func (srv *ClusterServer) Collect(ch chan<- prom.Metric) {
	srv.mHello.Collect(ch)
}

// Check interfaces.
var (
	_ prom.Collector = (*ClusterServer)(nil)
)
