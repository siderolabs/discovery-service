package db

import (
	"fmt"
	"sync"

	"github.com/talos-systems/wglan-manager/types"
)

type DB interface {
   // AddEndpoints adds a set of known Endpoints to a node, creating the node, if it does not exist.
   AddEndpoints(cluster, id string, ep []string) error

   // Get returns the details of the node.
   Get(cluster, id string) (*types.Node,error)

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

// AddEndpoints implements DB
func (d *ramDB) AddEndpoints(cluster, id string, endpoints []string) error {
   d.mu.Lock()
   defer d.mu.Unlock()

	c, ok := d.db[cluster]
	if !ok {
		c = make(map[string]*types.Node)
		d.db[cluster] = c
	}

   n, ok := c[id]
   if !ok {
      n = &types.Node{
         ID: id,
      }

      c[id] = n
   }

   var found bool
   for _, ep := range endpoints {
      found = false

      for _, existing := range n.KnownEndpoints {
         if existing == ep {
            found = true
            break
         }
      }

      if !found {
         n.KnownEndpoints = append(n.KnownEndpoints, ep)
      }
   }

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
func (d *ramDB) Get(cluster, id string) (*types.Node,error) {
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
