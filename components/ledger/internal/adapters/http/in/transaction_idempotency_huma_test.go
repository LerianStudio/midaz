// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"os"
	"reflect"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Money-write idempotency parity gates (Wave 4 self-heal). Two facts the pinning
// suite left uncovered, both silent money-path regressions:
//
//  1. The Huma CREATE envelopes must bind the idempotency headers under the SAME
//     names the Fiber path (http.GetIdempotencyKeyAndTTL) reads — the lib-commons
//     canonical "X-Idempotency" / "X-TTL". Huma binds header params by the literal
//     `header:` tag (huma.go: value = ctx.Header(p.Name)); a wrong tag silently
//     drops the caller's stable key, downgrading dedup to payload-hash and letting
//     a keyed retry with a tweaked body mutate balances twice.
//  2. The revert core must forward BOTH idempotency arguments the pre-migration Fiber
//     path resolved: the TTL through ParseIdempotencyTTL (an absent X-TTL defaults to
//     300, never 0 — a 0 TTL reaches SetNX(..., 0*time.Second), which go-redis emits
//     as `SET key val NX` with no expiry, leaking a permanent key and changing the
//     >5-minute replay/conflict semantics of every revert) and the origin-scoped hash
//     source through revertIdempotencyHashSource.

// createHumaEnvelopes enumerates the CREATE request structs whose idempotency header
// tags feed the money-write dedup. All four transaction CREATE ops share these two
// structs; holder/instrument carry the same headers and the same drift risk.
func createHumaEnvelopes() map[string]any {
	return map[string]any{
		"CreateTransactionJSONInputHuma":    CreateTransactionJSONInputHuma{},
		"CreateTransactionInflowInputHuma":  CreateTransactionInflowInputHuma{},
		"CreateTransactionOutflowInputHuma": CreateTransactionOutflowInputHuma{},
		"CreateHolderInputHuma":             CreateHolderInputHuma{},
		"CreateInstrumentInputHuma":         CreateInstrumentInputHuma{},
	}
}

// headerTag returns the `header:` struct tag of the named field, or "".
func headerTag(t reflect.Type, field string) string {
	f, ok := t.FieldByName(field)
	if !ok {
		return ""
	}

	return f.Tag.Get("header")
}

// TestHuma_CreateEnvelopes_CanonicalIdempotencyHeaders proves every Huma CREATE
// envelope binds the idempotency headers under the canonical lib-commons names the
// Fiber path reads. Because Huma binds by literal tag name, any drift here silently
// drops the caller's idempotency key on the money-write path.
func TestHuma_CreateEnvelopes_CanonicalIdempotencyHeaders(t *testing.T) {
	t.Parallel()

	for name, env := range createHumaEnvelopes() {
		typ := reflect.TypeOf(env)

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, libConstants.IdempotencyKey, headerTag(typ, "IdempotencyKey"),
				"IdempotencyKey header tag must equal libConstants.IdempotencyKey (%q) — the name the Fiber path reads", libConstants.IdempotencyKey)
			assert.Equal(t, libConstants.IdempotencyTTL, headerTag(typ, "IdempotencyTTL"),
				"IdempotencyTTL header tag must equal libConstants.IdempotencyTTL (%q) — the name the Fiber path reads", libConstants.IdempotencyTTL)
		})
	}
}

