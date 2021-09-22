// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package server

import (
	prom "github.com/prometheus/client_golang/prometheus"
)

var metricVersionGauge = prom.NewGaugeVec(prom.GaugeOpts{
	Name: "discovery_hello_requests",
	Help: "Number of hello requests split by client version.",
}, []string{"client_version"})

func init() {
	prom.MustRegister(metricVersionGauge)
}
