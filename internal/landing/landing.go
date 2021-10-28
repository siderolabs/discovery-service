// Copyright (c) 2021 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

// Package landing provides the HTML landing page.
package landing

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed "html/index.html"
var static embed.FS

// Handler returns static landing page handler.
func Handler() http.Handler {
	subfs, err := fs.Sub(static, "html")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(subfs)))

	return mux
}
