// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package client provides discovery service client.
package client

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	clientpb "github.com/talos-systems/discovery-service/api/v1alpha1/client/pb"
	serverpb "github.com/talos-systems/discovery-service/api/v1alpha1/server/pb"
)

// Options configures the client.
type Options struct {
	// Cipher, should have 32-bytes (128-bit key).
	Cipher cipher.Block
	// gRPC initial endpoint.
	Endpoint string
	// ClusterID of the client.
	ClusterID string
	// AffiliateID of the client.
	AffiliateID string
	// ClientVersion for the Hello request.
	ClientVersion string
	// TTL for the submitted data.
	TTL time.Duration
	// Insecure gRPC connection (only for testing)
	Insecure bool
}

// Client wraps all details related to discovery service interaction.
//
// Inputs for the client are:
// * Affiliate protobuf structure (for the node itself)
// * Additional endpoints discovered for other nodes (affiliates)
//
// Outputs are:
// * list of Affiliates (except for the node itself)
// * channel which notifies when list of Affiliates changes
//
// Client handles encryption of the data.
//
//nolint:govet
type Client struct {
	options Options

	localMu        sync.Mutex
	localAffiliate []byte
	localEndpoints [][]byte
	otherEndpoints []endpointData
	localUpdatesCh chan struct{}

	discoveredMu         sync.Mutex
	discoveredAffiliates map[string]*Affiliate

	gcm     cipher.AEAD
	backoff *backoff.ExponentialBackOff
}

// Affiliate information.
type Affiliate struct {
	Affiliate *clientpb.Affiliate
	Endpoints []*clientpb.Endpoint
}

type endpointData struct {
	affiliateID string
	endpoints   [][]byte
}

// NewClient initializes a client.
func NewClient(options Options) (*Client, error) {
	client := &Client{
		options: options,

		localUpdatesCh: make(chan struct{}, 1),
	}

	var err error

	client.gcm, err = cipher.NewGCM(client.options.Cipher)
	if err != nil {
		return nil, fmt.Errorf("error creating GCM encryption: %w", err)
	}

	client.backoff = backoff.NewExponentialBackOff()

	// don't limit retries
	client.backoff.MaxElapsedTime = 0

	return client, nil
}

// Endpoint specified additional endpoints for other affiliates.
type Endpoint struct {
	AffiliateID string
	Endpoints   []*clientpb.Endpoint
}

func encryptEndpoints(cipher cipher.Block, endpoints []*clientpb.Endpoint) ([][]byte, error) {
	result := make([][]byte, 0, len(endpoints))

	for i := range endpoints {
		data, err := endpoints[i].MarshalVT()
		if err != nil {
			return nil, fmt.Errorf("error marshaling endpoint: %w", err)
		}

		if len(data) > cipher.BlockSize()-1 {
			return nil, fmt.Errorf("endpoint is too big: %d", len(data))
		}

		data = append([]byte{byte(len(data))}, data...)

		// pad to cipher block size
		data = append(data, bytes.Repeat([]byte{0}, cipher.BlockSize()-len(data))...)

		// using ECB encryption to make sure endpoints can be deduplicated server-side
		cipher.Encrypt(data, data)

		result = append(result, data)
	}

	return result, nil
}

// SetLocalData updates local affiliate data.
func (client *Client) SetLocalData(localAffiliate *Affiliate, otherEndpoints []Endpoint) error {
	nonce := make([]byte, client.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("error building nonce: %w", err)
	}

	localAffiliateData, err := localAffiliate.Affiliate.MarshalVT()
	if err != nil {
		return fmt.Errorf("error marshaling local affiliate data: %w", err)
	}

	client.localMu.Lock()
	defer client.localMu.Unlock()

	client.localAffiliate = append([]byte(nil), nonce...)
	client.localAffiliate = client.gcm.Seal(client.localAffiliate, nonce, localAffiliateData, nil)

	client.localEndpoints, err = encryptEndpoints(client.options.Cipher, localAffiliate.Endpoints)
	if err != nil {
		return err
	}

	client.otherEndpoints = make([]endpointData, 0, len(otherEndpoints))

	for i := range otherEndpoints {
		endpoints, err := encryptEndpoints(client.options.Cipher, otherEndpoints[i].Endpoints)
		if err != nil {
			return err
		}

		client.otherEndpoints = append(client.otherEndpoints, endpointData{
			affiliateID: otherEndpoints[i].AffiliateID,
			endpoints:   endpoints,
		})
	}

	select {
	case client.localUpdatesCh <- struct{}{}:
	default:
	}

	return nil
}

