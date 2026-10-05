// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package services

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_dbsuite"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestMain(m *testing.M) {
	_, filename, _, _ := runtime.Caller(0)

	os.Exit(testutil_dbsuite.SetupTestDBSuite(m,
		testutil_dbsuite.WithMigrations(filepath.Join(filepath.Dir(filename), "..", "..", "migrations")),
	))
}

// TestValidate_ConcurrencyAbovePoolSize_Integration runs more concurrent
// validations than the pool has connections. Every validation holds one
// connection for its transaction, so any lookup inside that transaction that
// asks the pool for a second one deadlocks once all connections are held.
func TestValidate_ConcurrencyAbovePoolSize_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t) // MaxOpenConns = 2
	svc := newPoolTestService(t, db)

	t.Run("distinct request ids", func(t *testing.T) {
		limitID := createGlobalLimit(t, db, "BRL")

		results, errs := validateConcurrently(svc, "BRL", limitID, uuid.New)

		for i := range results {
			require.NoError(t, errs[i], "validation %d", i)
			assert.Equal(t, model.DecisionAllow, results[i].Response.Decision, "validation %d", i)
		}

		assertUsage(t, db, limitID, 10*poolTestConcurrency)
	})

	// The losers of the request-id race hit a unique violation inside their
	// transaction and then look the winner up.
	t.Run("shared request id", func(t *testing.T) {
		limitID := createGlobalLimit(t, db, "USD")
		requestID := uuid.New()

		results, errs := validateConcurrently(svc, "USD", limitID, func() uuid.UUID { return requestID })

		var winners []*ValidateResult

		for i := range results {
			require.NoError(t, errs[i], "validation %d", i)

			if !results[i].IsDuplicate {
				winners = append(winners, results[i])
			}
		}

		require.Len(t, winners, 1)

		for i := range results {
			assert.Equal(t, winners[0].Response.ValidationID, results[i].Response.ValidationID, "validation %d", i)
		}

		assertUsage(t, db, limitID, 10)
	})
}

const poolTestConcurrency = 8

func newPoolTestService(t *testing.T, db *sql.DB) *ValidationService {
	t.Helper()

	conn := &testutil.IntegrationDBAdapter{DB: db}

	evalResult, err := model.NewEvaluationResult(model.DecisionAllow, nil, nil, "no rule matched")
	require.NoError(t, err)

	ruleEval := mocks.NewMockRuleEvaluator(gomock.NewController(t))
	ruleEval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(evalResult, nil).AnyTimes()

	limitChecker, err := query.NewLimitChecker(
		postgres.NewLimitRepositoryWithConnection(conn),
		postgres.NewUsageCounterRepositoryWithConnection(conn),
		clock.RealClock{},
	)
	require.NoError(t, err)

	validationRepo := postgres.NewTransactionValidationRepositoryWithConnection(conn)

	svc, err := NewValidationService(
		pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db))),
		ruleEval,
		limitChecker,
		validationRepo,
		validationRepo,
		command.NewRecordAuditEventCommand(postgres.NewAuditEventRepositoryWithConnection(conn)),
		clock.RealClock{},
	)
	require.NoError(t, err)

	return svc
}

// createGlobalLimit inserts an ACTIVE unscoped DAILY limit on asset. Ids are
// random because validation records cannot be deleted, so a rerun needs fresh ones.
func createGlobalLimit(t *testing.T, db *sql.DB, asset string) uuid.UUID {
	t.Helper()

	limitID := uuid.New()
	_, err := db.Exec(`INSERT INTO limits (id, name, limit_type, max_amount, asset, scopes, status)
		VALUES ($1, $2, 'DAILY', 1000000, $3, '[]', 'ACTIVE')`, limitID, "pool regression "+limitID.String(), asset)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM limits WHERE id = $1`, limitID) })

	return limitID
}

// validateConcurrently runs poolTestConcurrency validations of 10 asset at once
// under one 10s deadline.
func validateConcurrently(svc *ValidationService, asset string, accountID uuid.UUID, requestID func() uuid.UUID) ([]*ValidateResult, []error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := make([]*ValidateResult, poolTestConcurrency)
	errs := make([]error, poolTestConcurrency)

	var wg sync.WaitGroup

	for i := range poolTestConcurrency {
		wg.Add(1)

		go func() {
			defer wg.Done()

			results[i], errs[i] = svc.Validate(ctx, &model.ValidationRequest{
				RequestID:            requestID(),
				TransactionType:      model.TransactionTypeCard,
				Amount:               decimal.NewFromInt(10),
				Asset:                asset,
				TransactionTimestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				Account:              model.AccountContext{ID: accountID},
			})
		}()
	}

	wg.Wait()

	return results, errs
}

func assertUsage(t *testing.T, db *sql.DB, limitID uuid.UUID, want int64) {
	t.Helper()

	var usage decimal.Decimal
	require.NoError(t, db.QueryRow(`SELECT current_usage FROM usage_counters WHERE limit_id = $1`, limitID).Scan(&usage))
	assert.True(t, decimal.NewFromInt(want).Equal(usage), "usage = %s, want %d", usage, want)
}
