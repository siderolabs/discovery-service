// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package server_test

import (
	"context"
	"crypto/aes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientpb "github.com/talos-systems/discovery-api/api/v1alpha1/client/pb"
	"github.com/talos-systems/discovery-client/pkg/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sync/errgroup"
)

//nolint:gocognit,gocyclo,cyclop
func TestClient(t *testing.T) {
	t.Parallel()

	endpoint := setupServer(t)

	logger := zaptest.NewLogger(t)

	t.Run("TwoClients", func(t *testing.T) {
		t.Parallel()

		clusterID := "cluster_1"

		key := make([]byte, 32)
		_, err := io.ReadFull(rand.Reader, key)
		require.NoError(t, err)

		cipher, err := aes.NewCipher(key)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		affiliate1 := "af_1"
		affiliate2 := "af_2"

		client1, err := client.NewClient(client.Options{
			Cipher:      cipher,
			Endpoint:    endpoint,
			ClusterID:   clusterID,
			AffiliateID: affiliate1,
			TTL:         time.Minute,
			Insecure:    true,
		})
		require.NoError(t, err)

		client2, err := client.NewClient(client.Options{
			Cipher:      cipher,
			Endpoint:    endpoint,
			ClusterID:   clusterID,
			AffiliateID: affiliate2,
			TTL:         time.Minute,
			Insecure:    true,
		})
		require.NoError(t, err)

		notify1 := make(chan struct{}, 1)
		notify2 := make(chan struct{}, 1)

		eg, ctx := errgroup.WithContext(ctx)

		eg.Go(func() error {
			return client1.Run(ctx, logger, notify1)
		})

		eg.Go(func() error {
			return client2.Run(ctx, logger, notify2)
		})

		select {
		case <-notify1:
		case <-time.After(2 * time.Second):
			require.Fail(t, "no initial snapshot update")
		}

		assert.Empty(t, client1.GetAffiliates())

		select {
		case <-notify2:
		case <-time.After(2 * time.Second):
			require.Fail(t, "no initial snapshot update")
		}

		assert.Empty(t, client2.GetAffiliates())

		affiliate1PB := &client.Affiliate{
			Affiliate: &clientpb.Affiliate{
				NodeId:      affiliate1,
				Addresses:   [][]byte{{1, 2, 3}},
				Hostname:    "host1",
				Nodename:    "node1",
				MachineType: "controlplane",
			},
		}

		require.NoError(t, client1.SetLocalData(affiliate1PB, nil))

		affiliate2PB := &client.Affiliate{
			Affiliate: &clientpb.Affiliate{
				NodeId:      affiliate2,
				Addresses:   [][]byte{{2, 3, 4}},
				Hostname:    "host2",
				Nodename:    "node2",
				MachineType: "worker",
			},
		}

		require.NoError(t, client2.SetLocalData(affiliate2PB, nil))

		// both clients should eventually discover each other

		for {
			t.Logf("client1 affiliates = %d", len(client1.GetAffiliates()))

			if len(client1.GetAffiliates()) == 1 {
				break
			}

			select {
			case <-notify1:
			case <-time.After(2 * time.Second):
				t.Logf("client1 affiliates on timeout = %d", len(client1.GetAffiliates()))

				require.Fail(t, "no incremental update")
			}
		}

		require.Len(t, client1.GetAffiliates(), 1)

		assert.Equal(t, []*client.Affiliate{affiliate2PB}, client1.GetAffiliates())

		for {
			t.Logf("client2 affiliates = %d", len(client1.GetAffiliates()))

			if len(client2.GetAffiliates()) == 1 {
				break
			}

			select {
			case <-notify2:
			case <-time.After(2 * time.Second):
				require.Fail(t, "no incremental update")
			}
		}

		require.Len(t, client2.GetAffiliates(), 1)

		assert.Equal(t, []*client.Affiliate{affiliate1PB}, client2.GetAffiliates())

		// update affiliate1, client2 should see the update
		affiliate1PB.Endpoints = []*clientpb.Endpoint{
			{
				Ip:   []byte{1, 2, 3, 4},
				Port: 5678,
			},
		}
		require.NoError(t, client1.SetLocalData(affiliate1PB, nil))

		for {
			select {
			case <-notify2:
			case <-time.After(time.Second):
				require.Fail(t, "no incremental update")
			}

			if len(client2.GetAffiliates()[0].Endpoints) == 1 {
				break
			}
		}

		assert.Equal(t, []*client.Affiliate{affiliate1PB}, client2.GetAffiliates())

		cancel()

		err = eg.Wait()
		if err != nil && !errors.Is(err, context.Canceled) {
			assert.NoError(t, err)
		}
	})

	t.Run("AffiliateExpire", func(t *testing.T) {
		t.Parallel()

		clusterID := "cluster_2"

		key := make([]byte, 32)
		_, err := io.ReadFull(rand.Reader, key)
		require.NoError(t, err)

		cipher, err := aes.NewCipher(key)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		affiliate1 := "af_1"
		affiliate2 := "af_2"

		client1, err := client.NewClient(client.Options{
			Cipher:      cipher,
			Endpoint:    endpoint,
			ClusterID:   clusterID,
			AffiliateID: affiliate1,
			TTL:         time.Second,
			Insecure:    true,
		})
		require.NoError(t, err)

		client2, err := client.NewClient(client.Options{
			Cipher:      cipher,
			Endpoint:    endpoint,
			ClusterID:   clusterID,
			AffiliateID: affiliate2,
			TTL:         time.Minute,
			Insecure:    true,
		})
		require.NoError(t, err)

		notify1 := make(chan struct{}, 1)
		notify2 := make(chan struct{}, 1)

		eg, ctx := errgroup.WithContext(ctx)

		ctx1, cancel1 := context.WithCancel(ctx)
		defer cancel1()

		ctx2, cancel2 := context.WithCancel(ctx)
		defer cancel2()

		eg.Go(func() error {
			return client1.Run(ctx1, logger, notify1)
		})

		eg.Go(func() error {
			return client2.Run(ctx2, logger, notify2)
		})

		select {
		case <-notify1:
		case <-time.After(2 * time.Second):
			require.Fail(t, "no initial snapshot update")
		}

		assert.Empty(t, client1.GetAffiliates())

		select {
		case <-notify2:
		case <-time.After(2 * time.Second):
			require.Fail(t, "no initial snapshot update")
		}

		assert.Empty(t, client2.GetAffiliates())

		// client1 publishes an affiliate with short TTL
		affiliate1PB := &client.Affiliate{
			Affiliate: &clientpb.Affiliate{
				NodeId:      affiliate1,
				Addresses:   [][]byte{{1, 2, 3}},
				Hostname:    "host1",
				Nodename:    "node1",
				MachineType: "controlplane",
			},
		}

		require.NoError(t, client1.SetLocalData(affiliate1PB, nil))

		// client2 should see the update from client1
		for {
			t.Logf("client2 affiliates = %d", len(client2.GetAffiliates()))

			if len(client2.GetAffiliates()) == 1 {
				break
			}

			select {
			case <-notify2:
			case <-time.After(2 * time.Second):
				t.Logf("client2 affiliates on timeout = %d", len(client2.GetAffiliates()))

				require.Fail(t, "no incremental update")
			}
		}

		require.Len(t, client2.GetAffiliates(), 1)

		assert.Equal(t, []*client.Affiliate{affiliate1PB}, client2.GetAffiliates())

		// stop client1
		cancel1()

		for {
			t.Logf("client2 affiliates = %d", len(client2.GetAffiliates()))

			if len(client2.GetAffiliates()) == 0 {
				break
			}

			select {
			case <-notify2:
			case <-time.After(2 * time.Second):
				require.Fail(t, "no expiration")
			}
		}

		require.Len(t, client2.GetAffiliates(), 0)

		cancel()

		err = eg.Wait()
		if err != nil && !errors.Is(err, context.Canceled) {
			assert.NoError(t, err)
		}
	})

	t.Run("Cluster1", func(t *testing.T) {
		clusterSimulator(t, endpoint, logger, 5)
	})

	t.Run("Cluster2", func(t *testing.T) {
		clusterSimulator(t, endpoint, logger, 15)
	})

	t.Run("Cluster3", func(t *testing.T) {
		clusterSimulator(t, endpoint, logger, 50)
	})
}

