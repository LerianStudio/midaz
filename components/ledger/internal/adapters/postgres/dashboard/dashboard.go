// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package dashboard answers the ledger's three dashboard reads. It aggregates
// at READ TIME over the tables the ledger already writes — no rollup table, no
// materialised view, no backfill story — which is affordable because every
// figure it publishes is bounded by a window index the schema already carries
// (idx_transaction_date_range, migration 000029) or by account cardinality
// rather than history (the balance table).
package dashboard

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// Repository is the port the dashboard reads go through.
//
// It is stated as an interface because TWO things implement it: the postgres
// repository below and the Valkey read-through cache in
// internal/adapters/redis/dashboard, which decorates it. Bootstrap substitutes
// one for the other, so nothing above this line learns whether an answer was
// computed or served from cache.
//
// Every method is scoped to one organization and one ledger. There is no
// ledger-wide or tenant-wide variant, and adding one would be a new port, not
// a nil argument: the scope is what bounds the read.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=dashboard.postgresql_mock.go --package=dashboard . Repository
type Repository interface {
	// Metrics returns the window's transaction count, its breakdown by status,
	// and the settled volume per asset.
	Metrics(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardMetrics, error)

	// Volume returns one point per UTC calendar day the window touches, days
	// with no transactions included and zero-valued.
	Volume(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardVolume, error)

	// Assets returns the ledger's current position per asset. It takes no
	// window: a balance is a running total, not an aggregate over history.
	Assets(ctx context.Context, organizationID, ledgerID uuid.UUID) (*mmodel.DashboardAssets, error)
}
