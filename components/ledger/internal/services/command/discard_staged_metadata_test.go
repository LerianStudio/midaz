// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	txMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// discardStagedFlavor runs the same scenarios against both repositories: the
// onboarding and the transaction discard helpers share one contract.
type discardStagedFlavor struct {
	name       string
	entityName string
	// build returns a use case whose matching metadata repository expects
	// Purge(collection=entityName, id=entityID) `times` times, answered by purge.
	build func(t *testing.T, ctrl *gomock.Controller, entityName, entityID string, times int, purge purgeFunc) *UseCase
	// discard invokes the helper under test.
	discard func(uc *UseCase, ctx context.Context, span trace.Span, logger libLog.Logger, entityName, entityID string, metadata map[string]any)
}

// purgeFunc answers a mocked Purge call; it receives the context the helper passed down.
type purgeFunc func(ctx context.Context, collection, id string) error

// purgeReturning answers every Purge call with err.
func purgeReturning(err error) purgeFunc {
	return func(context.Context, string, string) error { return err }
}

func discardStagedFlavors() []discardStagedFlavor {
	return []discardStagedFlavor{
		{
			name:       "onboarding",
			entityName: constant.EntityOrganization,
			build: func(_ *testing.T, ctrl *gomock.Controller, entityName, entityID string, times int, purge purgeFunc) *UseCase {
				repo := onbMongo.NewMockRepository(ctrl)
				repo.EXPECT().Purge(gomock.Any(), entityName, entityID).DoAndReturn(purge).Times(times)

				return &UseCase{OnboardingMetadataRepo: repo}
			},
			discard: (*UseCase).discardStagedOnboardingMetadata,
		},
		{
			name:       "transaction",
			entityName: constant.EntityOperationRoute,
			build: func(_ *testing.T, ctrl *gomock.Controller, entityName, entityID string, times int, purge purgeFunc) *UseCase {
				repo := txMongo.NewMockRepository(ctrl)
				repo.EXPECT().Purge(gomock.Any(), entityName, entityID).DoAndReturn(purge).Times(times)

				return &UseCase{TransactionMetadataRepo: repo}
			},
			discard: (*UseCase).discardStagedTransactionMetadata,
		},
	}
}

// discardStagedCallerSpan starts a span on an in-memory recorder so a test can read its final status.
func discardStagedCallerSpan(ctx context.Context) (trace.Span, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	_, span := tp.Tracer("command-test").Start(ctx, "test.create")

	return span, recorder
}

func TestDiscardStagedMetadata_EmptyMetadataSkipsRepository(t *testing.T) {
	t.Parallel()

	for _, flavor := range discardStagedFlavors() {
		for _, tc := range []struct {
			name     string
			metadata map[string]any
		}{
			{name: "nil", metadata: nil},
			{name: "empty", metadata: map[string]any{}},
		} {
			t.Run(flavor.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				ctrl := gomock.NewController(t)
				uc := flavor.build(t, ctrl, flavor.entityName, "entity-id", 0, purgeReturning(nil))
				logger := &capturingLogger{}
				span, recorder := discardStagedCallerSpan(context.Background())

				flavor.discard(uc, context.Background(), span, logger, flavor.entityName, "entity-id", tc.metadata)
				span.End()

				assert.Empty(t, logger.snapshot(), "nothing was staged, so nothing is logged")
				require.Len(t, recorder.Ended(), 1)
				assert.Equal(t, codes.Unset, recorder.Ended()[0].Status().Code)
			})
		}
	}
}

func TestDiscardStagedMetadata_PurgesStagedDocument(t *testing.T) {
	t.Parallel()

	for _, flavor := range discardStagedFlavors() {
		t.Run(flavor.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := flavor.build(t, ctrl, flavor.entityName, "entity-id", 1, purgeReturning(nil))
			logger := &capturingLogger{}
			span, recorder := discardStagedCallerSpan(context.Background())

			flavor.discard(uc, context.Background(), span, logger, flavor.entityName, "entity-id", map[string]any{"k": "v"})
			span.End()

			assert.Empty(t, logger.snapshot(), "a successful discard logs nothing")
			require.Len(t, recorder.Ended(), 1)
			assert.Equal(t, codes.Unset, recorder.Ended()[0].Status().Code)
		})
	}
}

func TestDiscardStagedMetadata_PurgeFailureIsLoggedNotReturned(t *testing.T) {
	t.Parallel()

	purgeErr := errors.New("server selection timeout")

	for _, flavor := range discardStagedFlavors() {
		t.Run(flavor.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := flavor.build(t, ctrl, flavor.entityName, "entity-id", 1, purgeReturning(purgeErr))
			logger := &capturingLogger{}
			span, recorder := discardStagedCallerSpan(context.Background())

			assert.NotPanics(t, func() {
				flavor.discard(uc, context.Background(), span, logger, flavor.entityName, "entity-id", map[string]any{"k": "v"})
			})
			span.End()

			line := onlyLoggedLine(t, logger)
			assert.Equal(t, libLog.LevelError, line.Level)
			assert.Equal(t, discardStagedMetadataMessage, line.Msg)
			assert.Contains(t, line.Fields, purgeErr.Error())
			assert.Contains(t, line.Fields, flavor.entityName)
			assert.Contains(t, line.Fields, "entity-id")

			require.Len(t, recorder.Ended(), 1)
			assert.Equal(t, codes.Error, recorder.Ended()[0].Status().Code, "the failed discard is recorded on the caller span")
		})
	}
}

// TestDiscardStagedMetadata_PurgesWhenCallerContextCanceled proves the purge runs on a context
// detached from the caller's cancellation and bounded by its own deadline, so a request whose
// context already expired still discards the staged document.
func TestDiscardStagedMetadata_PurgesWhenCallerContextCanceled(t *testing.T) {
	t.Parallel()

	for _, flavor := range discardStagedFlavors() {
		t.Run(flavor.name, func(t *testing.T) {
			t.Parallel()

			callerCtx, cancel := context.WithCancel(context.Background())
			cancel()

			purge := func(ctx context.Context, _, _ string) error {
				assert.NoError(t, ctx.Err(), "the purge context must not inherit the caller's cancellation")

				deadline, ok := ctx.Deadline()
				if assert.True(t, ok, "the purge context must carry its own deadline") {
					assert.LessOrEqual(t, time.Until(deadline), discardStagedMetadataTimeout)
				}

				return nil
			}

			ctrl := gomock.NewController(t)
			uc := flavor.build(t, ctrl, flavor.entityName, "entity-id", 1, purge)
			logger := &capturingLogger{}
			span, recorder := discardStagedCallerSpan(callerCtx)

			flavor.discard(uc, callerCtx, span, logger, flavor.entityName, "entity-id", map[string]any{"k": "v"})
			span.End()

			assert.Empty(t, logger.snapshot(), "a successful discard logs nothing")
			require.Len(t, recorder.Ended(), 1)
			assert.Equal(t, codes.Unset, recorder.Ended()[0].Status().Code)
		})
	}
}