// clusterSimulator simulates cluster with a number of affiliates discovering each other.
//
//nolint:gocognit
func clusterSimulator(t *testing.T, endpoint string, logger *zap.Logger, numAffiliates int) {
	clusterIDBytes := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, clusterIDBytes)
	require.NoError(t, err)

	cluterID := base64.StdEncoding.EncodeToString(clusterIDBytes)

	key := make([]byte, 32)
	_, err = io.ReadFull(rand.Reader, key)
	require.NoError(t, err)

	cipher, err := aes.NewCipher(key)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	affiliates := make([]*client.Client, numAffiliates)

	for i := range affiliates {
		affiliates[i], err = client.NewClient(client.Options{
			Cipher:        cipher,
			Endpoint:      endpoint,
			ClusterID:     cluterID,
			AffiliateID:   fmt.Sprintf("affiliate-%d", i),
			ClientVersion: "v0.0.1",
			TTL:           10 * time.Second,
			Insecure:      true,
		})
		require.NoError(t, err)
	}

	notifyCh := make([]chan struct{}, numAffiliates)

	for i := range notifyCh {
		notifyCh[i] = make(chan struct{}, 1)
	}

	eg, ctx := errgroup.WithContext(ctx)

	for i := range affiliates {
		i := i

		eg.Go(func() error {
			return affiliates[i].Run(ctx, logger, notifyCh[i])
		})
	}

	// establish data for each affiliate
	for i := range affiliates {
		require.NoError(t, affiliates[i].SetLocalData(&client.Affiliate{
			Affiliate: &clientpb.Affiliate{
				NodeId:   fmt.Sprintf("affiliate-%d", i),
				Hostname: fmt.Sprintf("%d", i),
			},
			Endpoints: []*clientpb.Endpoint{
				{
					Ip:   make([]byte, 4), // IPv4
					Port: uint32((i + 1) * 10),
				},
			},
		}, []client.Endpoint{
			{
				AffiliateID: fmt.Sprintf("affiliate-%d", (i+1)%numAffiliates),
				Endpoints: []*clientpb.Endpoint{
					{
						Ip:   make([]byte, 16), // IPv6
						Port: uint32(((i+1)%numAffiliates + 1) * 100),
					},
				},
			},
		}))
	}

	checkDiscoveredState := func(affiliateID int, discovered []*client.Affiliate) error {
		if len(discovered) != numAffiliates-1 {
			return fmt.Errorf("discovered count %d != expected %d", len(discovered), numAffiliates-1)
		}

		expected := make(map[int]struct{})

		for i := 0; i < numAffiliates; i++ {
			if i != affiliateID {
				expected[i] = struct{}{}
			}
		}

		for _, affiliate := range discovered {
			var thisID int

			thisID, err = strconv.Atoi(affiliate.Affiliate.Hostname)

			require.NoError(t, err)

			delete(expected, thisID)

			// each affiliate should have two endpoints: one coming from itself, another from different affiliate
			if len(affiliate.Endpoints) != 2 {
				return fmt.Errorf("expected 2 endpoints, got %d", len(affiliate.Endpoints))
			}

			ports := []int{
				int(affiliate.Endpoints[0].Port),
				int(affiliate.Endpoints[1].Port),
			}

			sort.Ints(ports)

			expectedPorts := []int{
				(thisID + 1) * 10,
				(thisID + 1) * 100,
			}

			if !reflect.DeepEqual(expectedPorts, ports) {
				return fmt.Errorf("expected ports %v, got %v", expectedPorts, ports)
			}
		}

		if len(expected) > 0 {
			return fmt.Errorf("some affiliates not discovered: %v", expected)
		}

		return nil
	}

	// eventually all affiliates should see discovered state
	const NumAttempts = 50 // 50 * 100ms = 5s

	for j := 0; j < NumAttempts; j++ {
		matches := true

		for i := range affiliates {
			discovered := affiliates[i].GetAffiliates()

			if err = checkDiscoveredState(i, discovered); err != nil {
				t.Log(err)

				matches = false

				break
			}
		}

		if matches {
			break
		}

		if j == NumAttempts-1 {
			assert.Fail(t, "state not converged")
		}

		time.Sleep(100 * time.Millisecond)
	}

	cancel()

	err = eg.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		assert.NoError(t, err)
	}
}
