// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package client implements http client for the manager API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/talos-systems/kubespan-manager/pkg/types"
)

// Add adds a Node to the Cluster database, updating it if it already exists.
func Add(rootURL, clusterID string, n *types.Node) error {
	body := new(bytes.Buffer)

	if err := json.NewEncoder(body).Encode(n); err != nil {
		return fmt.Errorf("failed to encode Node information: %w", err)
	}

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, fmt.Sprintf("%s/%s", rootURL, clusterID), body)
	if err != nil {
		return fmt.Errorf("failed to post Node information: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to post Node information: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode > 299 {
		return fmt.Errorf("server rejected Node information: %s", resp.Status)
	}

	return nil
}

// AddAddresses adds a list of addresses to a node.
func AddAddresses(rootURL, clusterID, id string, epList ...*types.Address) error {
	buf := new(bytes.Buffer)

	if err := json.NewEncoder(buf).Encode(epList); err != nil {
		return fmt.Errorf("failed to encode known endpoints: %w", err)
	}

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPut, fmt.Sprintf("%s/%s/%s", rootURL, clusterID, id), buf)
	if err != nil {
		return fmt.Errorf("failed to make PUT request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add endpoints to node %q/%q: %w", clusterID, id, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode > 299 {
		return fmt.Errorf("server rejected new endpoints for node %q/%q: %s", clusterID, id, resp.Status)
	}

	return nil
}

// Get returns the Node defined by the given public key, if and only if it exists within the given Cluster ID.
func Get(rootURL, clusterID, publicKey string) (*types.Node, error) {
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, fmt.Sprintf("%s/%s/%s", rootURL, clusterID, publicKey), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to request node %q/%q from server %q: %w", clusterID, publicKey, rootURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request node %q/%q from server %q: %w", clusterID, publicKey, rootURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	node := new(types.Node)
	if err = json.NewDecoder(resp.Body).Decode(node); err != nil {
		return nil, fmt.Errorf("failed to decode response from server: %w", err)
	}

	return node, nil
}

// List returns the set of Nodes associated with the given Cluster ID.
func List(rootURL, clusterID string) ([]*types.Node, error) {
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, fmt.Sprintf("%s/%s", rootURL, clusterID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to request list from server %q: %w", rootURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request list from server %q: %w", rootURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var list []*types.Node
	if err = json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("failed to decode response from server: %w", err)
	}

	return list, nil
}
