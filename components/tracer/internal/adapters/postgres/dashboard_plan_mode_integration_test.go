// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
)

// TestDashboardReadsNeverGetAGenericPlan_Integration is the behavioural half of
// the plan-mode guard.
//
// dashboard_plan_mode_test.go asserts the exec-mode ARGUMENT is passed, which
// is precise but only as durable as pgx's current handling of that argument.
// This asserts the property the argument buys, and asserts it against
// PostgreSQL's own record rather than against a stopwatch.
//
// pg_prepared_statements carries custom_plans and generic_plans per cached
// statement. Run a read more than five times on ONE pooled connection and the
// default exec mode produces a cached statement whose generic_plans climbs
// (measured on the benchmark trail: custom=5 generic=3 after eight
// executions), which is the 3.4x cliff. Under windowPlanMode the statement is
// unnamed, so it is never cached and never planned generically — there is no
// row to find.
//
// A timing assertion was tried first and rejected: on a fixture this small a
// generic plan costs nothing measurable, so the test passed against code with
// the mode removed. Counting plans is data-independent and exact.
func TestDashboardReadsNeverGetAGenericPlan_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	// ONE connection, so every execution reuses the same server-side plan
	// cache. Spread over several connections none would reach six and the test
	// would prove nothing.
	db.SetMaxOpenConns(1)

	window := dashboardWindowAt(t, 12)
	repo := NewDashboardRepository(&testutil.IntegrationDBAdapter{DB: db}, clock.New())
	ctx := context.Background()

	// Six is the first execution PostgreSQL may answer from a generic plan;
	// eight leaves margin.
	for range 8 {
		_, err := repo.Metrics(ctx, window)
		require.NoError(t, err)

		_, err = repo.Volume(ctx, window)
		require.NoError(t, err)

		_, err = repo.FraudTypes(ctx, window)
		require.NoError(t, err)

		_, err = repo.TopRules(ctx, window)
		require.NoError(t, err)
	}

	for _, tc := range []struct{ name, fragment, query string }{
		{"metrics", "GROUPING SETS", metricsQuery},
		{"volume", "AT TIME ZONE", volumeQuery},
		{"fraud-types", "GROUP BY transaction_type", fraudTypesQuery},
		{"top-rules", "CROSS JOIN LATERAL", topRulesQuery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Positive control, and it has to come first. The two assertions
			// below are both "this count is zero", which a subtest that matches
			// NOTHING satisfies perfectly: reword the query — qualify a column,
			// say `GROUP BY v.transaction_type` — and the fragment stops
			// matching, no rows come back, and the subtest passes for ever
			// against code it is supposed to condemn. Proven by rewording this
			// exact fragment against a build with windowPlanMode removed: the
			// other three failed and fraud-types passed.
			require.Containsf(t, tc.query, tc.fragment,
				"the %s fragment no longer occurs in its query: this subtest is asserting nothing", tc.name)

			var (
				cached  int
				generic int
			)

			require.NoError(t, db.QueryRowContext(ctx, `
				SELECT COUNT(*), COALESCE(SUM(generic_plans), 0)
				FROM pg_prepared_statements
				WHERE statement LIKE '%' || $1 || '%'`, tc.fragment).Scan(&cached, &generic))

			assert.Zerof(t, generic,
				"the %s read has been answered from a generic plan %d times: PostgreSQL no longer "+
					"plans it against its window, which is the several-times-slower path windowPlanMode "+
					"exists to prevent", tc.name, generic)

			assert.Zerof(t, cached,
				"the %s read left %d cached prepared statement(s) behind; under windowPlanMode it "+
					"should use an unnamed statement, which is what keeps it out of the generic-plan "+
					"path in the first place", tc.name, cached)
		})
	}
}
