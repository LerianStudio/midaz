// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
)

// classifiedExecutionSpan records one invocation outcome on a fresh execute span.
func classifiedExecutionSpan(t *testing.T, err error) sdktrace.ReadOnlySpan {
	t.Helper()

	ctx, recorder := spantest.RecordingContext()
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, executeAccountingSpanName)
	recordExecutionSpan(span, accounting.Execution{}, err)
	span.End()

	return spantest.FindSpan(t, recorder, executeAccountingSpanName)
}

// Scenarios: Execução aprovada; Saldo insuficiente é evento, não erro; Falha técnica do
// engine marca erro. The outcome attribute carries the same label as the metrics.
func TestExecuteAccountingSpan_ClassifiesTheOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		outcome    string
		status     codes.Code
		wantsEvent bool
	}{
		{name: "Execução aprovada", err: nil, outcome: "success", status: codes.Unset},
		{
			name:    "Saldo insuficiente é evento, não erro",
			err:     &accounting.Failure{Code: accounting.FailureInsufficientFunds},
			outcome: "refused", status: codes.Unset, wantsEvent: true,
		},
		{
			name:    "confirmed refusal answered by the script as technical",
			err:     technical("transaction_already_reverted", false, nil),
			outcome: "refused", status: codes.Unset, wantsEvent: true,
		},
		{
			name:    "Falha técnica do engine marca erro (indeterminate)",
			err:     fmt.Errorf("wrapper: %w", technical("transport", true, errors.New("connection dropped after write"))),
			outcome: "indeterminate", status: codes.Error,
		},
		{
			name:    "caller cancellation is an event, not an error",
			err:     technical("context_canceled", false, context.Canceled),
			outcome: "technical_error", status: codes.Unset,
		},
		{
			name:    "Falha técnica do engine marca erro (certain)",
			err:     technical("connection_unavailable", false, errors.New("dial refused")),
			outcome: "technical_error", status: codes.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			span := classifiedExecutionSpan(t, tt.err)

			assert.Contains(t, span.Attributes(), attribute.String("app.response.engine_outcome", tt.outcome))
			assert.Equal(t, tt.status, span.Status().Code)

			_, hasEvent := spantest.FindEvent(span, "Accounting execution refused")
			assert.Equal(t, tt.wantsEvent, hasEvent)
		})
	}
}

// Execute opens the span around the whole invocation and leaves the returned error, and
// its outcome certainty, exactly as before.
func TestExecuteAccountingSpan_WrapsTheInvocationWithoutChangingTheError(t *testing.T) {
	t.Parallel()

	_, limits, _ := validWireExecution()
	provider := &countingProvider{}
	adapter, err := newAdapterWithLimits(provider, limits)
	require.NoError(t, err)

	ctx, recorder := spantest.RecordingContext()
	ctx, cancel := context.WithCancel(ctx)
	cancel()

	result, err := adapter.Execute(ctx, command.EngineExecution{})
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)

	var failure *TechnicalError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "context_canceled", failure.Code)
	assert.False(t, failure.OutcomeIndeterminate(), "the certainty of the outcome is unchanged")
	assert.Zero(t, provider.calls)

	// A caller cancellation keeps the metric label but not an error status.
	span := spantest.FindSpan(t, recorder, executeAccountingSpanName)
	assert.Contains(t, span.Attributes(), attribute.String("app.response.engine_outcome", "technical_error"))
	assert.Equal(t, codes.Unset, span.Status().Code)

	_, abandoned := spantest.FindEvent(span, "Accounting execution abandoned by caller")
	assert.True(t, abandoned)
}

// An indeterminate outcome whose cause is a deadline stays an error: the accounting may
// have committed, which is never a caller-side abandonment.
func TestExecuteAccountingSpan_IndeterminateDeadlineStaysAnError(t *testing.T) {
	t.Parallel()

	span := classifiedExecutionSpan(t, technical("transport", true, context.DeadlineExceeded))

	assert.Contains(t, span.Attributes(), attribute.String("app.response.engine_outcome", "indeterminate"))
	assert.Equal(t, codes.Error, span.Status().Code)
}
