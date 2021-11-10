// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package landing provides the HTML landing page.
package landing

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"github.com/talos-systems/discovery-service/internal/state"
)

//go:embed "html/index.html"
var static embed.FS

//go:embed "templates/inspect.html.tmpl"
var inspectTemplate []byte

// ClusterInspectData represents all affiliate data asssociated with a cluster.
type ClusterInspectData struct {
	ClusterID  string
	Affiliates []*state.AffiliateExport
}

var inspectPage = template.Must(template.New("inspect").Parse(string(inspectTemplate)))

// InspectHandler provides data to inspect a cluster.
func InspectHandler(w http.ResponseWriter, req *http.Request, state *state.State) error {
	clusterData := ClusterInspectData{}

	formData, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		return err
	}

	clusterData.ClusterID = formData.Get("clusterID")
	fetchedCluster := state.GetCluster(clusterData.ClusterID)
	clusterData.Affiliates = fetchedCluster.List()

	if err := inspectPage.Execute(w, clusterData); err != nil {
		return err
	}

	return nil
}

// Handler returns static landing page handler.
func Handler(state *state.State, logger *zap.Logger) http.Handler {
	subfs, err := fs.Sub(static, "html")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.FS(subfs)))

	// wrapper to pass state
	mux.HandleFunc("/inspect", func(w http.ResponseWriter, r *http.Request) {
		if err := InspectHandler(w, r, state); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Oops, try again")
			logger.Error("failed to return the page:", zap.Error(err))
		}
	})

	return mux
}
