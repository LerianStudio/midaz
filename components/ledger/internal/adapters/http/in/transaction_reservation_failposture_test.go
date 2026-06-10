// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// F3-T19 — fail-posture PROOFS (Gates 4 and 5). These complement the
// branch-Kind assertions in transaction_reservation_anchor_test.go with the two
// behaviors the spec names explicitly:
//
//	Gate 4 (fail-open):  enforce + unavailable tracer COMMITS and the SKIPPED
//	                     decision is recorded. On a timeout the tracer never
//	                     received the call, so no tracer-side audit row can
//	                     exist; the ledger's own record of the skip IS the
//	                     `app.tracer.reservation_skipped=true` span attribute set
//	                     by handleReserveError. This test captures it with a real
//	                     recording span (the noop span used elsewhere discards
//	                     attributes) and proves fail-closed does NOT set it.
//	Gate 5 (fail-closed): enforce + unavailable tracer REJECTS, and the create
//	                     seam releases the idempotency key + removes the
//	                     Redis-queue seed BEFORE — and instead of —
//	                     ProcessBalanceOperations, so no balance is mutated. The
//	                     reject-Kind is proven at the helper level; the call-site
//	                     mechanics (idempotency release + no balance commit) are a
//	                     structural guarantee asserted directly over the live
//	                     executeCreateTransaction source AST, mirroring the
//	                     fee-seam structural gate. A "bites" fixture proves the
//	                     gate fails if the release is dropped or the reject falls
//	                     through to the balance commit.

// recordingSpan returns a ctx, the real SDK span the helper writes into, and an
// `ended` closure that ends the span and returns the recorded spans. Unlike
// anchorDeps's noop span, the SDK span retains SetAttributes calls so the
// SKIPPED marker can be asserted.
func recordingSpan(t *testing.T) (context.Context, trace.Span, func() []sdktrace.ReadOnlySpan) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx, span := tp.Tracer("failposture-test").Start(context.Background(), "reserve")

	ended := func() []sdktrace.ReadOnlySpan {
		span.End()
		return recorder.Ended()
	}

	return ctx, span, ended
}

// spanHasSkippedMarker reports whether any ended span carries
// app.tracer.reservation_skipped=true.
func spanHasSkippedMarker(spans []sdktrace.ReadOnlySpan) bool {
	for _, s := range spans {
		for _, kv := range s.Attributes() {
			if kv.Key == attribute.Key("app.tracer.reservation_skipped") && kv.Value.AsBool() {
				return true
			}
		}
	}

	return false
}

// TestTracerFailOpenSkipped — Gate 4. enforce + unavailable tracer proceeds
// (COMMITS) and records the SKIPPED decision on the span.
func TestTracerFailOpenSkipped(t *testing.T) {
	ctx, span, ended := recordingSpan(t)

	logger := &libLog.NopLogger{}
	reserver := &stubReserver{reserveErr: fmt.Errorf("timeout: %w", tracer.ErrTracerUnavailable)}
	handler := &TransactionHandler{TracerReserver: reserver}

	out := handler.reserveTransaction(ctx, span, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault)

	assert.Equal(t, reservationProceed, out.Kind, "fail-open must COMMIT (proceed) when the tracer is unavailable")
	assert.Empty(t, out.Handle.ReservationIDs, "no reservation is held when the reserve call never succeeded")

	require.True(t, spanHasSkippedMarker(ended()),
		"fail-open must record the SKIPPED decision via app.tracer.reservation_skipped=true")
}

// TestTracerFailClosedDoesNotMarkSkipped is the discriminator: fail-closed
// rejects rather than skips, so it must NOT set the SKIPPED marker — otherwise
// the Gate-4 assertion above would pass vacuously.
func TestTracerFailClosedDoesNotMarkSkipped(t *testing.T) {
	ctx, span, ended := recordingSpan(t)

	logger := &libLog.NopLogger{}
	reserver := &stubReserver{reserveErr: fmt.Errorf("timeout: %w", tracer.ErrTracerUnavailable)}
	handler := &TransactionHandler{TracerReserver: reserver}

	out := handler.reserveTransaction(ctx, span, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault)

	require.Equal(t, reservationReject, out.Kind)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, out.Err, &unavailable)
	assert.Equal(t, constant.ErrTransactionReservationUnavailable.Error(), unavailable.Code)

	assert.False(t, spanHasSkippedMarker(ended()),
		"fail-closed rejects; it must NOT record the SKIPPED marker")
}

