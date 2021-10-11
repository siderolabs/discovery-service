// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

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
