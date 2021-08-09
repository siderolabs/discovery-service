// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"github.com/talos-systems/kubespan-manager/pkg/types"
)

const redisTTL = 12 * time.Minute

type redisDB struct {
	logger *zap.Logger

	rc *redis.Client
}

// NewRedis creates new redis DB.
func NewRedis(addr string, logger *zap.Logger) (DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rc := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := rc.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &redisDB{
		rc:     rc,
		logger: logger,
	}, nil
}

func (d *redisDB) clusterNodesKey(cluster string) string {
	return fmt.Sprintf("cluster:%s:nodelist", cluster)
}

func (d *redisDB) clusterNodeKey(cluster, id string) string {
	return fmt.Sprintf("cluster:%s:node:%s", cluster, id)
}

func (d *redisDB) clusterAddressKey(cluster string, addr *types.Address) string {
	if !addr.IP.IsZero() {
		return fmt.Sprintf("cluster:%s:address:%s", cluster, addr.IP.String())
	}

	return fmt.Sprintf("cluster:%s:address:%s", cluster, addr.Name)
}

// Add implements db.DB.
func (d *redisDB) Add(ctx context.Context, cluster string, n *types.Node) error {
	tx := d.rc.TxPipeline()

	// Store the node data
	tx.Set(ctx, d.clusterNodeKey(cluster, n.ID), n, redisTTL)

	// Add the node to the cluster
	if err := tx.SAdd(ctx, d.clusterNodesKey(cluster), n.ID).Err(); err != nil {
		return fmt.Errorf("failed to add node %s to cluster %q: %w", n.Name, cluster, err)
	}

	tx.Expire(ctx, d.clusterNodesKey(cluster), redisTTL)

	// Update the address assignments
	for _, addr := range n.Addresses {
		tx.Set(ctx, d.clusterAddressKey(cluster, addr), n.ID, redisTTL)
	}

	_, err := tx.Exec(ctx)

	return err
}

// AddAddresses implements db.DB.
func (d *redisDB) AddAddresses(ctx context.Context, cluster, id string, ep ...*types.Address) error {
	n, err := d.Get(ctx, cluster, id)
	if err != nil {
		return fmt.Errorf("failed to retrieve node %q from cluster %q: %w", id, cluster, err)
	}

	n.AddAddresses(ep...)

	return d.Add(ctx, cluster, n)
}

// Clean implements db.DB.
func (d *redisDB) Clean() {} // no-op

// Get implements db.DB.
func (d *redisDB) Get(ctx context.Context, cluster, id string) (*types.Node, error) {
	n := new(types.Node)

	if err := d.rc.Get(ctx, d.clusterNodeKey(cluster, id)).Scan(n); err != nil {
		if errors.Is(redis.Nil, err) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to parse node %q of cluster %q: %w", id, cluster, err)
	}

	var validAddresses []*types.Address

	for _, a := range n.Addresses {
		owner, err := d.rc.Get(ctx, d.clusterAddressKey(cluster, a)).Result()
		if err == nil && owner == id {
			validAddresses = append(validAddresses, a)
		}
	}

	n.Addresses = validAddresses

	return n, nil
}

// List implements db.DB.
func (d *redisDB) List(ctx context.Context, cluster string) ([]*types.Node, error) {
	nodeList, err := d.rc.SMembers(ctx, d.clusterNodesKey(cluster)).Result()
	if err != nil {
		if errors.Is(redis.Nil, err) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to get members of cluster %q: %w", cluster, err)
	}

	ret := make([]*types.Node, 0, len(nodeList))

	for _, id := range nodeList {
		n, err := d.Get(ctx, cluster, id)
		if err != nil {
			if errors.Is(redis.Nil, err) {
				d.logger.Debug("removing stale node from cluster",
					zap.String("node", id),
					zap.String("cluster", cluster),
				)

				if err = d.rc.SRem(ctx, d.clusterNodesKey(cluster), id).Err(); err != nil {
					d.logger.Warn("failed to remove node from cluster set which did not exist",
						zap.String("node", id),
						zap.String("cluster", cluster),
						zap.Error(err),
					)
				}
			} else {
				d.logger.Error("failed to get node listen in nodeList",
					zap.String("node", id),
					zap.String("cluster", cluster),
					zap.Error(err),
				)
			}

			continue
		}

		ret = append(ret, n)
	}

	if len(ret) == 0 {
		return nil, ErrNotFound
	}

	return ret, nil
}
