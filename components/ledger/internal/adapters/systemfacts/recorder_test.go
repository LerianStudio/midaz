// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package systemfacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-system-facts/emitter"
	"github.com/LerianStudio/lib-system-facts/outbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// capturingEmitter is the kit's Emitter port: it records every fact handed
// to Emit and answers err, so a test states what the kit answers.
type capturingEmitter struct {
	mu    sync.Mutex
	facts []emitter.Fact
	err   error
}

func (c *capturingEmitter) Emit(_ context.Context, _ *sql.Tx, f emitter.Fact) (emitter.Receipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.facts = append(c.facts, f)

	if c.err != nil {
		return emitter.Receipt{}, c.err
	}

	return emitter.Receipt{FactID: f.FactID}, nil
}

func (c *capturingEmitter) captured() []emitter.Fact {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.facts)
}

type logRecord struct {
	level  int
	msg    string
	fields []libLog.Field
}

type logRecorder struct {
	mu      sync.Mutex
	records []logRecord
}

func (r *logRecorder) add(rec logRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.records = append(r.records, rec)
}

func (r *logRecorder) snapshot() []logRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.records)
}

// recordingLogger is the lib-observability GoMock logger wired to append
// every Log call into the returned recorder, so level, message and fields
// are assertable in the test body.
func recordingLogger(t *testing.T) (libLog.Logger, *logRecorder) {
	t.Helper()

	ctrl := gomock.NewController(t)
	recorder := &logRecorder{}
	logger := libLog.NewMockLogger(ctrl)

	// The closure runs through reflection, so its parameters are the exact
	// universal types of the mocked method: (int, ...any).
	capture := func(_ context.Context, level int, msg string, fields ...any) {
		recorder.add(logRecord{level: level, msg: msg, fields: libLog.Fields(fields...)})
	}

	// Four matchers for a three-arg-plus-variadic method: with exactly
	// NumIn() matchers gomock accepts any number of variadic args.
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(capture).AnyTimes()
	logger.EXPECT().With(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithGroup(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Enabled(gomock.Any()).Return(true).AnyTimes()
	logger.EXPECT().Sync(gomock.Any()).Return(nil).AnyTimes()

	return logger, recorder
}

// recordingContext returns a context whose tracer records every ended span.
func recordingContext(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return libObservability.ContextWithTracer(context.Background(), tp.Tracer("systemfacts-test")), recorder
}

func endedSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range recorder.Ended() {
		if span.Name() == name {
			return span
		}
	}

	require.Failf(t, "span not found", "no ended span named %q", name)

	return nil
}

// outflowCommit is the fact scenario the recorder tests exercise: it carries
// a description, so a leaked payload value is detectable in the log fields.
func outflowCommit() command.TransactionWriteSet {
	return writeSet(constant.ActionCommit,
		[]*operation.Operation{aliceDebit(1, "89.90"), externalCredit(2, "89.90")},
		withAmount("89.90"), withScheme("PIX"), withDescription("Pix payment, order 48213"))
}

func TestNewRecorder_RefusesNilDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		facts  emitter.Emitter
		logger libLog.Logger
	}{
		{name: "nil emitter", facts: nil, logger: libLog.NewNop()},
		{name: "nil logger", facts: &capturingEmitter{}, logger: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder, err := NewRecorder(tc.facts, tc.logger)

			require.Error(t, err)
			assert.Nil(t, recorder)
		})
	}
}

func TestRecorder_RecordTransactionCommitted_NotAFact_DoesNotEmit(t *testing.T) {
	t.Parallel()

	facts := &capturingEmitter{}
	recorder, err := NewRecorder(facts, libLog.NewNop())
	require.NoError(t, err)

	ctx, spans := recordingContext(t)
	hold := writeSet(constant.ActionHold,
		[]*operation.Operation{aliceDebit(1, "150.00"), externalCredit(2, "150.00")},
		withStatus(constant.PENDING))

	require.NoError(t, recorder.RecordTransactionCommitted(ctx, nil, hold))

	assert.Empty(t, facts.captured(), "a write set that is not a fact never reaches the kit")

	span := endedSpan(t, spans, "systemfacts.record_transaction_committed")
	assert.Contains(t, span.Attributes(), attribute.String("app.request.transaction_id", fixedTransactionID))
	assert.NotContains(t, span.Attributes(), attribute.String("app.systemfacts.fact_type", transactionCommittedFactType))
}

