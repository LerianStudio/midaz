// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// anchorDeps returns the ctx, noop span, and a nop logger used by the anchor
// unit tests. The span is a real otel noop span so SetAttributes /
// HandleSpanError are valid no-ops.
func anchorDeps() (context.Context, trace.Span, libLog.Logger) {
	ctx := context.Background()
	_, span := noop.NewTracerProvider().Tracer("t").Start(ctx, "test")

	return ctx, span, &libLog.NopLogger{}
}

// recordingSpan returns a ctx, a real SDK span that retains attributes, and an
// `ended` closure that ends the span and returns the recorded spans.
func recordingSpan(t *testing.T) (context.Context, trace.Span, func() []sdktrace.ReadOnlySpan) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx, span := tp.Tracer("anchor-test").Start(context.Background(), "reserve")

	return ctx, span, func() []sdktrace.ReadOnlySpan {
		span.End()
		return recorder.Ended()
	}
}

func spanBoolAttribute(spans []sdktrace.ReadOnlySpan, key string) (bool, bool) {
	for _, s := range spans {
		for _, kv := range s.Attributes() {
			if kv.Key == attribute.Key(key) {
				return kv.Value.AsBool(), true
			}
		}
	}

	return false, false
}

// byTxnIdentity builds the identity a by-transaction completion is addressed
// and reported with.
func byTxnIdentity(transactionID uuid.UUID) reservationHandle {
	return reservationHandle{
		TransactionID: transactionID,
		Amount:        decimal.NewFromInt(1000),
		Asset:         "BRL",
	}
}

// anchorPrepared is a prepared @payer -> @payee transfer the anchor can project
// into tracer entries.
func anchorPrepared() (mtransaction.Transaction, *mtransaction.Responses, []*mmodel.Balance) {
	payer := uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338")
	payee := uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10")
	transaction := mtransaction.Transaction{Send: mtransaction.Send{
		Asset:      "BRL",
		Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "0#@payer#default"}}},
		Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "0#@payee#default"}}},
	}}
	validated := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{"0#@payer#default": {Value: decimal.NewFromInt(1000)}},
		To:   map[string]mtransaction.Amount{"0#@payee#default": {Value: decimal.NewFromInt(1000)}},
	}
	balances := []*mmodel.Balance{
		{Alias: "@payer", Key: "default", AccountID: payer.String(), AssetCode: "BRL", AccountType: "deposit"},
		{Alias: "@payee", Key: "default", AccountID: payee.String(), AssetCode: "BRL", AccountType: "deposit"},
	}

	return transaction, validated, balances
}

func anchorInput(settings mmodel.TracerSettings, honoredSkip bool) ContextTracerInput {
	return ContextTracerInput{
		Key: ContextTracerKey{
			OrganizationID: uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			LedgerID:       uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			TransactionID:  uuid.MustParse("44444444-4444-4444-8444-444444444444"),
		},
		Settings:    settings,
		Amount:      decimal.NewFromInt(1000),
		AssetCode:   "BRL",
		HonoredSkip: honoredSkip,
	}
}

func enforceSettings(posture string) mmodel.TracerSettings {
	return mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: posture, ValidationMode: string(tracercontract.ValidationLimits), TimeoutMs: stubContextTracerTimeoutMs}
}

func TestReservePreparedTransaction_NilContextTracerProceeds(t *testing.T) {
	ctx, span, logger := anchorDeps()
	uc := &UseCase{}

	out := uc.reservePreparedTransaction(ctx, span, logger, anchorInput(enforceSettings(mmodel.TracerFailPostureClosed), false), mtransaction.Transaction{}, nil, nil)

	assert.Equal(t, reservationProceed, out.Kind, "no tracer integration means no gate")
	assert.NoError(t, out.Err)
	assert.Nil(t, out.Handle.ContextAttempt, "nothing to complete when the tracer was never consulted")
}

func TestReservePreparedTransaction_OffOrSkippedNeverReserves(t *testing.T) {
	for name, scenario := range map[string]struct {
		settings    mmodel.TracerSettings
		honoredSkip bool
	}{
		"mode off":                 {settings: mmodel.TracerSettings{Mode: mmodel.TracerModeOff}},
		"empty mode":               {settings: mmodel.TracerSettings{}},
		"honored skip on enforce":  {settings: enforceSettings(mmodel.TracerFailPostureClosed), honoredSkip: true},
		"honored skip on advisory": {settings: mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, TimeoutMs: stubContextTracerTimeoutMs}, honoredSkip: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, span, logger := anchorDeps()
			stub := &stubContextTracer{decision: tracercontract.DecisionDeny}
			uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}

			out := uc.reservePreparedTransaction(ctx, span, logger, anchorInput(scenario.settings, scenario.honoredSkip), mtransaction.Transaction{}, nil, nil)

			assert.Equal(t, reservationProceed, out.Kind)
			require.NotNil(t, out.Handle.ContextAttempt)
			assert.True(t, out.Handle.ContextAttempt.Skipped)
			assert.Empty(t, stub.reserves(), "an off or skipped admission must not call the tracer")
		})
	}
}

