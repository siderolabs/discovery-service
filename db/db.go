package db

import (
	"sync"

	"github.com/talos-systems/wglan-manager/types"
)

type DB interface {
   // AddEndpoints adds a set of known Endpoints to a node, creating the node, if it does not exist.
   AddEndpoints(id string, ep []string) error

   // Get returns the details of the node.
   Get(id string) *types.Node
}

type ramDB struct {
   db map[string]*types.Node
   mu sync.RWMutex
}

// New returns a new database.
func New() DB {
   return &ramDB{
      db: make(map[string]*types.Node),
   }
}

// AddEndpoints implements DB
func (d *ramDB) AddEndpoints(id string, endpoints []string) error {
   d.mu.Lock()
   defer d.mu.Unlock()

   n, ok := d.db[id]
   if !ok {
      n = &types.Node{
         ID: id,
      }

      d.db[id] = n
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

// Get implements DB
func (d *ramDB) Get(id string) *types.Node {
   d.mu.RLock()
   defer d.mu.Unlock()

   n, ok := d.db[id]
   if !ok {
      return nil
   }

   return n
}