// GetAffiliates returns discovered affiliates.
func (client *Client) GetAffiliates() []*Affiliate {
	client.discoveredMu.Lock()
	defer client.discoveredMu.Unlock()

	result := make([]*Affiliate, 0, len(client.discoveredAffiliates))

	for _, affiliate := range client.discoveredAffiliates {
		result = append(result, affiliate)
	}

	return result
}

// Run the client loop.
//
// Client automatically keeps the connection, refreshes data based on TTL.
// Run loop shuts down when context is canceled.
//
//nolint:gocognit,cyclop,gocyclo
func (client *Client) Run(ctx context.Context, logger *zap.Logger, notifyCh chan<- struct{}) error {
	var (
		discoveryConn   *grpc.ClientConn
		discoveryClient serverpb.ClusterClient
	)

	defer func() {
		if discoveryConn != nil {
			discoveryConn.Close() //nolint:errcheck
		}
	}()

	var (
		watchCh        <-chan watchReply
		watchCtx       context.Context
		watchCtxCancel context.CancelFunc
	)

	defer func() {
		if watchCtxCancel != nil {
			watchCtxCancel()
		}
	}()

	// refresh data on TTL/2 anyways
	ticker := time.NewTicker(client.options.TTL / 2)
	defer ticker.Stop()

	refreshData := true

	for ctx.Err() == nil {
		// establish connection
		if discoveryConn == nil {
			var err error

			opts := []grpc.DialOption{}

			if client.options.Insecure {
				opts = append(opts, grpc.WithInsecure())
			}

			discoveryConn, err = grpc.DialContext(ctx, client.options.Endpoint, opts...)
			if err != nil {
				return err
			}

			discoveryClient = serverpb.NewClusterClient(discoveryConn)
		}

		// establish watch if none was found
		if watchCh == nil {
			waitInterval := client.backoff.NextBackOff()

			logger.Debug("waiting before attempting next discovery refresh", zap.Duration("interval", waitInterval))

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(waitInterval):
			}

			// send Hello before establishing watch, as real reconnects are handled
			// by gRPC and are not visible to the client
			newEndpoint, err := client.sendHello(ctx, discoveryClient)
			if err != nil {
				logger.Error("hello failed", zap.Error(err), zap.String("endpoint", client.options.Endpoint))
			}

			if newEndpoint != "" {
				// reconnect to new endpoint
				client.options.Endpoint = newEndpoint

				discoveryConn.Close() //nolint:errcheck

				discoveryConn = nil
				discoveryClient = nil

				continue
			}

			watchCtx, watchCtxCancel = context.WithCancel(ctx) //nolint:govet

			watchCh = watch(watchCtx, discoveryClient, client.options.ClusterID)
		}

		if refreshData {
			if err := client.refreshData(ctx, discoveryClient); err != nil {
				// failed to refresh, abort watch
				watchCtxCancel()

				logger.Error("failed refreshing discovery service data", zap.Error(err))
			} else {
				refreshData = false

				client.backoff.Reset()
			}
		}

		select {
		case <-ctx.Done():
			return nil //nolint:govet
		case <-ticker.C:
			// time to refresh
			refreshData = true
		case <-client.localUpdatesCh:
			// new data
			refreshData = true
		case reply := <-watchCh:
			if reply.err != nil {
				// watch connection errored out
				watchCh = nil

				if watchCtxCancel != nil {
					watchCtxCancel()
				}

				watchCtxCancel = nil

				refreshData = true

				if status.Code(reply.err) != codes.Canceled {
					logger.Error("error watching discovery service state", zap.Error(reply.err))
				}
			} else {
				// new data arrived
				client.parseReply(logger, reply)

				select {
				case notifyCh <- struct{}{}:
				default:
				}
			}
		}
	}

	return nil
}

