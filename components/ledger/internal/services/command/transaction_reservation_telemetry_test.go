// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// alreadyReleasedAmount carries non-zero decimals so it cannot appear inside a
// random uuid's hex: asserting it is absent from the log proves no amount leaked.
const alreadyReleasedAmount = "1234.56"

func alreadyReleasedIdentity(transactionID uuid.UUID) reservationHandle {
	return reservationHandle{
		TransactionID: transactionID,
		Amount:        decimal.RequireFromString(alreadyReleasedAmount),
		Asset:         "BRL",
	}
}

// alreadyReleasedSeries collects tracer_reservation_confirm_already_released_total
// keyed by its operation attribute, asserting the series carries nothing else.
func alreadyReleasedSeries(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	values := make(map[string]int64)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "tracer_reservation_confirm_already_released_total" {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				attrs := dp.Attributes.ToSlice()
				require.Len(t, attrs, 1, "the series carries only the operation attribute")
				require.Equal(t, "operation", string(attrs[0].Key))

				values[attrs[0].Value.AsString()] = dp.Value
			}
		}
	}

	return values
}

// alreadyReleasedEvents counts the confirm_already_released span events across
// the recorded spans, and reports whether any span ended red.
func alreadyReleasedEvents(spans []sdktrace.ReadOnlySpan) (count int, anyError bool) {
	for _, s := range spans {
		if s.Status().Code == codes.Error {
			anyError = true
		}

		for _, ev := range s.Events() {
			if ev.Name == "tracer.reservation.confirm_already_released" {
				count++
			}
		}
	}

	return count, anyError
}

func alreadyReleasedWarns(logger *capturingLogger) []capturedLogLine {
	var out []capturedLogLine

	for _, line := range logger.atLevelOrMoreSevere(libLog.LevelWarn) {
		if strings.Contains(line.Msg, "already released") {
			out = append(out, line)
		}
	}

	return out
}

func TestConfirmReservationsByTransaction_AlreadyReleased(t *testing.T) {
	enforce := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen}

	t.Run("released rows are flagged once and the confirm is not retried", func(t *testing.T) {
		withFastSharedRetrier(t)

		reader, factory := newReaderFactory(t)
		ctx, span, ended := recordingSpan(t)
		logger := &capturingLogger{}
		txID := uuid.New()
		reserver := &stubReserver{confirmByTxnOutcome: tracer.ConfirmOutcome{Confirmed: 0, AlreadyReleased: 2}}
		uc := &UseCase{TracerReserver: reserver, MetricsFactory: factory}

		uc.confirmReservationsByTransaction(ctx, span, logger, enforce, alreadyReleasedIdentity(txID), false)

		sharedReservationRetrier.wait()

		assert.Equal(t, []uuid.UUID{txID}, reserver.confirmedTransactions(),
			"a confirm that found released rows is not retried: the retry cannot count the spend")

		warns := alreadyReleasedWarns(logger)
		require.Len(t, warns, 1)
		assert.Equal(t, libLog.LevelWarn, warns[0].Level)
		assert.Contains(t, warns[0].Fields, txID.String())
		assert.Contains(t, warns[0].Fields, "already_released")
		assert.NotContains(t, rendered(logger.snapshot()), alreadyReleasedAmount, "no amount reaches the log")

		events, red := alreadyReleasedEvents(ended())
		assert.Equal(t, 1, events)
		assert.False(t, red, "a business observation keeps the span green")

		assert.Equal(t, map[string]int64{reservationConfirmOperationByTransaction: 1}, alreadyReleasedSeries(t, reader))
	})

	t.Run("a clean confirm records nothing", func(t *testing.T) {
		withFastSharedRetrier(t)

		reader, factory := newReaderFactory(t)
		ctx, span, ended := recordingSpan(t)
		logger := &capturingLogger{}
		reserver := &stubReserver{confirmByTxnOutcome: tracer.ConfirmOutcome{Confirmed: 1, AlreadyReleased: 0}}
		uc := &UseCase{TracerReserver: reserver, MetricsFactory: factory}

		uc.confirmReservationsByTransaction(ctx, span, logger, enforce, alreadyReleasedIdentity(uuid.New()), false)

		sharedReservationRetrier.wait()

		assert.Empty(t, alreadyReleasedWarns(logger))

		events, _ := alreadyReleasedEvents(ended())
		assert.Zero(t, events)
		assert.Empty(t, alreadyReleasedSeries(t, reader))
	})

	t.Run("a nil metrics factory still logs and records the event", func(t *testing.T) {
		withFastSharedRetrier(t)

		ctx, span, ended := recordingSpan(t)
		logger := &capturingLogger{}
		reserver := &stubReserver{confirmByTxnOutcome: tracer.ConfirmOutcome{AlreadyReleased: 1}}
		uc := &UseCase{TracerReserver: reserver}

		uc.confirmReservationsByTransaction(ctx, span, logger, enforce, alreadyReleasedIdentity(uuid.New()), false)

		assert.Len(t, alreadyReleasedWarns(logger), 1)

		events, _ := alreadyReleasedEvents(ended())
		assert.Equal(t, 1, events)
	})
}

