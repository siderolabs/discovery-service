package db

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/talos-systems/wglan-manager/types"
	"go.uber.org/zap"
)

// AddressExpirationTimeout is the amount of time after which addresses of a node should be expired.
const AddressExpirationTimeout = 10 * time.Minute

type DB interface {
	// Add adds a set of known Endpoints to a node, creating the node, if it does not exist.
	Add(ctx context.Context, cluster string, n *types.Node) error

	// AddAddresses adds a set of addresses for a node.
	AddAddresses(ctx context.Context, cluster string, id string, ep ...*types.Address) error

	// Clean executes a database cleanup routine.
	Clean()

	// Get returns the details of the node.
	Get(ctx context.Context, cluster string, id string) (*types.Node, error)

	// List returns the set of Nodes for the given Cluster.
	List(ctx context.Context, cluster string) ([]*types.Node, error)
}

type ramDB struct {
	logger *zap.Logger
	db map[string]map[string]*types.Node
	mu sync.RWMutex
}

// New returns a new database.
func New(logger *zap.Logger) DB {
	return &ramDB{
		logger: logger,
		db: make(map[string]map[string]*types.Node),
	}
}

// Add implements DB
func (d *ramDB) Add(ctx context.Context, cluster string, n *types.Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	c, ok := d.db[cluster]
	if !ok {
		c = make(map[string]*types.Node)
		d.db[cluster] = c
	}

	if existing, ok := c[n.ID]; ok {
		existing.AddAddresses(n.Addresses...)

		return nil
	}

	c[n.ID] = n

	return nil
}

func (d *ramDB) AddAddresses(ctx context.Context, cluster string, id string, addresses ...*types.Address) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	c, ok := d.db[cluster]
	if !ok {
		return fmt.Errorf("cluster does not exist")
	}

	n, ok := c[id]
	if !ok {
		return fmt.Errorf("node does not exist")
	}

	n.AddAddresses(addresses...)

	return nil
}

func (d *ramDB) List(ctx context.Context, cluster string) (list []*types.Node, err error) {
	c, ok := d.db[cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %q not found", cluster)
	}

	for _, n := range c {
		n.ExpireAddressesOlderThan(AddressExpirationTimeout)

		if len(n.Addresses) > 0 {
			list = append(list, n)
		}
	}

	return list, nil
}

// Get implements DB
func (d *ramDB) Get(ctx context.Context, cluster string, id string) (*types.Node, error) {
	d.mu.RLock()
	defer d.mu.Unlock()

	c, ok := d.db[cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %q not found", cluster)
	}

	n, ok := c[id]
	if !ok {
		return nil, fmt.Errorf("node %q in cluster %q not found", id, cluster)
	}

	return n, nil
}

// Clean runs the database cleanup routine.
func (d *ramDB) Clean() {
	d.mu.Lock()
	defer d.mu.Unlock()

	var clusterDeleteList []string

	for clusterID, c := range d.db {
		var nodeDeleteList []string

		for id, n := range c {

			n.ExpireAddressesOlderThan(AddressExpirationTimeout)

			if len(n.Addresses) < 1 {
				nodeDeleteList = append(nodeDeleteList, id)
			}
		}

		for _, id := range nodeDeleteList {
			c[id] = nil
			delete(c, id)
		}

		if len(c) < 0 {
			clusterDeleteList = append(clusterDeleteList, clusterID)
		}
	}

	for _, id := range clusterDeleteList {
		delete(d.db, id)
	}
}
