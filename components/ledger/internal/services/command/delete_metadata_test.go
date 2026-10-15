// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"strings"
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

const softDeleteTestEntityID = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

// fastMetadataDeleteRetryPolicy keeps the production attempt count with no
// backoff wait, so unit tests never sleep.
func fastMetadataDeleteRetryPolicy() metadataDeleteRetryPolicy {
	policy := defaultMetadataDeleteRetryPolicy()
	policy.BaseDelay = 0

	return policy
}

// softDeleteSpan starts a recorded span and returns a function that ends it and
// reports its final status code.
func softDeleteSpan(t *testing.T) (trace.Span, func() codes.Code) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	_, span := tp.Tracer("delete-metadata-test").Start(context.Background(), "command.delete_entity")

	return span, func() codes.Code {
		span.End()

		ended := recorder.Ended()
		require.Len(t, ended, 1)

		return ended[0].Status().Code
	}
}

func exhaustionWarns(logger *capturingLogger) []capturedLogLine {
	var out []capturedLogLine

	for _, line := range logger.atLevelOrMoreSevere(libLog.LevelWarn) {
		if strings.Contains(line.Msg, "Failed to soft delete metadata") {
			out = append(out, line)
		}
	}

	return out
}

func TestDeleteMetadata_SucceedsOnFirstAttempt(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := onbMongo.NewMockRepository(ctrl)
	repo.EXPECT().Delete(gomock.Any(), constant.EntityAccount, softDeleteTestEntityID).Return(nil).Times(1)

	uc := &UseCase{OnboardingMetadataRepo: repo, metadataDeleteRetry: fastMetadataDeleteRetryPolicy()}
	logger := &capturingLogger{}
	span, status := softDeleteSpan(t)

	uc.softDeleteOnboardingMetadata(context.Background(), span, logger, constant.EntityAccount, softDeleteTestEntityID)

	assert.Empty(t, logger.atLevelOrMoreSevere(libLog.LevelWarn))
	assert.NotEqual(t, codes.Error, status())
}

func TestDeleteMetadata_TransientFailureThenSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := txMongo.NewMockRepository(ctrl)
	gomock.InOrder(
		repo.EXPECT().Delete(gomock.Any(), constant.EntityOperationRoute, softDeleteTestEntityID).Return(errors.New("connection reset")),
		repo.EXPECT().Delete(gomock.Any(), constant.EntityOperationRoute, softDeleteTestEntityID).Return(nil),
	)

	uc := &UseCase{TransactionMetadataRepo: repo, metadataDeleteRetry: fastMetadataDeleteRetryPolicy()}
	logger := &capturingLogger{}
	span, status := softDeleteSpan(t)

	uc.softDeleteTransactionMetadata(context.Background(), span, logger, constant.EntityOperationRoute, softDeleteTestEntityID)

	assert.Empty(t, exhaustionWarns(logger), "a retry that succeeds must not report exhaustion")
	assert.NotEqual(t, codes.Error, status())
}

func TestDeleteMetadata_PersistentFailureWarnsWithEntityID(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := onbMongo.NewMockRepository(ctrl)
	policy := fastMetadataDeleteRetryPolicy()
	repo.EXPECT().Delete(gomock.Any(), constant.EntityLedger, softDeleteTestEntityID).
		Return(errors.New("server selection timeout")).Times(policy.Attempts)

	uc := &UseCase{OnboardingMetadataRepo: repo, metadataDeleteRetry: policy}
	logger := &capturingLogger{}
	span, status := softDeleteSpan(t)

	uc.softDeleteOnboardingMetadata(context.Background(), span, logger, constant.EntityLedger, softDeleteTestEntityID)

	warns := exhaustionWarns(logger)
	require.Len(t, warns, 1, "exhaustion is reported exactly once")
	assert.Equal(t, libLog.LevelWarn, warns[0].Level)
	assert.Contains(t, warns[0].Fields, "{entity_id "+softDeleteTestEntityID+"}", "the Warn names the entity under entity_id")
	assert.Len(t, logger.atLevelOrMoreSevere(libLog.LevelWarn), 1, "intermediate failures are not logged at Warn")
	assert.Equal(t, codes.Error, status(), "exhaustion is a technical failure on the span")
}

func TestDeleteMetadata_StopsWhenContextIsCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := onbMongo.NewMockRepository(ctrl)
	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(cancel)

	repo.EXPECT().Delete(gomock.Any(), constant.EntitySegment, softDeleteTestEntityID).
		DoAndReturn(func(context.Context, string, string) error {
			cancel()

			return errors.New("context canceled")
		}).Times(1)

	uc := &UseCase{OnboardingMetadataRepo: repo, metadataDeleteRetry: fastMetadataDeleteRetryPolicy()}
	logger := &capturingLogger{}
	span, status := softDeleteSpan(t)

	uc.softDeleteOnboardingMetadata(ctx, span, logger, constant.EntitySegment, softDeleteTestEntityID)

	require.Len(t, exhaustionWarns(logger), 1, "giving up on a done context is still reported")
	assert.Equal(t, codes.Error, status())
}

func TestDeleteMetadata_StopsWhenContextIsCancelledDuringWait(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := onbMongo.NewMockRepository(ctrl)
	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(cancel)

	firstCall := make(chan struct{})
	cancelled := make(chan struct{})

	repo.EXPECT().Delete(gomock.Any(), constant.EntityPortfolio, softDeleteTestEntityID).
		DoAndReturn(func(context.Context, string, string) error {
			close(firstCall)

			return errors.New("server down")
		}).Times(1)

	go func() {
		defer close(cancelled)

		<-firstCall
		cancel()
	}()

	// A one-hour base keeps the jittered wait far longer than the cancel takes
	// to land, so the loop is inside WaitContext when ctx is done.
	uc := &UseCase{OnboardingMetadataRepo: repo, metadataDeleteRetry: metadataDeleteRetryPolicy{Attempts: 3, BaseDelay: time.Hour}}
	logger := &capturingLogger{}
	span, status := softDeleteSpan(t)

	start := time.Now()

	uc.softDeleteOnboardingMetadata(ctx, span, logger, constant.EntityPortfolio, softDeleteTestEntityID)

	elapsed := time.Since(start)

	<-cancelled

	warns := exhaustionWarns(logger)
	require.Len(t, warns, 1, "giving up during the wait is reported once")
	assert.Contains(t, warns[0].Fields, "server down", "the Warn carries the Delete error")
	assert.NotContains(t, warns[0].Fields, context.Canceled.Error(), "the Warn does not report the cancellation as the cause")
	assert.Equal(t, codes.Error, status())
	assert.Less(t, elapsed, time.Second, "the wait ends when ctx is done")
}

func TestDeleteMetadata_DefaultPolicyAppliesWhenUnset(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := onbMongo.NewMockRepository(ctrl)
	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(cancel)

	repo.EXPECT().Delete(gomock.Any(), constant.EntityAsset, softDeleteTestEntityID).
		DoAndReturn(func(context.Context, string, string) error {
			cancel()

			return errors.New("not primary")
		}).Times(1)

	uc := &UseCase{OnboardingMetadataRepo: repo}
	span, _ := softDeleteSpan(t)

	uc.softDeleteOnboardingMetadata(ctx, span, &capturingLogger{}, constant.EntityAsset, softDeleteTestEntityID)

	assert.Equal(t, metadataDeleteRetryPolicy{}, uc.metadataDeleteRetry, "the default policy is resolved per call, not stored")
	assert.Equal(t, 3, defaultMetadataDeleteRetryPolicy().Attempts)
}
