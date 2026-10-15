//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounttype

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// observedContext wires an in-memory span recorder and a mock logger with no
// expectations into the tracking context: any adapter Log call fails the test.
func observedContext(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx := libObservability.ContextWithTracer(t.Context(), tp.Tracer("accounttype-integration-test"))
	ctx = libObservability.ContextWithLogger(ctx, libLog.NewMockLogger(gomock.NewController(t)))

	return ctx, recorder
}

// endedSpan returns the ended span with the given name.
func endedSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() == name {
			return s
		}
	}

	t.Fatalf("span %q was not recorded", name)

	return nil
}

// closedPoolContext injects a tenant-style connection whose pool is already closed,
// so every query fails with an infrastructure error without stopping the shared container.
func closedPoolContext(t *testing.T, ctx context.Context, container *pgtestutil.ContainerResult) context.Context {
	t.Helper()

	db, err := sql.Open("pgx", container.DSN)
	require.NoError(t, err, "failed to open the pool that will be closed")
	require.NoError(t, db.Close(), "failed to close the pool")

	return tmcore.ContextWithPG(ctx, dbresolver.New(dbresolver.WithPrimaryDBs(db), dbresolver.WithReplicaDBs(db)))
}

// assertInfraError checks a database failure surfaces as a technical error, never as a
// not-found sentinel or a business error.
func assertInfraError(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	assert.False(t, errors.Is(err, services.ErrDatabaseItemNotFound), "a database failure must not look like not-found")
	assert.False(t, pkg.IsBusinessError(err), "a database failure must not be a business error")
}

// TestIntegration_AccountTypeRepository_NotFoundDoesNotLog verifies Find, Update and
// Delete answer a missing row with the not-found sentinel and emit no log line.
func TestIntegration_AccountTypeRepository_NotFoundDoesNotLog(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	missingID := uuid.Must(libCommons.GenerateUUIDv7())

	t.Run("find", func(t *testing.T) {
		ctx, recorder := observedContext(t)

		got, err := repo.FindByID(ctx, orgID, ledgerID, missingID)

		assert.Nil(t, got)
		require.ErrorIs(t, err, services.ErrDatabaseItemNotFound)
		assert.Equal(t, codes.Unset, endedSpan(t, recorder, "postgres.find_account_type_by_id").Status().Code)
	})

	t.Run("update", func(t *testing.T) {
		ctx, recorder := observedContext(t)

		got, err := repo.Update(ctx, orgID, ledgerID, missingID, &mmodel.AccountType{Name: "Updated Name"})

		assert.Nil(t, got)
		require.ErrorIs(t, err, services.ErrDatabaseItemNotFound)
		assert.Equal(t, codes.Unset, endedSpan(t, recorder, "postgres.update_account_type").Status().Code)
	})

	t.Run("delete", func(t *testing.T) {
		ctx, recorder := observedContext(t)

		require.ErrorIs(t, repo.Delete(ctx, orgID, ledgerID, missingID), services.ErrDatabaseItemNotFound)
		assert.Equal(t, codes.Unset, endedSpan(t, recorder, "postgres.delete_account_type").Status().Code)
	})
}

// TestIntegration_AccountTypeRepository_ConnectionFailureDoesNotLog verifies a closed
// pool reaches the caller as a technical error on Find, Update and Delete, and the
// adapter records it on the span only, emitting no log line.
func TestIntegration_AccountTypeRepository_ConnectionFailureDoesNotLog(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	id := uuid.Must(libCommons.GenerateUUIDv7())

	t.Run("find", func(t *testing.T) {
		observed, recorder := observedContext(t)
		ctx := closedPoolContext(t, observed, container)

		got, err := repo.FindByID(ctx, orgID, ledgerID, id)

		assert.Nil(t, got)
		assertInfraError(t, err)
		assert.Equal(t, codes.Error, endedSpan(t, recorder, "postgres.find_account_type_by_id").Status().Code)
	})

	t.Run("update", func(t *testing.T) {
		observed, recorder := observedContext(t)
		ctx := closedPoolContext(t, observed, container)

		got, err := repo.Update(ctx, orgID, ledgerID, id, &mmodel.AccountType{Name: "Updated Name"})

		assert.Nil(t, got)
		assertInfraError(t, err)
		assert.Equal(t, codes.Error, endedSpan(t, recorder, "postgres.update_account_type").Status().Code)
	})

	t.Run("delete", func(t *testing.T) {
		observed, recorder := observedContext(t)
		ctx := closedPoolContext(t, observed, container)

		assertInfraError(t, repo.Delete(ctx, orgID, ledgerID, id))
		assert.Equal(t, codes.Error, endedSpan(t, recorder, "postgres.delete_account_type").Status().Code)
	})
}