func TestReservePreparedTransaction_Decisions(t *testing.T) {
	for name, scenario := range map[string]struct {
		mode        string
		decision    tracercontract.Decision
		wantKind    reservationOutcomeKind
		wantCode    string
		wantRelease bool
	}{
		"enforce allow":  {mode: mmodel.TracerModeEnforce, decision: tracercontract.DecisionAllow, wantKind: reservationProceed},
		"enforce deny":   {mode: mmodel.TracerModeEnforce, decision: tracercontract.DecisionDeny, wantKind: reservationReject, wantCode: constant.ErrTransactionReservationDenied.Error(), wantRelease: true},
		"advisory deny":  {mode: mmodel.TracerModeAdvisory, decision: tracercontract.DecisionDeny, wantKind: reservationProceed},
		"advisory allow": {mode: mmodel.TracerModeAdvisory, decision: tracercontract.DecisionAllow, wantKind: reservationProceed},
	} {
		t.Run(name, func(t *testing.T) {
			withFastSharedRetrier(t)

			ctx, span, logger := anchorDeps()
			stub := &stubContextTracer{decision: scenario.decision}
			uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}
			settings := enforceSettings(mmodel.TracerFailPostureClosed)
			settings.Mode = scenario.mode
			transaction, validated, balances := anchorPrepared()
			input := anchorInput(settings, false)

			out := uc.reservePreparedTransaction(ctx, span, logger, input, transaction, validated, balances)
			sharedReservationRetrier.wait()

			require.Equal(t, scenario.wantKind, out.Kind)
			require.Len(t, stub.reserves(), 1)
			require.Equal(t, input.Key.TransactionID, stub.reserves()[0].TransactionID)
			require.Equal(t, reservationRequestID(input.Key.TransactionID), stub.reserves()[0].RequestID)
			require.NotNil(t, out.Handle.ContextAttempt)
			assert.True(t, out.Handle.ContextAttempt.Dispatched)

			if scenario.wantCode != "" {
				var rejected pkg.UnprocessableOperationError
				require.ErrorAs(t, out.Err, &rejected)
				assert.Equal(t, scenario.wantCode, rejected.Code)
			} else {
				assert.NoError(t, out.Err)
			}

			if scenario.wantRelease {
				assert.Equal(t, []uuid.UUID{input.Key.TransactionID}, stub.releasedTransactions(), "a rejected dispatched admission is released")
			} else {
				assert.Empty(t, stub.releasedTransactions())
			}
		})
	}
}

// TestReservePreparedTransaction_FailPosture pins the enforce + unavailable
// tracer branch: fail-open proceeds and marks the reservation skipped on the
// span; fail-closed rejects with 0178 and does not mark it skipped; advisory
// proceeds whatever the posture.
func TestReservePreparedTransaction_FailPosture(t *testing.T) {
	for name, scenario := range map[string]struct {
		mode, posture string
		wantKind      reservationOutcomeKind
	}{
		"enforce fail-open":   {mode: mmodel.TracerModeEnforce, posture: mmodel.TracerFailPostureOpen, wantKind: reservationProceed},
		"enforce fail-closed": {mode: mmodel.TracerModeEnforce, posture: mmodel.TracerFailPostureClosed, wantKind: reservationReject},
		"advisory closed":     {mode: mmodel.TracerModeAdvisory, posture: mmodel.TracerFailPostureClosed, wantKind: reservationProceed},
	} {
		t.Run(name, func(t *testing.T) {
			withFastSharedRetrier(t)

			ctx, span, ended := recordingSpan(t)
			stub := &stubContextTracer{reserveErr: fmt.Errorf("timeout: %w", tracer.ErrTracerUnavailable)}
			uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}
			settings := enforceSettings(scenario.posture)
			settings.Mode = scenario.mode
			transaction, validated, balances := anchorPrepared()

			out := uc.reservePreparedTransaction(ctx, span, &libLog.NopLogger{}, anchorInput(settings, false), transaction, validated, balances)
			sharedReservationRetrier.wait()

			require.Equal(t, scenario.wantKind, out.Kind)

			skipped, recorded := spanBoolAttribute(ended(), "app.response.tracer.reservation_skipped")
			require.True(t, recorded, "an unavailable admission always records whether the reservation was skipped")
			assert.Equal(t, scenario.wantKind == reservationProceed, skipped)

			if scenario.wantKind == reservationReject {
				var unavailable pkg.ServiceUnavailableError
				require.ErrorAs(t, out.Err, &unavailable)
				assert.Equal(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code)
			}
		})
	}
}

