// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package middleware provides HTTP middleware for the Tracer API.
package middleware

import (
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
)

// Metric is an alias for libMetrics.Metric to allow local usage without importing.
type Metric = libMetrics.Metric

// MetricAuthFailures tracks authentication failures by reason.
// Name follows TRD Section 9.3 convention with tracer_ prefix.
// Labels: reason (missing_api_key, invalid_api_key)
var MetricAuthFailures = Metric{
	Name:        "tracer_auth_failures_total",
	Unit:        "1",
	Description: "Total authentication failures by reason",
}
