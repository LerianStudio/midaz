// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=dashboard_repository.go -destination=mocks/dashboard_repository_mock.go -package=mocks

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// DashboardRepository reads the aggregate figures the operator dashboard
// renders. Every method answers ONE bounded aggregation over the validation
// trail for the window it is handed, and returns the figures already shaped
// for the wire.
//
// The interface is deliberately identical for the postgres adapter and the
// Valkey cache decorator: caching is a property of the wiring, not of the
// call site, so the query service cannot accidentally read one endpoint
// through the cache and another around it.
//
// Read-only by construction: the validation trail is append-only (a postgres
// DO INSTEAD NOTHING rule enforces it), and nothing here would be allowed to
// write even if it tried.
type DashboardRepository interface {
	// Metrics returns the headline counters, rates, blocked volume per asset
	// and decision latency for the window, plus the live active rule and limit
	// counts (which are point-in-time, not windowed).
	Metrics(ctx context.Context, window model.DashboardWindow) (*model.DashboardMetrics, error)

	// Volume returns the per-day validation counts across the window.
	Volume(ctx context.Context, window model.DashboardWindow) (*model.DashboardVolume, error)

	// FraudTypes returns the flagged (DENY + REVIEW) breakdown by transaction
	// type across the window.
	FraudTypes(ctx context.Context, window model.DashboardWindow) (*model.DashboardFraudTypes, error)

	// TopRules returns the busiest rules in the window, capped at
	// model.DashboardTopRulesLimit.
	TopRules(ctx context.Context, window model.DashboardWindow) (*model.DashboardTopRules, error)
}