func TestReservePreparedTransaction_DeterministicFailureRejectsInEveryPosture(t *testing.T) {
	for _, mode := range []string{mmodel.TracerModeAdvisory, mmodel.TracerModeEnforce} {
		for _, posture := range []string{mmodel.TracerFailPostureOpen, mmodel.TracerFailPostureClosed} {
			t.Run(mode+"/"+posture, func(t *testing.T) {
				withFastSharedRetrier(t)

				ctx, span, logger := anchorDeps()
				stub := &stubContextTracer{reserveErr: constant.ErrContextPolicyUnavailable}
				uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}
				settings := enforceSettings(posture)
				settings.Mode = mode
				transaction, validated, balances := anchorPrepared()

				out := uc.reservePreparedTransaction(ctx, span, logger, anchorInput(settings, false), transaction, validated, balances)
				sharedReservationRetrier.wait()

				require.Equal(t, reservationReject, out.Kind, "a deterministic failure never authorizes accounting")
				require.Error(t, out.Err)
			})
		}
	}
}

func TestReservationRequestID_Deterministic(t *testing.T) {
	txID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	first := reservationRequestID(txID)
	second := reservationRequestID(txID)

	assert.Equal(t, first, second, "the same transactionID must derive the same requestId")
	assert.NotEqual(t, uuid.Nil, first, "requestId must be non-nil for the tracer reserve contract")
	assert.NotEqual(t, reservationRequestID(uuid.MustParse("55555555-5555-5555-5555-555555555555")), first,
		"distinct transactionIDs must derive distinct requestIds")
}

func TestConfirmAndReleaseReservations(t *testing.T) {
	transactionID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	enforce := enforceSettings(mmodel.TracerFailPostureOpen)

	for name, scenario := range map[string]struct {
		attempt   *ContextTracerAttempt
		wantCalls int
	}{
		"no admission":      {},
		"skipped admission": {attempt: &ContextTracerAttempt{Skipped: true, Settings: enforce}},
		"admitted":          {attempt: &ContextTracerAttempt{Dispatched: true, Settings: enforce}, wantCalls: 1},
	} {
		t.Run(name, func(t *testing.T) {
			withFastSharedRetrier(t)

			ctx, span, logger := anchorDeps()
			stub := &stubContextTracer{}
			uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}
			handle := byTxnIdentity(transactionID)
			handle.ContextAttempt = scenario.attempt

			uc.confirmReservations(ctx, span, logger, handle)
			uc.releaseReservations(ctx, span, logger, handle)
			sharedReservationRetrier.wait()

			assert.Len(t, stub.confirmedTransactions(), scenario.wantCalls)
			assert.Len(t, stub.releasedTransactions(), scenario.wantCalls)

			for _, id := range append(stub.confirmedTransactions(), stub.releasedTransactions()...) {
				assert.Equal(t, transactionID, id, "completion is addressed by transaction")
			}
		})
	}
}

func TestReservationsByTransactionGates(t *testing.T) {
	transactionID := uuid.MustParse("77777777-7777-4777-8777-777777777771")

	for name, scenario := range map[string]struct {
		withTracer  bool
		settings    mmodel.TracerSettings
		honoredSkip bool
		wantCalls   int
	}{
		"integration off": {settings: enforceSettings(mmodel.TracerFailPostureOpen)},
		"mode off":        {withTracer: true, settings: mmodel.TracerSettings{Mode: mmodel.TracerModeOff}},
		"empty mode":      {withTracer: true},
		"honored skip":    {withTracer: true, settings: enforceSettings(mmodel.TracerFailPostureOpen), honoredSkip: true},
		"advisory":        {withTracer: true, settings: mmodel.TracerSettings{Mode: mmodel.TracerModeAdvisory, TimeoutMs: stubContextTracerTimeoutMs}, wantCalls: 1},
		"enforce":         {withTracer: true, settings: enforceSettings(mmodel.TracerFailPostureClosed), wantCalls: 1},
	} {
		t.Run(name, func(t *testing.T) {
			withFastSharedRetrier(t)

			ctx, span, logger := anchorDeps()
			stub := &stubContextTracer{}

			uc := &UseCase{}
			if scenario.withTracer {
				uc.ContextTracer = stub.coordinatorFor(t)
			}

			uc.confirmReservationsByTransaction(ctx, span, logger, scenario.settings, byTxnIdentity(transactionID), scenario.honoredSkip)
			uc.releaseReservationsByTransaction(ctx, span, logger, scenario.settings, byTxnIdentity(transactionID), scenario.honoredSkip)
			sharedReservationRetrier.wait()

			assert.Len(t, stub.confirmedTransactions(), scenario.wantCalls)
			assert.Len(t, stub.releasedTransactions(), scenario.wantCalls)
		})
	}
}
