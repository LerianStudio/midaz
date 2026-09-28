// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// capturedLogLine is one structured line a test logger recorded.
type capturedLogLine struct {
	Level  int
	Msg    string
	Fields string
}

// capturingLogger records every structured line so a test can assert on what an
// operator would actually see. Safe for concurrent use because the retrier logs
// from a background goroutine.
type capturingLogger struct {
	mu    sync.Mutex
	lines []capturedLogLine
}

func (l *capturingLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, capturedLogLine{Level: level, Msg: msg, Fields: fmt.Sprint(fields...)})
}

func (l *capturingLogger) With(_ ...any) libLog.Logger      { return l }
func (l *capturingLogger) WithGroup(_ string) libLog.Logger { return l }
func (l *capturingLogger) Enabled(_ int) bool               { return true }
func (l *capturingLogger) Sync(_ context.Context) error     { return nil }

// snapshot returns a copy of the recorded lines.
func (l *capturingLogger) snapshot() []capturedLogLine {
	l.mu.Lock()
	defer l.mu.Unlock()

	return append([]capturedLogLine(nil), l.lines...)
}

// atLevelOrMoreSevere returns the lines logged at the given level or a more
// severe one. lib-observability inverts the usual scale: Error=0, Warn=1,
// Info=2, Debug=3, so "at Warn or higher" is level <= LevelWarn.
func (l *capturingLogger) atLevelOrMoreSevere(level int) []capturedLogLine {
	var out []capturedLogLine

	for _, line := range l.snapshot() {
		if line.Level <= level {
			out = append(out, line)
		}
	}

	return out
}

// rendered joins a set of lines into one searchable blob.
func rendered(lines []capturedLogLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, fmt.Sprintf("level=%d msg=%q fields=%s", line.Level, line.Msg, line.Fields))
	}

	return strings.Join(parts, "\n")
}

// TestLostConfirmIsReportedWithTheTransactionAndAmount is the visibility
// contract. A confirm the tracer did not accept means a committed spend the
// customer's limit has not counted; the line an operator reads has to say which
// transaction and how much, because the tracer's own store — the only other
// place that mapping exists — may be exactly what is unreachable.
func TestLostConfirmIsReportedWithTheTransactionAndAmount(t *testing.T) {
	withFastSharedRetrier(t)

	ctx := context.Background()
	_, span := noop.NewTracerProvider().Tracer("t").Start(ctx, "test")

	transactionID := uuid.New()
	amount := decimal.RequireFromString("1234.56")
	settings := mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, TimeoutMs: stubContextTracerTimeoutMs}

	for name, confirm := range map[string]func(uc *UseCase, logger libLog.Logger){
		"after a direct create": func(uc *UseCase, logger libLog.Logger) {
			uc.confirmReservations(ctx, span, logger, reservationHandle{
				ContextAttempt: &ContextTracerAttempt{Dispatched: true, Settings: settings},
				TransactionID:  transactionID,
				Amount:         amount,
				Asset:          "BRL",
			})
		},
		"at commit": func(uc *UseCase, logger libLog.Logger) {
			uc.confirmReservationsByTransaction(ctx, span, logger, settings,
				reservationHandle{TransactionID: transactionID, Amount: amount, Asset: "BRL"}, false)
		},
	} {
		t.Run(name, func(t *testing.T) {
			logger := &capturingLogger{}
			stub := &stubContextTracer{completeErr: fmt.Errorf("down: %w", tracer.ErrTracerUnavailable)}
			uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}

			confirm(uc, logger)

			reported := rendered(logger.atLevelOrMoreSevere(libLog.LevelWarn))
			require.NotEmpty(t, reported, "a lost confirm must be reported at Warn or a more severe level")

			assert.Contains(t, reported, transactionID.String(), "the report must name the transaction whose spend went uncounted")
			assert.Contains(t, reported, "1234.56", "the report must name how much spending went uncounted")
			assert.Contains(t, reported, "BRL", "the report must name the asset the amount is denominated in")
		})
	}
}

// TestReserveHandleCarriesTheIdentityItWillNeedToReport proves the identity is
// captured at reserve time rather than reconstructed later: by the time a
// confirm fails, the only thing in hand is the handle.
func TestReserveHandleCarriesTheIdentityItWillNeedToReport(t *testing.T) {
	ctx, span, logger := anchorDeps()

	stub := &stubContextTracer{}
	uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}
	transaction, validated, balances := anchorPrepared()
	input := anchorInput(enforceSettings(mmodel.TracerFailPostureOpen), false)
	input.Amount = decimal.RequireFromString("42.50")

	out := uc.reservePreparedTransaction(ctx, span, logger, input, transaction, validated, balances)

	require.Equal(t, reservationProceed, out.Kind)
	assert.Equal(t, input.Key.TransactionID, out.Handle.TransactionID)
	assert.True(t, decimal.RequireFromString("42.50").Equal(out.Handle.Amount))
	assert.Equal(t, "BRL", out.Handle.Asset)

	transition := out.Handle.transitionByTransaction(reservationActionConfirm)
	assert.Equal(t, reservationActionConfirm, transition.Action)
	assert.Equal(t, input.Key.TransactionID, transition.TransactionID)
}