// ---- Gate 5 (fail-closed): structural proof of the call-site mechanics --------

const createSeamFuncName = "executeCreateTransaction"

// failClosedSeamMetrics captures the statement-list ordering facts the Gate-5
// structural assertion relies on, all within executeCreateTransaction.
type failClosedSeamMetrics struct {
	reservePos          int  // index of the reserveTransaction call (-1 if absent)
	rejectDeleteIdemp   bool // deleteIdempotencyKey appears inside the reservationReject branch
	rejectRemoveRedis   bool // RemoveTransactionFromRedisQueue appears inside the reject branch
	rejectReturnsBefore bool // the reject branch returns (no fall-through to the balance commit)
	processBalancePos   int  // index of the top-level ProcessBalanceOperations call (-1)
}

// analyzeFailClosedSeam walks executeCreateTransaction and extracts the ordering
// and reject-branch facts. The reservationReject guard is an `if` whose cond is
// `reservation.Kind == reservationReject`; the release mechanics must live in
// that block and the block must return.
func analyzeFailClosedSeam(t *testing.T, src string) failClosedSeamMetrics {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var fn *ast.FuncDecl

	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == createSeamFuncName {
			fn = d
			break
		}
	}

	if fn == nil || fn.Body == nil {
		t.Fatalf("function %q not found or has no body", createSeamFuncName)
	}

	m := failClosedSeamMetrics{reservePos: -1, processBalancePos: -1}

	for i, stmt := range fn.Body.List {
		if m.reservePos == -1 && stmtCallsMethod(stmt, "reserveTransaction") {
			m.reservePos = i
		}

		if m.processBalancePos == -1 && stmtCallsMethod(stmt, "ProcessBalanceOperations") {
			m.processBalancePos = i
		}

		if ifStmt, ok := stmt.(*ast.IfStmt); ok && isReservationRejectGuard(ifStmt) {
			m.rejectDeleteIdemp = blockCallsMethod(ifStmt.Body, "deleteIdempotencyKey")
			m.rejectRemoveRedis = blockCallsMethod(ifStmt.Body, "RemoveTransactionFromRedisQueue")
			m.rejectReturnsBefore = blockEndsInReturn(ifStmt.Body)
		}
	}

	return m
}

// isReservationRejectGuard reports whether the if-cond is
// `reservation.Kind == reservationReject` (the fail-closed / denied reject gate
// that must precede the balance commit).
func isReservationRejectGuard(ifStmt *ast.IfStmt) bool {
	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return false
	}

	// RHS must reference the reservationReject identifier.
	rhs, ok := bin.Y.(*ast.Ident)
	if !ok || rhs.Name != "reservationReject" {
		return false
	}

	// LHS must be reservation.Kind.
	sel, ok := bin.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Kind" {
		return false
	}

	x, ok := sel.X.(*ast.Ident)

	return ok && x.Name == "reservation"
}

// blockCallsMethod reports whether the block contains a selector call to the
// named method.
func blockCallsMethod(block *ast.BlockStmt, method string) bool {
	for _, stmt := range block.List {
		if stmtCallsMethod(stmt, method) {
			return true
		}
	}

	return false
}

// blockEndsInReturn reports whether the last statement of the block is a return
// — proving the reject branch does not fall through to the balance commit.
func blockEndsInReturn(block *ast.BlockStmt) bool {
	if len(block.List) == 0 {
		return false
	}

	_, ok := block.List[len(block.List)-1].(*ast.ReturnStmt)

	return ok
}