func (client *Client) parseReply(logger *zap.Logger, reply watchReply) {
	client.discoveredMu.Lock()
	defer client.discoveredMu.Unlock()

	// clear current data if it's a snapshot
	if reply.snapshot {
		client.discoveredAffiliates = make(map[string]*Affiliate, len(reply.resp.Affiliates))
	}

	for _, affiliate := range reply.resp.Affiliates {
		if affiliate.Id == client.options.AffiliateID {
			// skip updates about itself
			continue
		}

		if len(affiliate.Data) == 0 {
			// no affiliate data (yet?), skip it
			continue
		}

		if len(affiliate.Data) < client.gcm.NonceSize() {
			// malformed affiliated data?
			logger.Error("short data", zap.String("affiliate_id", affiliate.Id))

			continue
		}

		parsedAffiliate := &Affiliate{
			Affiliate: &clientpb.Affiliate{},
		}

		nonce, ciphertext := affiliate.Data[:client.gcm.NonceSize()], affiliate.Data[client.gcm.NonceSize():]

		data, err := client.gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			logger.Error("decryption failure", zap.String("affiliate_id", affiliate.Id), zap.Error(err))

			continue
		}

		if err = parsedAffiliate.Affiliate.UnmarshalVT(data); err != nil {
			logger.Error("unmarshal failure", zap.String("affiliate_id", affiliate.Id), zap.Error(err))

			continue
		}

		for _, endpoint := range affiliate.Endpoints {
			if len(endpoint) != client.options.Cipher.BlockSize() {
				logger.Error("endpoint size is not cipher block size", zap.String("affiliate_id", affiliate.Id))

				continue
			}

			client.options.Cipher.Decrypt(endpoint, endpoint)

			var size byte

			size, endpoint = endpoint[0], endpoint[1:]
			endpoint = endpoint[:size]

			endpt := &clientpb.Endpoint{}

			if err = endpt.UnmarshalVT(endpoint); err != nil {
				logger.Error("endpoint unmarshal failure", zap.String("affiliate_id", affiliate.Id), zap.Error(err))

				continue
			}

			parsedAffiliate.Endpoints = append(parsedAffiliate.Endpoints, endpt)
		}

		client.discoveredAffiliates[affiliate.Id] = parsedAffiliate
	}
}

func (client *Client) sendHello(ctx context.Context, discoveryClient serverpb.ClusterClient) (newEndpoint string, err error) {
	// set timeout for operation
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := discoveryClient.Hello(ctx, &serverpb.HelloRequest{
		ClusterId:     client.options.ClusterID,
		ClientVersion: client.options.ClientVersion,
	})
	if err != nil {
		return "", err
	}

	if resp.Redirect != nil {
		return resp.Redirect.Endpoint, nil
	}

	return "", nil
}

func (client *Client) refreshData(ctx context.Context, discoveryClient serverpb.ClusterClient) error {
	// set timeout for all updates
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client.localMu.Lock()
	localAffiliate := client.localAffiliate
	localEndpoints := client.localEndpoints
	otherEndpoints := client.otherEndpoints
	client.localMu.Unlock()

	if localAffiliate == nil {
		// no local data yet
		return nil
	}

	_, err := discoveryClient.AffiliateUpdate(ctx, &serverpb.AffiliateUpdateRequest{
		ClusterId:          client.options.ClusterID,
		AffiliateId:        client.options.AffiliateID,
		AffiliateData:      localAffiliate,
		AffiliateEndpoints: localEndpoints,
		Ttl:                durationpb.New(client.options.TTL),
	})
	if err != nil {
		return fmt.Errorf("error updating local affiliate data: %w", err)
	}

	for _, otherEndpoint := range otherEndpoints {
		_, err := discoveryClient.AffiliateUpdate(ctx, &serverpb.AffiliateUpdateRequest{
			ClusterId:          client.options.ClusterID,
			AffiliateId:        otherEndpoint.affiliateID,
			AffiliateEndpoints: otherEndpoint.endpoints,
			Ttl:                durationpb.New(client.options.TTL),
		})
		if err != nil {
			return fmt.Errorf("error updating local affiliate data: %w", err)
		}
	}

	return nil
}

type watchReply struct {
	resp     *serverpb.WatchResponse
	err      error
	snapshot bool
}

func watch(ctx context.Context, client serverpb.ClusterClient, clusterID string) <-chan watchReply {
	ch := make(chan watchReply, 1)

	go func() {
		cli, err := client.Watch(ctx, &serverpb.WatchRequest{
			ClusterId: clusterID,
		})
		if err != nil {
			ch <- watchReply{
				err: err,
			}

			return
		}

		isSnapshot := true

		for ctx.Err() == nil {
			resp, err := cli.Recv()
			ch <- watchReply{
				snapshot: isSnapshot,
				resp:     resp,
				err:      err,
			}

			isSnapshot = false

			if err != nil {
				return
			}
		}
	}()

	return ch
}