func TestConfirmReservations_AlreadyReleased(t *testing.T) {
	t.Run("a released reservation is flagged under the by-id operation", func(t *testing.T) {
		withFastSharedRetrier(t)

		reader, factory := newReaderFactory(t)
		ctx, span, ended := recordingSpan(t)
		logger := &capturingLogger{}
		txID := uuid.New()
		id := uuid.New()
		reserver := &stubReserver{confirmOutcome: tracer.ConfirmOutcome{Confirmed: 0, AlreadyReleased: 1}}
		uc := &UseCase{TracerReserver: reserver, MetricsFactory: factory}

		handle := alreadyReleasedIdentity(txID)
		handle.ReservationIDs = []uuid.UUID{id}

		uc.confirmReservations(ctx, span, logger, handle)

		sharedReservationRetrier.wait()

		assert.Equal(t, []uuid.UUID{id}, reserver.confirmed(), "not retried")

		warns := alreadyReleasedWarns(logger)
		require.Len(t, warns, 1)
		assert.Contains(t, warns[0].Fields, txID.String())
		assert.NotContains(t, rendered(logger.snapshot()), alreadyReleasedAmount)

		events, red := alreadyReleasedEvents(ended())
		assert.Equal(t, 1, events)
		assert.False(t, red)

		assert.Equal(t, map[string]int64{reservationConfirmOperationByID: 1}, alreadyReleasedSeries(t, reader))
	})

	t.Run("each released reservation of a multi-id handle is flagged on its own", func(t *testing.T) {
		withFastSharedRetrier(t)

		reader, factory := newReaderFactory(t)
		ctx, span, ended := recordingSpan(t)
		logger := &capturingLogger{}
		txID := uuid.New()
		ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
		released := tracer.ConfirmOutcome{Confirmed: 0, AlreadyReleased: 1}
		reserver := &stubReserver{
			confirmOutcome:     tracer.ConfirmOutcome{Confirmed: 1},
			confirmOutcomeByID: map[uuid.UUID]tracer.ConfirmOutcome{ids[0]: released, ids[2]: released},
		}
		uc := &UseCase{TracerReserver: reserver, MetricsFactory: factory}

		handle := alreadyReleasedIdentity(txID)
		handle.ReservationIDs = ids

		uc.confirmReservations(ctx, span, logger, handle)

		sharedReservationRetrier.wait()

		assert.Equal(t, ids, reserver.confirmed(), "each id confirmed once, none retried")

		warns := alreadyReleasedWarns(logger)
		require.Len(t, warns, 2)

		for _, warn := range warns {
			assert.Contains(t, warn.Fields, txID.String())
		}

		assert.NotContains(t, rendered(logger.snapshot()), alreadyReleasedAmount)

		events, red := alreadyReleasedEvents(ended())
		assert.Equal(t, 2, events)
		assert.False(t, red)

		assert.Equal(t, map[string]int64{reservationConfirmOperationByID: 2}, alreadyReleasedSeries(t, reader))
	})

	t.Run("a clean confirm records nothing", func(t *testing.T) {
		withFastSharedRetrier(t)

		reader, factory := newReaderFactory(t)
		ctx, span, ended := recordingSpan(t)
		logger := &capturingLogger{}
		reserver := &stubReserver{confirmOutcome: tracer.ConfirmOutcome{Confirmed: 1}}
		uc := &UseCase{TracerReserver: reserver, MetricsFactory: factory}

		handle := alreadyReleasedIdentity(uuid.New())
		handle.ReservationIDs = []uuid.UUID{uuid.New(), uuid.New()}

		uc.confirmReservations(ctx, span, logger, handle)

		assert.Empty(t, alreadyReleasedWarns(logger))

		events, _ := alreadyReleasedEvents(ended())
		assert.Zero(t, events)
		assert.Empty(t, alreadyReleasedSeries(t, reader))
	})
}

