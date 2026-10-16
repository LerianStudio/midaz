//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"database/sql"
	"errors"
	"sync"
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

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const getLedgerByIDSpan = "query.get_ledger_by_id"

// ledgerLogLine is one Log call captured from the use case.
type ledgerLogLine struct {
	level int
	msg   string
}

// ledgerLogRecorder captures every Log call made through the mock logger.
type ledgerLogRecorder struct {
	mu    sync.Mutex
	lines []ledgerLogLine
}

// errors returns the captured lines logged at Error.
func (r *ledgerLogRecorder) errors() []ledgerLogLine {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []ledgerLogLine

	for _, line := range r.lines {
		if line.level == libLog.LevelError {
			out = append(out, line)
		}
	}

	return out
}

// getLedgerByIDHarness builds the query use case over the real ledger repository of a
// migrated container, a metadata mock with no expectations (it is only read after a
// found ledger), a span recorder and a recording logger in the tracking context.
func getLedgerByIDHarness(t *testing.T) (*UseCase, *pgtestutil.ContainerResult, context.Context, *tracetest.SpanRecorder, *ledgerLogRecorder) {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := pgtestutil.ConnectPostgresClient(t.Context(), t, connStr, connStr)

	ctrl := gomock.NewController(t)

	uc := &UseCase{
		LedgerRepo:             ledger.NewLedgerPostgreSQLRepository(conn),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	logs := &ledgerLogRecorder{}
	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, level int, msg string, _ ...any) {
			logs.mu.Lock()
			defer logs.mu.Unlock()

			logs.lines = append(logs.lines, ledgerLogLine{level: level, msg: msg})
		}).
		AnyTimes()

	ctx := libObservability.ContextWithTracer(t.Context(), tp.Tracer("query-integration-test"))
	ctx = libObservability.ContextWithLogger(ctx, logger)

	return uc, container, ctx, recorder, logs
}

func getLedgerByIDEndedSpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() == getLedgerByIDSpan {
			return s
		}
	}

	t.Fatalf("span %q was not recorded", getLedgerByIDSpan)

	return nil
}

// TestIntegration_GetLedgerByID_NotFoundIsBusinessWithoutErrorLog verifies a missing
// ledger reaches the caller as an EntityNotFoundError with no ERROR log and a span
// that is not marked as error.
func TestIntegration_GetLedgerByID_NotFoundIsBusinessWithoutErrorLog(t *testing.T) {
	uc, container, ctx, recorder, logs := getLedgerByIDHarness(t)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	got, err := uc.GetLedgerByID(ctx, orgID, uuid.Must(libCommons.GenerateUUIDv7()))

	assert.Nil(t, got)
	require.Error(t, err)

	var notFound pkg.EntityNotFoundError
	require.True(t, errors.As(err, &notFound), "a missing ledger must be an EntityNotFoundError, got %T", err)

	assert.Empty(t, logs.errors(), "a business miss must not be logged at Error")
	assert.Equal(t, codes.Unset, getLedgerByIDEndedSpan(t, recorder).Status().Code)
}

// TestIntegration_GetLedgerByID_DatabaseUnavailableLogsOnce verifies an unavailable
// database reaches the caller as a technical error, logged exactly once at Error by the
// use case, with the span marked as error.
func TestIntegration_GetLedgerByID_DatabaseUnavailableLogsOnce(t *testing.T) {
	uc, container, ctx, recorder, logs := getLedgerByIDHarness(t)

	// A closed pool injected through the tenant context makes every query fail
	// without stopping the shared container.
	db, err := sql.Open("pgx", container.DSN)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	ctx = tmcore.ContextWithPG(ctx, dbresolver.New(dbresolver.WithPrimaryDBs(db), dbresolver.WithReplicaDBs(db)))

	got, err := uc.GetLedgerByID(ctx, uuid.Must(libCommons.GenerateUUIDv7()), uuid.Must(libCommons.GenerateUUIDv7()))

	assert.Nil(t, got)
	require.Error(t, err)
	assert.False(t, pkg.IsBusinessError(err), "a database failure must not be a business error")

	errorLines := logs.errors()
	require.Len(t, errorLines, 1, "the use case must log the infrastructure error exactly once")
	assert.Equal(t, "Failed to get ledger on repo by id", errorLines[0].msg)

	assert.Equal(t, codes.Error, getLedgerByIDEndedSpan(t, recorder).Status().Code)
}