// TestTracerFailClosedReject_ReleasesIdempotencyAndSkipsBalanceCommit — Gate 5.
// The fail-closed reject (proven at the helper in
// TestReserveTransaction_FailClosed_Rejects and
// TestTracerFailClosedDoesNotMarkSkipped) must, at the call site, release the
// idempotency key and the Redis-queue seed and return BEFORE
// ProcessBalanceOperations — so no balance is mutated. Asserted over the live
// source so a future reorder that drops the release or falls through to the
// balance commit fails this gate.
func TestTracerFailClosedReject_ReleasesIdempotencyAndSkipsBalanceCommit(t *testing.T) {
	src := readSeamSource(t) // reads transaction_create.go (shared with the fee-seam gate)

	m := analyzeFailClosedSeam(t, src)

	require.NotEqual(t, -1, m.reservePos, "reserveTransaction call not found in executeCreateTransaction")
	require.NotEqual(t, -1, m.processBalancePos, "ProcessBalanceOperations call not found")

	assert.Less(t, m.reservePos, m.processBalancePos,
		"the reserve anchor must precede the balance commit (reject before any balance move)")

	assert.True(t, m.rejectDeleteIdemp,
		"fail-closed reject branch must release the idempotency key (deleteIdempotencyKey)")
	assert.True(t, m.rejectRemoveRedis,
		"fail-closed reject branch must remove the Redis-queue seed (RemoveTransactionFromRedisQueue)")
	assert.True(t, m.rejectReturnsBefore,
		"fail-closed reject branch must return — it must NOT fall through to ProcessBalanceOperations")
}

// TestTracerFailClosedSeam_Bites proves the Gate-5 analyzer actually fails on a
// reject branch that drops the idempotency release or falls through to the
// balance commit — a gate that cannot bite is not a guard.
func TestTracerFailClosedSeam_Bites(t *testing.T) {
	// Fixture 1: reject branch missing deleteIdempotencyKey and the return.
	leaky := `package in
func (handler *TransactionHandler) executeCreateTransaction() error {
	reservation := handler.reserveTransaction()
	if reservation.Kind == reservationReject {
		// BUG: neither releases the idempotency key nor returns
		_ = reservation.Err
	}
	result, err := handler.Command.ProcessBalanceOperations()
	_ = result
	return err
}`

	m := analyzeFailClosedSeam(t, leaky)

	if m.reservePos == -1 || m.processBalancePos == -1 {
		t.Fatalf("Gate 5 fixture sanity: missing positions reserve=%d processBalance=%d", m.reservePos, m.processBalancePos)
	}

	if m.rejectDeleteIdemp {
		t.Error("Gate 5 failed to bite: a reject branch with no deleteIdempotencyKey was reported as releasing the key")
	}

	if m.rejectReturnsBefore {
		t.Error("Gate 5 failed to bite: a reject branch with no return was reported as returning before the balance commit")
	}

	// Fixture 2: the canonical, correct shape must pass all three reject facts.
	correct := `package in
func (handler *TransactionHandler) executeCreateTransaction() error {
	reservation := handler.reserveTransaction()
	if reservation.Kind == reservationReject {
		handler.deleteIdempotencyKey()
		handler.Command.RemoveTransactionFromRedisQueue()
		return handler.WithError(reservation.Err)
	}
	result, err := handler.Command.ProcessBalanceOperations()
	_ = result
	return err
}`

	mc := analyzeFailClosedSeam(t, correct)
	if !(mc.rejectDeleteIdemp && mc.rejectRemoveRedis && mc.rejectReturnsBefore) {
		t.Errorf("Gate 5 fixture sanity: the correct shape was not fully recognized: delete=%v removeRedis=%v returns=%v",
			mc.rejectDeleteIdemp, mc.rejectRemoveRedis, mc.rejectReturnsBefore)
	}

	if !(mc.reservePos < mc.processBalancePos) {
		t.Error("Gate 5 fixture sanity: reserve should precede the balance commit in the correct shape")
	}
}