// TestRevertTransaction_ForwardsIdempotencyCallArgs is a cheap unit-tier TRIPWIRE over the
// live source AST: it asserts the revert core still resolves each idempotency argument through
// the helper that gives it its contract, at the position createRevertTransaction expects. The
// arguments are addressed positionally because both sit in the middle of the signature
// (ctx, params, transactionInput, transactionStatus, idempotencyKey, idempotencyTTL,
// idempotencyHashSource); each row asserts POSITIVELY which helper the argument must resolve
// to, so a shifted position fails the row instead of silently passing a negative check.
//
// The behavioral proof lives in the integration suite —
// TestIntegration_TransactionV2Revert_IdempotencyScopedByOrigin (per-origin scoping and the
// repeat-revert invariant) and TestIntegration_TransactionV2Revert_ConcurrentSingleWinner
// (the shared claim serializing concurrent reverts of one origin). This gate only catches the
// call-site regression before the containers boot.
//
// Deliberate limitation: it matches the argument SYNTACTICALLY. Either helper call may be
// qualified (`pkg.F(...)`) or plain (`F(...)`), but hoisting one into a local and passing the
// variable trips the gate even though behavior is unchanged. Following the assignment would
// turn a syntax check into dataflow analysis; if a refactor needs the local, delete the
// affected row here and lean on the integration tests named above.
func TestRevertTransaction_ForwardsIdempotencyCallArgs(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile(stateHandlersFile)
	require.NoError(t, err, "read %s", stateHandlersFile)

	fn := findFuncDecl(t, string(src), "revertTransaction")
	call := findAssignedCall(t, fn, "createRevertTransaction")

	args := []struct {
		name     string
		position int
		wantFunc string
		why      string
	}{
		{
			name:     "idempotency TTL resolves through ParseIdempotencyTTL",
			position: 5,
			wantFunc: "ParseIdempotencyTTL",
			why:      `revert must resolve its idempotency TTL through ParseIdempotencyTTL (an absent X-TTL defaults to 300); a literal 0 makes the Redis idempotency key permanent`,
		},
		{
			name:     "idempotency hash source resolves through revertIdempotencyHashSource",
			position: 6,
			wantFunc: "revertIdempotencyHashSource",
			why:      `revert must key its idempotency slot through revertIdempotencyHashSource(originID); hashing the reversal payload instead re-opens the cross-origin replay, because that payload carries NO reference to the origin`,
		},
	}

	for _, tt := range args {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Greater(t, len(call.Args), tt.position,
				"createRevertTransaction call must pass an argument at position %d", tt.position)

			inner, isCall := call.Args[tt.position].(*ast.CallExpr)
			require.Truef(t, isCall, "argument %d must be a %s(...) call: %s", tt.position, tt.wantFunc, tt.why)

			assert.Equalf(t, tt.wantFunc, calleeName(inner), "%s", tt.why)
		})
	}
}

// TestRevertIdempotencyHashSource_GoldenPreimage locks the revert idempotency preimage to an
// exact string per origin. The golden values ARE the two halves of the contract: the same
// origin always yields the same preimage (so two concurrent reverts of one origin collide on
// one SetNX — the only thing serializing the GetParentByTransactionID read-then-act race), and
// distinct origins yield distinct preimages (so two economically-identical origins never share
// a slot). Pinning the exact bytes also matters because the preimage is a live Redis key input:
// changing it silently re-slots every in-flight revert across a deploy.
func TestRevertIdempotencyHashSource_GoldenPreimage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin uuid.UUID
		want   string
	}{
		{
			name:   "origin A",
			origin: uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb50c"),
			want:   "revert\x00019fae1d-8423-75b8-91c2-0b37952eb50c",
		},
		{
			name:   "origin B differing only in the last nibble",
			origin: uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb50d"),
			want:   "revert\x00019fae1d-8423-75b8-91c2-0b37952eb50d",
		},
		{
			name:   "nil origin",
			origin: uuid.Nil,
			want:   "revert\x0000000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, revertIdempotencyHashSource(tt.origin),
				"the revert preimage must be the NUL-joined revert action discriminator followed by the origin transaction id")
		})
	}

	// The golden strings above are spelled out literally so the test fails on a constant
	// rename that changes the VALUE; these bind them to the constants they are built from.
	assert.Equal(t, "revert", constant.ActionRevert,
		"the golden preimages above encode constant.ActionRevert — changing its value re-slots every live revert idempotency key")
	assert.Equal(t, "\x00", idempotencyDiscriminatorSep,
		"the golden preimages above encode idempotencyDiscriminatorSep, shared with v2IdempotencyHashSource")
}

// findAssignedCall returns the createRevertTransaction CallExpr from the function body
// (it is the RHS of a multi-value assignment: tranReverted, replayed, err := handler.createRevertTransaction(...)).
func findAssignedCall(t *testing.T, fn *ast.FuncDecl, method string) *ast.CallExpr {
	t.Helper()

	var found *ast.CallExpr

	ast.Inspect(fn, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
				found = call
			}
		}

		return true
	})

	require.NotNil(t, found, "%s call not found", method)

	return found
}

// calleeName returns the called function's name for a plain call (`f(...)`) or a qualified one
// (`pkg.F(...)` / `x.M(...)`), or "" for anything else. Accepting both forms keeps the gate
// pointed at WHICH helper produces the argument, not at how the helper is spelled.
func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	default:
		return ""
	}
}
