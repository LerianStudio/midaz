// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"os"
	"reflect"
	"strings"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
//  2. The revert core must resolve the idempotency TTL the same way the pre-migration
//     Fiber path did: revert carries no X-TTL, so ParseIdempotencyTTL("") == 300, NOT
//     a hardcoded 0. A 0 TTL reaches SetNX(..., 0*time.Second), which go-redis emits
//     as `SET key val NX` (no expiry) — a permanent idempotency key that leaks and
//     changes the >5-minute replay/conflict semantics of every revert.

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
	for name, env := range createHumaEnvelopes() {
		typ := reflect.TypeOf(env)

		t.Run(name, func(t *testing.T) {
			assert.Equal(t, libConstants.IdempotencyKey, headerTag(typ, "IdempotencyKey"),
				"IdempotencyKey header tag must equal libConstants.IdempotencyKey (%q) — the name the Fiber path reads", libConstants.IdempotencyKey)
			assert.Equal(t, libConstants.IdempotencyTTL, headerTag(typ, "IdempotencyTTL"),
				"IdempotencyTTL header tag must equal libConstants.IdempotencyTTL (%q) — the name the Fiber path reads", libConstants.IdempotencyTTL)
		})
	}
}

// revertTTLArgIndex is the position of idempotencyTTL in the createRevertTransaction
// signature (ctx, params, transactionInput, transactionStatus, idempotencyKey,
// idempotencyTTL, idempotencyHashSource...). The TTL is addressed positionally, not as the
// last argument, because the trailing variadic hash-source override sits after it.
const revertTTLArgIndex = 5

// TestRevertTransaction_DoesNotHardcodeZeroTTL proves the revert core does not pass a
// bare `0` literal as the idempotency TTL to createRevertTransaction. The pre-migration
// Fiber revert defaulted the TTL to 300 (ParseIdempotencyTTL("")); a literal 0 makes the
// Redis idempotency key permanent. Asserted over the live source AST (mirrors the fee-seam
// and tracer-skip call-site gates) since the TTL never surfaces on the transport response.
func TestRevertTransaction_DoesNotHardcodeZeroTTL(t *testing.T) {
	src, err := os.ReadFile(stateHandlersFile)
	require.NoError(t, err, "read %s", stateHandlersFile)

	fn := findFuncDecl(t, string(src), "revertTransaction")

	call := findAssignedCall(t, fn, "createRevertTransaction")
	require.Greater(t, len(call.Args), revertTTLArgIndex,
		"createRevertTransaction call must pass an idempotency TTL argument at position %d", revertTTLArgIndex)

	if lit, isLit := call.Args[revertTTLArgIndex].(*ast.BasicLit); isLit {
		assert.NotEqual(t, "0", lit.Value,
			`revert must not hardcode idempotency TTL 0 — the pre-migration Fiber path defaulted to 300 (ParseIdempotencyTTL("")); a 0 TTL makes the Redis idempotency key permanent`)
	}
}

// TestRevertTransaction_ForwardsOriginScopedIdempotencyHashSource proves the revert core
// still passes an origin-scoped idempotency hash source to createRevertTransaction. Without
// it the core falls back to hashing the reversal payload, which carries NO reference to the
// origin: two economically-identical origins in one ledger then share one slot and the
// second revert silently replays the FIRST origin's reverse (201, wrong
// parentTransactionId, second origin never reverted). Asserted over the live source AST
// because the hash source never surfaces on the transport response.
func TestRevertTransaction_ForwardsOriginScopedIdempotencyHashSource(t *testing.T) {
	src, err := os.ReadFile(stateHandlersFile)
	require.NoError(t, err, "read %s", stateHandlersFile)

	fn := findFuncDecl(t, string(src), "revertTransaction")

	call := findAssignedCall(t, fn, "createRevertTransaction")
	require.Greater(t, len(call.Args), revertTTLArgIndex+1,
		"createRevertTransaction call must forward an idempotency hash source after the TTL")

	hashSourceArg, isCall := call.Args[revertTTLArgIndex+1].(*ast.CallExpr)
	require.True(t, isCall, "the idempotency hash source argument must be a revertIdempotencyHashSource(...) call")

	ident, isIdent := hashSourceArg.Fun.(*ast.Ident)
	require.True(t, isIdent, "the idempotency hash source argument must be a plain function call")
	assert.Equal(t, "revertIdempotencyHashSource", ident.Name,
		"revert must key its idempotency slot through revertIdempotencyHashSource(originID) — dropping it re-opens the cross-origin replay")
}

// TestRevertIdempotencyHashSource_ScopedByOrigin locks the two halves of the revert
// idempotency key contract at the preimage level, where they are decidable without Redis:
// the SAME origin always yields the SAME preimage (so two concurrent reverts of one origin
// still collide on one SetNX — the only thing serializing the GetParentByTransactionID
// read-then-act race), and DIFFERENT origins always yield DIFFERENT preimages (so two
// economically-identical origins never share a slot).
func TestRevertIdempotencyHashSource_ScopedByOrigin(t *testing.T) {
	t.Parallel()

	originA := uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb50c")
	originB := uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb50d")

	assert.Equal(t, revertIdempotencyHashSource(originA), revertIdempotencyHashSource(originA),
		"the same origin must always produce the same preimage: two concurrent reverts of one origin must collide on one idempotency slot")

	assert.NotEqual(t, revertIdempotencyHashSource(originA), revertIdempotencyHashSource(originB),
		"distinct origins must produce distinct preimages: economically-identical origins must never share a revert idempotency slot")

	assert.Contains(t, revertIdempotencyHashSource(originA), originA.String(),
		"the preimage must carry the origin transaction id")

	// The NUL-joined action discriminator keeps a revert preimage disjoint from any create
	// preimage (v2IdempotencyHashSource uses the same separator, and no action label or JSON
	// body can contain a NUL byte).
	assert.True(t, strings.HasPrefix(revertIdempotencyHashSource(originA), "revert\x00"),
		"the preimage must carry the NUL-joined revert action discriminator")
}

// findAssignedCall returns the createRevertTransaction CallExpr from the function body
// (it is the RHS of a multi-value assignment: tranReverted, _, err := handler.createRevertTransaction(...)).
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
