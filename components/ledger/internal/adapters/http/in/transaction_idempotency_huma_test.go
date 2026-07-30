// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/stretchr/testify/assert"
)

// Money-write idempotency parity gate (Wave 4 self-heal). One fact the pinning suite left
// uncovered, a silent money-path regression: the Huma CREATE envelopes must bind the
// idempotency headers under the SAME names the Fiber path (http.GetIdempotencyKeyAndTTL)
// reads — the lib-commons canonical "X-Idempotency" / "X-TTL". Huma binds header params by
// the literal `header:` tag (huma.go: value = ctx.Header(p.Name)); a wrong tag silently
// drops the caller's stable key, downgrading dedup to payload-hash and letting a keyed
// retry with a tweaked body mutate balances twice.
//
// Revert derives no preimage of its own — it passes no override, so the create core keys on
// the serialized reversal payload (the origin-agnostic slot documented as a KNOWN DEFECT on
// revertTransaction). Its call-site contract — an absent X-TTL
// resolving to 300s rather than a permanent key — is pinned behaviorally by the Redis
// expectations in arrangeReplayedRevert (transaction_revert_replayed_test.go).

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
