// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestPendingContextCompletionDeliversByTransaction(t *testing.T) {
	evaluationID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	for _, scenario := range []string{"commit confirms", "cancel releases", "mode off", "skip honored", "delivery retried", "operation conflict"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("AUDIT_LOG_ENABLED", "false")
			withFastSharedRetrier(t)
			status := constant.APPROVED
			if scenario == "cancel releases" {
				status = constant.CANCELED
			}
			uc, reader, engine, _, input := newTransitionEngineUseCase(t, status)
			reader.settings.Tracer.Mode, reader.settings.Tracer.TimeoutMs = "enforce", 250
			if scenario == "mode off" {
				reader.settings.Tracer.Mode = "off"
			}
			if scenario == "skip honored" {
				reader.settings.Overrides.AllowTracerSkip = true
				reader.persisted.Body.Skip = &mtransaction.TransactionSkip{Tracer: true}
				reader.loaded.Body.Skip = &mtransaction.TransactionSkip{Tracer: true}
			}
			legacy := &stubReserver{}
			uc.TracerReserver = legacy
			ctrl := gomock.NewController(t)
			client := NewMockContextTracerReserver(ctrl)
			coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
			require.NoError(t, err)
			uc.ContextTracer = coordinator
			evaluated := func(transactionID uuid.UUID, status string) *tracercontract.TransactionCompletionResult {
				result := completionResult(transactionID, status)
				result.EvaluationID = &evaluationID
				return result
			}
			switch scenario {
			case "commit confirms":
				client.EXPECT().ConfirmByTransaction(gomock.Any(), input.TransactionID).DoAndReturn(func(ctx context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
					require.Len(t, engine.requests, 1, "completion follows accounting")
					deadline, bounded := ctx.Deadline()
					require.True(t, bounded)
					require.LessOrEqual(t, time.Until(deadline), 250*time.Millisecond, "completion is bounded by the ledger timeout")
					return evaluated(transactionID, "CONFIRMED"), nil
				})
			case "cancel releases":
				client.EXPECT().ReleaseByTransaction(gomock.Any(), input.TransactionID).DoAndReturn(func(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
					require.Len(t, engine.requests, 1, "completion follows accounting")
					return evaluated(transactionID, "RELEASED"), nil
				})
			case "delivery retried":
				gomock.InOrder(
					client.EXPECT().ConfirmByTransaction(gomock.Any(), input.TransactionID).Return(nil, traceradapter.ErrTracerUnavailable),
					client.EXPECT().ConfirmByTransaction(gomock.Any(), input.TransactionID).DoAndReturn(func(ctx context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
						deadline, bounded := ctx.Deadline()
						assert.True(t, bounded)
						assert.LessOrEqual(t, time.Until(deadline), 250*time.Millisecond, "redelivery keeps the ledger timeout")
						return evaluated(transactionID, "CONFIRMED"), nil
					}),
				)
			case "operation conflict":
				client.EXPECT().ConfirmByTransaction(gomock.Any(), input.TransactionID).Return(nil, constant.ErrReserveOperationConflict).Times(1)
			}
			ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-a")
			if status == constant.CANCELED {
				_, err = uc.CancelTransactionV2(ctx, input)
			} else {
				_, err = uc.CommitTransactionV2(ctx, input)
			}
			require.NoError(t, err, "a completion failure never fails a transaction whose accounting has run")
			sharedReservationRetrier.wait()
			require.Len(t, engine.requests, 1)
			require.Empty(t, legacy.confirmedTxns, "the context client owns by-transaction completion")
			require.Empty(t, legacy.releasedTxns)
		})
	}
}

func TestContextTracerCompleteRejectsMismatchedEcho(t *testing.T) {
	transactionID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	for name, result := range map[string]*tracercontract.TransactionCompletionResult{
		"missing":          nil,
		"other status":     completionResult(transactionID, "RELEASED"),
		"other identity":   completionResult(uuid.MustParse("22222222-2222-4222-8222-222222222222"), "CONFIRMED"),
		"invalid revision": {TransactionID: transactionID, Status: "CONFIRMED"},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockContextTracerReserver(ctrl)
			coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
			require.NoError(t, err)
			client.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(result, nil)
			err = coordinator.Complete(t.Context(), transactionID, reservationActionConfirm, 0)
			require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
		})
	}

	t.Run("unknown action", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := NewMockContextTracerReserver(ctrl)
		coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
		require.NoError(t, err)
		err = coordinator.Complete(t.Context(), transactionID, "revert", 0)
		require.ErrorIs(t, err, constant.ErrTracerContractUnavailable, "an unknown action is rejected, never sent as a confirm")
		require.True(t, contextCompletionTerminal(err))
	})

	t.Run("transport error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := NewMockContextTracerReserver(ctrl)
		coordinator, err := NewContextTracerCoordinator(client, NewMockTracerFactsLoader(ctrl), ContextTracerConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxReservations: 100, AdmissionTimeout: time.Second}, time.Now)
		require.NoError(t, err)
		cause := errors.New("connection reset")
		client.EXPECT().ReleaseByTransaction(gomock.Any(), transactionID).Return(nil, cause)
		err = coordinator.Complete(t.Context(), transactionID, reservationActionRelease, 0)
		require.ErrorIs(t, err, cause)
	})
}
