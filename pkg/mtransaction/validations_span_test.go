// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"context"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v7/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// commandSpanName stands in for the command span that runs the validation in production.
const commandSpanName = "command.create_transaction_v1"

// withRecordedCommandSpan opens a recorded parent span named like the calling command,
// runs fn under it and returns every ended span. pkg/ cannot import the ledger's internal
// spantest package, so the recorder is wired here.
func withRecordedCommandSpan(t *testing.T, fn func(ctx context.Context)) []sdktrace.ReadOnlySpan {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracer := tp.Tracer("mtransaction-test")

	ctx := libObservability.ContextWithTracer(context.Background(), tracer)

	ctx, span := tracer.Start(ctx, commandSpanName)
	fn(ctx)
	span.End()

	return recorder.Ended()
}

// onlyCommandSpan asserts the command span is the only recorded span and returns it.
func onlyCommandSpan(t *testing.T, ended []sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	t.Helper()

	names := make([]string, 0, len(ended))
	for _, s := range ended {
		names = append(names, s.Name())
	}

	require.Equal(t, []string{commandSpanName}, names, "the validation steps must not open spans of their own")

	return ended[0]
}

func spanTestSend(sourceValue, destinationValue int64) Transaction {
	source := Amount{Asset: "BRL", Value: decimal.NewFromInt(sourceValue)}
	destination := Amount{Asset: "BRL", Value: decimal.NewFromInt(destinationValue)}

	return Transaction{Send: Send{
		Asset:      "BRL",
		Value:      decimal.NewFromInt(sourceValue),
		Source:     Source{From: []FromTo{{AccountAlias: "@payer", Amount: &source, IsFrom: true}}},
		Distribute: Distribute{To: []FromTo{{AccountAlias: "@receiver", Amount: &destination}}},
	}}
}

// Scenario: Validação de envio com sucesso não deixa rastro.
func TestValidationSpans_AcceptedSendLeavesNoTrace(t *testing.T) {
	t.Parallel()

	ended := withRecordedCommandSpan(t, func(ctx context.Context) {
		validate, err := ValidateSendSourceAndDistribute(ctx, spanTestSend(10, 10), constant.APPROVED)
		require.NoError(t, err)

		PropagateRouteValidation(ctx, validate, constant.APPROVED)
	})

	span := onlyCommandSpan(t, ended)
	assert.Empty(t, span.Events(), "an accepted send records no step event on the command span")
	assert.Equal(t, otelcodes.Unset, span.Status().Code)
}

// Scenario: Validação de envio com erro de negócio vira evento no comando.
func TestValidationSpans_RejectedSendIsABusinessEventOnTheCommandSpan(t *testing.T) {
	t.Parallel()

	var err error

	ended := withRecordedCommandSpan(t, func(ctx context.Context) {
		// The distribution total differs from the sent value.
		_, err = ValidateSendSourceAndDistribute(ctx, spanTestSend(10, 7), constant.CREATED)
	})

	require.Error(t, err)
	assert.Equal(t, pkgConstant.ErrTransactionValueMismatch.Error(), codeFromError(err),
		"the rejection keeps its business error code")

	span := onlyCommandSpan(t, ended)
	assert.Equal(t, otelcodes.Unset, span.Status().Code, "a business rejection keeps the command span status unset")

	require.Len(t, span.Events(), 1)
	assert.Equal(t, "transaction.validate_send_source_and_distribute", span.Events()[0].Name,
		"the event is named after the step that failed")
}

// Without a span in the context (callers outside a request) the rejection is still
// returned and recording falls on a no-op span.
func TestValidationSpans_RejectionWithoutSpanInContextIsSafe(t *testing.T) {
	t.Parallel()

	require.False(t, trace.SpanFromContext(context.Background()).SpanContext().IsValid())

	_, err := ValidateSendSourceAndDistribute(context.Background(), spanTestSend(10, 7), constant.CREATED)
	require.Error(t, err)
	assert.Equal(t, pkgConstant.ErrTransactionValueMismatch.Error(), codeFromError(err))
}
