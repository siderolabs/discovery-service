// Copyright (c) 2026 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package protomux

import (
	"net"
	"net/http"

	prom "github.com/prometheus/client_golang/prometheus"
)

// Protocol is the protocol a connection was routed by.
type Protocol int

// Connection protocols reported by the metrics.
const (
	ProtocolGRPC Protocol = iota
	ProtocolHTTP

	numProtocols
)

// protocolLabels are the label values for each protocol.
var protocolLabels = [numProtocols]string{
	ProtocolGRPC: "grpc",
	ProtocolHTTP: "http",
}

// String implements fmt.Stringer interface.
func (protocol Protocol) String() string {
	return protocolLabels[protocol]
}

// Metrics tracks the lifecycle of the connections accepted on the main listener.
//
// The gRPC request metrics can't tell whether a request arrived over a fresh connection or an
// existing one: clients keep a single connection with a long-lived Watch stream, so a rising
// connection rate means clients are reconnecting rather than that they are doing more work.
type Metrics struct {
	mOpened   *prom.CounterVec
	mClosed   *prom.CounterVec
	mRejected prom.Counter
	mActive   *prom.GaugeVec

	// perProtocol holds the label children resolved once, so that the per-connection hot
	// path doesn't pay for a label lookup under the vector lock.
	perProtocol [numProtocols]protocolMetrics
}

type protocolMetrics struct {
	opened prom.Counter
	closed prom.Counter
	active prom.Gauge
}

// NewMetrics creates the connection metrics.
func NewMetrics() *Metrics {
	metrics := &Metrics{
		mOpened: prom.NewCounterVec(prom.CounterOpts{
			Name: "discovery_connections_opened_total",
			Help: "Number of connections accepted on the main listener by protocol.",
		}, []string{"protocol"}),
		mClosed: prom.NewCounterVec(prom.CounterOpts{
			Name: "discovery_connections_closed_total",
			Help: "Number of connections closed on the main listener by protocol.",
		}, []string{"protocol"}),
		mRejected: prom.NewCounter(prom.CounterOpts{
			Name: "discovery_connections_rejected_total",
			Help: "Number of connections which never reached a server: failed TLS handshake or protocol detection, or accepted during shutdown.",
		}),
		mActive: prom.NewGaugeVec(prom.GaugeOpts{
			Name: "discovery_connections_active",
			Help: "The current number of connections on the main listener by protocol.",
		}, []string{"protocol"}),
	}

	// resolving the label values here also makes the series exist from the start
	for protocol, label := range protocolLabels {
		metrics.perProtocol[protocol] = protocolMetrics{
			opened: metrics.mOpened.WithLabelValues(label),
			closed: metrics.mClosed.WithLabelValues(label),
			active: metrics.mActive.WithLabelValues(label),
		}
	}

	return metrics
}

// connOpened reports a new connection handed over to a server.
func (metrics *Metrics) connOpened(protocol Protocol) {
	metrics.perProtocol[protocol].opened.Inc()
	metrics.perProtocol[protocol].active.Inc()
}

// connClosed reports a connection which is gone.
func (metrics *Metrics) connClosed(protocol Protocol) {
	metrics.perProtocol[protocol].closed.Inc()
	metrics.perProtocol[protocol].active.Dec()
}

// connRejected reports a connection which never made it to a server.
func (metrics *Metrics) connRejected() {
	metrics.mRejected.Inc()
}

// HTTPConnState reports the HTTP connection lifecycle into the metrics.
//
// Meant to be used as http.Server.ConnState; the gRPC connections are counted by the Mux, which
// hands them over.
func HTTPConnState(metrics *Metrics) func(net.Conn, http.ConnState) {
	return func(_ net.Conn, state http.ConnState) {
		switch state { //nolint:exhaustive // only the terminal states are of interest
		case http.StateNew:
			metrics.connOpened(ProtocolHTTP)
		case http.StateClosed, http.StateHijacked:
			metrics.connClosed(ProtocolHTTP)
		}
	}
}

// Describe implements prom.Collector interface.
func (metrics *Metrics) Describe(ch chan<- *prom.Desc) {
	prom.DescribeByCollect(metrics, ch)
}

// Collect implements prom.Collector interface.
func (metrics *Metrics) Collect(ch chan<- prom.Metric) {
	metrics.mOpened.Collect(ch)
	metrics.mClosed.Collect(ch)
	metrics.mActive.Collect(ch)

	ch <- metrics.mRejected
}

// Check interfaces.
var (
	_ prom.Collector = (*Metrics)(nil)
)
