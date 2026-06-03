// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// Metrics holds the OTel instruments emitted by the Fetcher HTTP client.
//
// F3 (defensive auth retry) introduces AuthRetries to track 401-driven retries.
// All instruments carry their description on the OTel metric definition, not on
// the struct field — descriptions live where the metric is created.
type Metrics struct {
	// AuthRetries counts the number of times the fetcher client retried a
	// request after receiving an HTTP 401 from the downstream Fetcher API.
	// Attributes:
	//   - tenant_id (string): tenant whose credential was retried
	//   - endpoint (string): request URL path (e.g., /v1/fetcher)
	//   - outcome (string): "success" (retry returned 2xx) or "failure"
	//     (retry returned non-2xx, including another 401)
	AuthRetries metric.Int64Counter
}

// NewMetrics creates real OTel instruments for the Fetcher client. Use this
// when a real meter provider is wired (multi-tenant mode in production).
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	authRetries, err := meter.Int64Counter("reporter_fetcher_auth_retry_total",
		metric.WithDescription("Number of M2M auth retries triggered by 401 responses from the fetcher API."))
	if err != nil {
		return nil, fmt.Errorf("create reporter_fetcher_auth_retry_total counter: %w", err)
	}

	return &Metrics{
		AuthRetries: authRetries,
	}, nil
}

// NoopMetrics returns a no-op Metrics instance for single-tenant mode, tests,
// or contexts where a meter provider is not available.
func NoopMetrics() *Metrics {
	provider := noop.NewMeterProvider()
	meter := provider.Meter("noop")

	// noop meter never returns errors, so we can safely ignore them.
	authRetries, _ := meter.Int64Counter("reporter_fetcher_auth_retry_total")

	return &Metrics{
		AuthRetries: authRetries,
	}
}
