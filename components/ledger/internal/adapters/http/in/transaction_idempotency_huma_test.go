// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"os"
	"reflect"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
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
	require.NotEmpty(t, call.Args, "createRevertTransaction call has no args")

	last := call.Args[len(call.Args)-1]
	if lit, isLit := last.(*ast.BasicLit); isLit {
		assert.NotEqual(t, "0", lit.Value,
			`revert must not hardcode idempotency TTL 0 — the pre-migration Fiber path defaulted to 300 (ParseIdempotencyTTL("")); a 0 TTL makes the Redis idempotency key permanent`)
	}
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
