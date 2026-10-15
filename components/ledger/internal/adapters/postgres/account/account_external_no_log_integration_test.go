//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"database/sql"
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

	"github.com/LerianStudio/midaz/v4/pkg"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const listExternalAccountsSpan = "postgres.list_external_accounts_by_asset_code"

// externalAccountsObservedContext wires an in-memory span recorder and a mock logger
// into the tracking context. Only the adapter's Debug "Executing query" line is
// allowed; any other Log call fails the test.
func externalAccountsObservedContext(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	logger := libLog.NewMockLogger(gomock.NewController(t))
	logger.EXPECT().Log(gomock.Any(), libLog.LevelDebug, "Executing query", gomock.Any()).AnyTimes()

	ctx := libObservability.ContextWithTracer(t.Context(), tp.Tracer("account-integration-test"))
	ctx = libObservability.ContextWithLogger(ctx, logger)

	return ctx, recorder
}

func externalAccountsSpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() == listExternalAccountsSpan {
			return s
		}
	}

	t.Fatalf("span %q was not recorded", listExternalAccountsSpan)

	return nil
}

// TestIntegration_AccountRepository_ListExternalAccountsByAssetCode_EmptyWithoutLog
// verifies the listing has no not-found branch: no external account for the asset is
// an empty result, not an error, with a clean span and no adapter log beyond Debug.
func TestIntegration_AccountRepository_ListExternalAccountsByAssetCode_EmptyWithoutLog(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx, recorder := externalAccountsObservedContext(t)

	got, err := repo.ListExternalAccountsByAssetCode(ctx, orgID, ledgerID, "BRL")

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, codes.Unset, externalAccountsSpan(t, recorder).Status().Code)
}

// TestIntegration_AccountRepository_ListExternalAccountsByAssetCode_DatabaseFailureWithoutLog
// verifies a closed pool comes back as a technical error with the span marked as error
// and no adapter log line beyond Debug: the use case owns the single log.
func TestIntegration_AccountRepository_ListExternalAccountsByAssetCode_DatabaseFailureWithoutLog(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	db, err := sql.Open("pgx", container.DSN)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	ctx, recorder := externalAccountsObservedContext(t)
	ctx = tmcore.ContextWithPG(ctx, dbresolver.New(dbresolver.WithPrimaryDBs(db), dbresolver.WithReplicaDBs(db)))

	got, err := repo.ListExternalAccountsByAssetCode(ctx, uuid.Must(libCommons.GenerateUUIDv7()), uuid.Must(libCommons.GenerateUUIDv7()), "BRL")

	assert.Nil(t, got)
	require.Error(t, err)
	assert.False(t, pkg.IsBusinessError(err), "a database failure must not be a business error")
	assert.Equal(t, codes.Error, externalAccountsSpan(t, recorder).Status().Code)
}
