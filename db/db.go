package db

import (
	"fmt"
	"sync"

	"github.com/talos-systems/wglan-manager/types"
)

type DB interface {
   // Add adds a set of known Endpoints to a node, creating the node, if it does not exist.
   Add(cluster string, n *types.Node) error

	// AddKnownEndpoints adds a set of known-good endpoints for a node.
	AddKnownEndpoints(cluster string, id string, ep ...*types.KnownEndpoint) error

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

   existingNode, ok := c[n.ID]
   if !ok {
      c[n.ID] = n
		return nil
   }

	var found bool

	existingNode.Name = n.Name
	existingNode.ID = n.ID
	existingNode.IP = n.IP

   for _, ep := range n.KnownEndpoints {
		found = false

      for _, existing := range existingNode.KnownEndpoints {
         if existing == ep {
            found = true
            break
         }
      }

      if !found {
         existingNode.KnownEndpoints = append(existingNode.KnownEndpoints, ep)
      }
   }

   return nil
}

func (d *ramDB) AddKnownEndpoints(cluster string, id string, knownEndpoints ...*types.KnownEndpoint) error {
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

	for _, ep := range knownEndpoints {
		var found bool

		for _, existing := range n.KnownEndpoints {
			if ep.Endpoint == existing.Endpoint {
				found = true

				existing.LastConnected = ep.LastConnected

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
