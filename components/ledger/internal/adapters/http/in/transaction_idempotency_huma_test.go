// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

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
//  2. The revert idempotency preimage is a LIVE Redis key input, so its exact bytes are
//     part of the deployed contract: changing them re-slots every in-flight revert. The
//     golden test below locks those bytes. The surrounding call-site contract (an absent
//     X-TTL resolving to 300s rather than a permanent key, and the slot keying on the
//     origin rather than on the origin-agnostic reversal payload) is pinned behaviorally
//     by the Redis expectations in arrangeReplayedRevert
//     (transaction_revert_replayed_test.go) and end-to-end by the V2 revert integration
//     tests.

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
