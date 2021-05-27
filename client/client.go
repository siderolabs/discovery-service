package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/talos-systems/wglan-manager/types"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Add adds a Node to the Cluster database, updating it if it already exists.
func Add(rootURL, clusterID string, n *types.Node) error {

	body := new(bytes.Buffer)

	if err := json.NewEncoder(body).Encode(n); err != nil {
		return fmt.Errorf("failed to encode Node information: %w", err)
	}

	resp, err := http.Post(fmt.Sprintf("%s/%s", rootURL, clusterID), "application/json", body); if err != nil {
		return fmt.Errorf("failed to post Node information: %w", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	if resp.StatusCode > 299 {
		return fmt.Errorf("server rejected Node information: %s", resp.Status)
	}

	return nil
}

// AddKnownEndpoints adds a list of known-good endpoints to a node.
func AddKnownEndpoints(rootURL, clusterID string, id wgtypes.Key, epList ... *types.KnownEndpoint) error {
	buf := new(bytes.Buffer)

	if err := json.NewEncoder(buf).Encode(epList); err != nil {
		return fmt.Errorf("failed to encode known endpoints: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s/%s", rootURL, clusterID, id.String()), buf)
	if err != nil {
		return fmt.Errorf("failed to make PUT request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add endpoints to node %q/%q: %w", clusterID, id.String(), err)
	}

	if resp.StatusCode > 299 {
		return fmt.Errorf("server rejected new endpoints for node %q/%q: %s", clusterID, id.String(), resp.Status)
	}

	return nil

}

// Get returns the Node defined by the given public key, if and only if it exists within the given Cluster ID.
func Get(rootURL, clusterID string, publicKey wgtypes.Key) (*types.Node, error) {

	resp, err := http.Get(fmt.Sprintf("%s/%s/%s", rootURL, clusterID, publicKey.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to request node %q/%q from server %q: %w", clusterID, publicKey, rootURL, err)
	}

	node := new(types.Node)
	if err = json.NewDecoder(resp.Body).Decode(node); err != nil {
		return nil, fmt.Errorf("failed to decode response from server: %w", err)
	}

	return node, nil
}

// List returns the set of Nodes associated with the given Cluster ID.
func List(rootURL string, clusterID string) ([]*types.Node,error) {

	resp, err := http.Get(fmt.Sprintf("%s/%s", rootURL, clusterID))
	if err != nil {
		return nil, fmt.Errorf("failed to request list from server %q: %w", rootURL, err)
	}
	defer resp.Body.Close() // nolint: errcheck

	var list []*types.Node
	if err = json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("failed to decode response from server: %w", err)
	}

	return list, nil

}