func TestRecorder_RecordTransactionCommitted_BuildsTheFactFromTheWriteSet(t *testing.T) {
	t.Parallel()

	facts := &capturingEmitter{}
	recorder, err := NewRecorder(facts, libLog.NewNop())
	require.NoError(t, err)

	ctx, spans := recordingContext(t)

	require.NoError(t, recorder.RecordTransactionCommitted(ctx, nil, outflowCommit()))

	captured := facts.captured()
	require.Len(t, captured, 1)

	fact := captured[0]
	transactionID := parseUUID(t, fixedTransactionID)

	assert.Equal(t, transactionCommittedFactType, fact.Type)
	assert.Equal(t, transactionCommittedSchemaVersion, fact.SchemaVersion)
	assert.Equal(t, TransactionCommittedFactID(transactionID), fact.FactID)
	assert.True(t, fact.OccurredAt.Equal(fixedUpdatedAt), "occurredAt is the completion's updated_at")
	assert.Equal(t, "UTC", fact.OccurredAt.Location().String())
	assert.Equal(t, emitter.Subject{Type: transactionSubjectType, ID: fixedTransactionID}, fact.Subject)
	assert.Equal(t, []emitter.Reference{{
		Type:  transactionIDReferenceType,
		Value: fixedTransactionID,
		Scope: map[string]string{scopeKeyOrganizationID: fixedOrganizationID, scopeKeyLedgerID: fixedLedgerID},
	}}, fact.References)

	payload, isPayload := fact.Data.(TransactionCommittedPayload)
	require.True(t, isPayload, "the data member is the typed payload")
	assert.Equal(t, constant.ActionCommit, payload.Action)

	span := endedSpan(t, spans, "systemfacts.record_transaction_committed")
	assert.Contains(t, span.Attributes(), attribute.String("app.systemfacts.fact_type", transactionCommittedFactType))
	assert.NotEqual(t, otelcodes.Error, span.Status().Code)
}

func TestRecorder_RecordTransactionCommitted_Refusal_CommitsWithoutTheFact(t *testing.T) {
	t.Parallel()

	refusal := fmt.Errorf("x: %w", emitter.ErrInvalidFact)
	facts := &capturingEmitter{err: refusal}
	logger, logs := recordingLogger(t)

	recorder, err := NewRecorder(facts, logger)
	require.NoError(t, err)

	ctx, spans := recordingContext(t)
	record := outflowCommit()

	require.NoError(t, recorder.RecordTransactionCommitted(ctx, nil, record), "a refusal never fails the business write")

	records := logs.snapshot()
	require.Len(t, records, 1, "exactly one line for the lost fact")
	assert.Equal(t, libLog.LevelWarn, records[0].level)
	assert.Equal(t, "System fact refused", records[0].msg)

	fields := map[string]any{}
	for _, field := range records[0].fields {
		fields[field.Key] = field.Value
	}

	assert.Equal(t, string(emitter.RefusalInvalidFact), fields["reason"])
	assert.Equal(t, fixedTransactionID, fields["transaction_id"])
	assert.Equal(t, refusal, fields["error"])

	for key, value := range fields {
		text := fmt.Sprint(value)
		assert.NotContains(t, text, record.Transaction.Description, "field %s leaks the description", key)
		assert.NotContains(t, text, "89.9", "field %s leaks the amount", key)
	}

	span := endedSpan(t, spans, "systemfacts.record_transaction_committed")
	assert.Equal(t, otelcodes.Error, span.Status().Code, "a refusal is a producer defect: technical class")
}

func TestRecorder_RecordTransactionCommitted_AlreadyRecorded_ReturnsSentinel(t *testing.T) {
	t.Parallel()

	facts := &capturingEmitter{err: fmt.Errorf("emitter: record fact: %w", outbox.ErrFactAlreadyRecorded)}
	logger, logs := recordingLogger(t)

	recorder, err := NewRecorder(facts, logger)
	require.NoError(t, err)

	ctx, _ := recordingContext(t)

	err = recorder.RecordTransactionCommitted(ctx, nil, outflowCommit())

	require.ErrorIs(t, err, ErrTransactionFactAlreadyRecorded)
	assert.Contains(t, err.Error(), fixedTransactionID)
	assert.Empty(t, logs.snapshot(), "an idempotent replay is the caller's decision, not a warning")
}

func TestRecorder_RecordTransactionCommitted_RecorderError_ReturnsWrapped(t *testing.T) {
	t.Parallel()

	driverErr := errors.New("connection reset")
	facts := &capturingEmitter{err: driverErr}

	recorder, err := NewRecorder(facts, libLog.NewNop())
	require.NoError(t, err)

	ctx, spans := recordingContext(t)

	err = recorder.RecordTransactionCommitted(ctx, nil, outflowCommit())

	require.ErrorIs(t, err, driverErr)
	assert.NotErrorIs(t, err, ErrTransactionFactAlreadyRecorded)

	span := endedSpan(t, spans, "systemfacts.record_transaction_committed")
	assert.Equal(t, otelcodes.Error, span.Status().Code)
}

func TestRecorder_RecordTransactionCommitted_NonUUIDTransactionID_ReturnsError(t *testing.T) {
	t.Parallel()

	facts := &capturingEmitter{}

	recorder, err := NewRecorder(facts, libLog.NewNop())
	require.NoError(t, err)

	record := outflowCommit()
	record.Transaction.ID = "not-a-uuid"

	err = recorder.RecordTransactionCommitted(context.Background(), nil, record)

	require.Error(t, err)
	assert.Empty(t, facts.captured(), "a fact without a valid subject never reaches the kit")
	assert.False(t, strings.Contains(err.Error(), record.Transaction.Description))
}
