// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package protomux_test

import (
	"net/http"
	"strings"
	"testing"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/discovery-service/internal/protomux"
)

func TestMetrics(t *testing.T) {
	t.Parallel()

	metrics := protomux.NewMetrics()

	problems, err := promtestutil.CollectAndLint(metrics)
	require.NoError(t, err)
	require.Empty(t, problems)

	// the series exist before any connection is made, so that rate() works from the start
	assert.NotZero(t, promtestutil.CollectAndCount(metrics))

	httpState := protomux.HTTPConnState(metrics)

	// one HTTP connection, still open (the gRPC side is counted by the Mux, and covered by
	// TestConnectionMetrics)
	httpState(nil, http.StateNew)
	httpState(nil, http.StateActive)

	expected := `
# HELP discovery_connections_active The current number of connections on the main listener by protocol.
# TYPE discovery_connections_active gauge
discovery_connections_active{protocol="grpc"} 0
discovery_connections_active{protocol="http"} 1
# HELP discovery_connections_opened_total Number of connections accepted on the main listener by protocol.
# TYPE discovery_connections_opened_total counter
discovery_connections_opened_total{protocol="grpc"} 0
discovery_connections_opened_total{protocol="http"} 1
# HELP discovery_connections_closed_total Number of connections closed on the main listener by protocol.
# TYPE discovery_connections_closed_total counter
discovery_connections_closed_total{protocol="grpc"} 0
discovery_connections_closed_total{protocol="http"} 0
`

	require.NoError(t, promtestutil.CollectAndCompare(metrics, strings.NewReader(expected),
		"discovery_connections_opened_total", "discovery_connections_closed_total", "discovery_connections_active"))

	// the HTTP connection goes away
	httpState(nil, http.StateClosed)

	expectedAfterClose := `
# HELP discovery_connections_active The current number of connections on the main listener by protocol.
# TYPE discovery_connections_active gauge
discovery_connections_active{protocol="grpc"} 0
discovery_connections_active{protocol="http"} 0
`

	require.NoError(t, promtestutil.CollectAndCompare(metrics, strings.NewReader(expectedAfterClose), "discovery_connections_active"))
}
