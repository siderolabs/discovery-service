package db

import (
	"fmt"
	"sync"

	"github.com/talos-systems/wglan-manager/types"
)

type DB interface {
   // Add adds a set of known Endpoints to a node, creating the node, if it does not exist.
   Add(cluster string, n *types.Node) error

	// AddAddresses adds a set of addresses for a node.
	AddAddresses(cluster string, id string, ep ...*types.Address) error

   // Get returns the details of the node.
   Get(cluster string, id string) (*types.Node,error)

	// List returns the set of Nodes for the given Cluster.
	List(cluster string) ([]*types.Node,error)
}

type ramDB struct {
   db map[string]map[string]*types.Node
   mu sync.RWMutex
}

// New returns a new database.
func New() DB {
   return &ramDB{
      db: make(map[string]map[string]*types.Node),
   }
}

// Add implements DB
func (d *ramDB) Add(cluster string, n *types.Node) error {
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

func (d *ramDB) AddAddresses(cluster string, id string, addresses ...*types.Address) error {
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

func (d *ramDB) List(cluster string) (list []*types.Node, err error) {
	c, ok := d.db[cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %q not found", cluster)
	}

	var i int

	list = make([]*types.Node, len(c))

	for _, n := range c {
		list[i] = n

		i++
	}

	return list, nil
}

// Get implements DB
func (d *ramDB) Get(cluster string, id string) (*types.Node,error) {
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
