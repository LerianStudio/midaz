// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// argSpy records the arguments each query was issued with. It answers with a
// closed *sql.Rows-shaped failure, because this test is about what is SENT,
// not about what comes back.
type argSpy struct{ args map[string][]any }

func (s *argSpy) GetDB(context.Context) (pgdb.DB, error) { return s, nil }

func (s *argSpy) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, sql.ErrConnDone
}

func (s *argSpy) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	s.args[query] = args

	return nil, sql.ErrConnDone
}

func (s *argSpy) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

// TestDashboardWindowedQueriesForceACustomPlan is a performance regression
// test, and it is here because the regression it guards is invisible.
//
// Dropping the exec-mode argument breaks nothing: every test still passes,
// every row is still correct, and the service is still fast — for five
// executions per connection. Then PostgreSQL switches the cached prepared
// statement to a generic plan built without the window's selectivity, and
// /top-rules over 90 days goes from 95 ms to 323 ms, past its budget, with no
// code change to blame and nothing in a test run to show it.
//
// So the argument itself is asserted. See windowPlanMode for the measurements.
func TestDashboardWindowedQueriesForceACustomPlan(t *testing.T) {
	t.Parallel()

	spy := &argSpy{args: map[string][]any{}}
	repo := NewDashboardRepository(spy, clock.New())
	window := model.DashboardWindow{}
	ctx := context.Background()

	// Each read fails at the spy, which is the point: the arguments are already
	// recorded by then.
	_, _ = repo.Metrics(ctx, window)
	_, _ = repo.Volume(ctx, window)
	_, _ = repo.FraudTypes(ctx, window)
	_, _ = repo.TopRules(ctx, window)

	for name, query := range map[string]string{
		"metrics":     metricsQuery,
		"volume":      volumeQuery,
		"fraud-types": fraudTypesQuery,
		"top-rules":   topRulesQuery,
	} {
		t.Run(name, func(t *testing.T) {
			args, ok := spy.args[query]
			require.Truef(t, ok, "the %s read never issued its query; this test is measuring nothing", name)
			require.NotEmptyf(t, args, "the %s query was issued with no arguments at all", name)

			assert.Equalf(t, pgx.QueryExecModeExec, args[0],
				"the %s read must pass windowPlanMode first, or PostgreSQL serves it a generic plan "+
					"after five executions per connection and it silently gets several times slower", name)
		})
	}

	// activeCountsQuery takes no parameters, so no plan can be built around
	// them and it needs no mode. Asserting that keeps the rule honest: it is
	// "every query whose plan depends on a parameter", not "every query".
	assert.NotContains(t, spy.args, activeCountsQuery,
		"activeCountsQuery is parameterless and is issued through QueryRowContext, not QueryContext")
}
