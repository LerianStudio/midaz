//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package segment

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

	"github.com/LerianStudio/midaz/v4/pkg"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const findSegmentSpan = "postgres.find_segment"

// observedContext wires an in-memory span recorder and a mock logger with no
// expectations into the tracking context: any adapter Log call fails the test.
func observedContext(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx := libObservability.ContextWithTracer(t.Context(), tp.Tracer("segment-integration-test"))
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

// TestIntegration_SegmentRepository_Find_NotFoundIsBusinessEventWithoutLog verifies a
// missing segment comes back as a business not-found error, recorded as a span event
// that leaves the span status untouched, with no adapter log line.
func TestIntegration_SegmentRepository_Find_NotFoundIsBusinessEventWithoutLog(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	ctx, recorder := observedContext(t)

	got, err := repo.Find(ctx, orgID, ledgerID, uuid.Must(libCommons.GenerateUUIDv7()))

	assert.Nil(t, got)
	require.Error(t, err)

	var notFound pkg.EntityNotFoundError
	require.True(t, errors.As(err, &notFound), "not-found must be an EntityNotFoundError, got %T", err)

	span := endedSpan(t, recorder, findSegmentSpan)
	assert.Equal(t, codes.Unset, span.Status().Code, "a business miss must not mark the span as error")
	require.NotEmpty(t, span.Events(), "the miss must be recorded as a span event")
	assert.Equal(t, "Failed to scan row", span.Events()[0].Name)
}

// TestIntegration_SegmentRepository_Find_DatabaseFailureIsInfraWithoutLog verifies a
// closed pool comes back as a technical error with the span marked as error and no
// adapter log line: the use case owns the single log.
func TestIntegration_SegmentRepository_Find_DatabaseFailureIsInfraWithoutLog(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	db, err := sql.Open("pgx", container.DSN)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	ctx, recorder := observedContext(t)
	ctx = tmcore.ContextWithPG(ctx, dbresolver.New(dbresolver.WithPrimaryDBs(db), dbresolver.WithReplicaDBs(db)))

	got, err := repo.Find(ctx, uuid.Must(libCommons.GenerateUUIDv7()), uuid.Must(libCommons.GenerateUUIDv7()), uuid.Must(libCommons.GenerateUUIDv7()))

	assert.Nil(t, got)
	require.Error(t, err)
	assert.False(t, pkg.IsBusinessError(err), "a database failure must not be a business error")

	assert.Equal(t, codes.Error, endedSpan(t, recorder, findSegmentSpan).Status().Code)
}