// TestRetryConfirm_AlreadyReleased covers the off-path redelivery: a confirm
// that lands on retry and finds released rows takes the same branch as the
// inline one, on the retry's own span.
func TestRetryConfirm_AlreadyReleased(t *testing.T) {
	tests := []struct {
		name       string
		transition func() reservationTransition
		outcome    tracer.ConfirmOutcome
		want       map[string]int64
	}{
		{
			name: "by transaction",
			transition: func() reservationTransition {
				return alreadyReleasedIdentity(uuid.New()).transitionByTransaction(reservationActionConfirm)
			},
			outcome: tracer.ConfirmOutcome{Confirmed: 0, AlreadyReleased: 2},
			want:    map[string]int64{reservationConfirmOperationByTransaction: 1},
		},
		{
			name: "by id",
			transition: func() reservationTransition {
				handle := alreadyReleasedIdentity(uuid.New())
				handle.ReservationIDs = []uuid.UUID{uuid.New()}

				return handle.transitions(reservationActionConfirm)[0]
			},
			outcome: tracer.ConfirmOutcome{Confirmed: 0, AlreadyReleased: 1},
			want:    map[string]int64{reservationConfirmOperationByID: 1},
		},
		{
			name: "clean confirm on retry",
			transition: func() reservationTransition {
				return alreadyReleasedIdentity(uuid.New()).transitionByTransaction(reservationActionConfirm)
			},
			outcome: tracer.ConfirmOutcome{Confirmed: 1},
			want:    map[string]int64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, factory := newReaderFactory(t)

			recorder := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			ctx := libObservability.ContextWithTracer(context.Background(), tp.Tracer("retry-test"))

			logger := &capturingLogger{}
			reserver := &scriptedReserver{confirm: failNTimes(1), outcome: tt.outcome}
			retrier := newReservationRetrier(fastRetryPolicy())

			retrier.schedule(ctx, reserver, factory, logger, tt.transition(), tracer.ErrTracerUnavailable)
			retrier.wait()

			attempts, delivered := reserver.attempts()
			require.True(t, delivered)
			assert.Equal(t, 2, attempts, "one refusal then acceptance; an already-released outcome schedules nothing further")

			wantEvents := len(tt.want)

			assert.Len(t, alreadyReleasedWarns(logger), wantEvents)
			assert.NotContains(t, strings.Join(linesFields(alreadyReleasedWarns(logger)), " "), alreadyReleasedAmount)

			events, _ := alreadyReleasedEvents(recorder.Ended())
			assert.Equal(t, wantEvents, events)

			assert.Equal(t, tt.want, alreadyReleasedSeries(t, reader))
		})
	}
}

func TestRecordReservationConfirmOutcome_IsNilSafe(t *testing.T) {
	ctx, span, _ := anchorDeps()

	assert.NotPanics(t, func() {
		recordReservationConfirmOutcome(ctx, span, nil, &libLog.NopLogger{},
			alreadyReleasedIdentity(uuid.New()).transitionByTransaction(reservationActionConfirm),
			tracer.ConfirmOutcome{AlreadyReleased: 3})
	})
}

func linesFields(lines []capturedLogLine) []string {
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		out = append(out, line.Fields)
	}

	return out
}
