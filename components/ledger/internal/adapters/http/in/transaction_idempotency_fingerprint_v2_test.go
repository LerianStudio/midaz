// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const fingerprintBaseBody = `{"description":"fingerprint","asset":"USD","amount":"100","metadata":{"ref":"a","rate":0.1},` +
	`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"60"},{"alias":"@src2",` + v2ScopeJSON + `,"amount":"40"}],` +
	`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`

func mustV2IdempotencyFingerprint(t *testing.T, body string, pending bool, operationTypeOverride string) string {
	t.Helper()

	fingerprint, err := v2IdempotencyFingerprint([]byte(body), pending, operationTypeOverride)
	require.NoError(t, err)
	require.NotEmpty(t, fingerprint)

	return fingerprint
}

func TestV2IdempotencyFingerprint_SameRequestReserializedMatches(t *testing.T) {
	reserialized := `{
		"credits": [ { "amount": "100", "ledgerId": "` + v2ScopeLedgerID + `", "organizationId": "` + v2ScopeOrgID + `", "alias": "@dst" } ],
		"debits": [
			{ "organizationId": "` + v2ScopeOrgID + `", "amount": "60", "ledgerId": "` + v2ScopeLedgerID + `", "alias": "@src" },
			{ "amount": "40", "alias": "@src2", "ledgerId": "` + v2ScopeLedgerID + `", "organizationId": "` + v2ScopeOrgID + `" }
		],
		"metadata": { "rate": 0.1, "ref": "a" },
		"amount": "100",
		"asset": "USD",
		"description": "fingerprint"
	}`

	assert.Equal(t,
		mustV2IdempotencyFingerprint(t, fingerprintBaseBody, false, ""),
		mustV2IdempotencyFingerprint(t, reserialized, false, ""),
		"whitespace and property order must not change the fingerprint")
}

func TestV2IdempotencyFingerprint_ActionIsPartOfTheIdentity(t *testing.T) {
	actions := []struct {
		name                  string
		pending               bool
		operationTypeOverride string
	}{
		{name: "direct"},
		{name: "hold", pending: true},
		{name: "block", operationTypeOverride: constant.BLOCK},
		{name: "unblock", operationTypeOverride: constant.UNBLOCK},
	}

	seen := make(map[string]string, len(actions))

	for _, action := range actions {
		fingerprint := mustV2IdempotencyFingerprint(t, fingerprintBaseBody, action.pending, action.operationTypeOverride)

		if previous, ok := seen[fingerprint]; ok {
			t.Fatalf("actions %q and %q share a fingerprint for the same body", previous, action.name)
		}

		seen[fingerprint] = action.name
	}
}

func TestV2IdempotencyFingerprint_DifferentContentDiffers(t *testing.T) {
	base := mustV2IdempotencyFingerprint(t, fingerprintBaseBody, false, "")

	cases := []struct {
		name string
		body string
	}{
		{
			name: "description changed",
			body: `{"description":"other","asset":"USD","amount":"100","metadata":{"ref":"a","rate":0.1},` +
				`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"60"},{"alias":"@src2",` + v2ScopeJSON + `,"amount":"40"}],` +
				`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
		},
		{
			name: "amount changed",
			body: `{"description":"fingerprint","asset":"USD","amount":"101","metadata":{"ref":"a","rate":0.1},` +
				`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"61"},{"alias":"@src2",` + v2ScopeJSON + `,"amount":"40"}],` +
				`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"101"}]}`,
		},
		{
			name: "metadata number differs beyond float64 precision",
			body: `{"description":"fingerprint","asset":"USD","amount":"100","metadata":{"ref":"a","rate":0.1000000000000000055},` +
				`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"60"},{"alias":"@src2",` + v2ScopeJSON + `,"amount":"40"}],` +
				`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
		},
		{
			name: "debit legs reordered",
			body: `{"description":"fingerprint","asset":"USD","amount":"100","metadata":{"ref":"a","rate":0.1},` +
				`"debits":[{"alias":"@src2",` + v2ScopeJSON + `,"amount":"40"},{"alias":"@src",` + v2ScopeJSON + `,"amount":"60"}],` +
				`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEqual(t, base, mustV2IdempotencyFingerprint(t, tc.body, false, ""))
		})
	}
}

// TestV2IdempotencyFingerprint_StableAcrossReleases pins the fingerprint of a fixed
// request. Stored slots outlive a deploy (up to the slot TTL), so a release that
// changed the fingerprint of an unchanged request would answer an honest retry
// across the rollout with 0084 instead of the replay. The expected values were
// computed independently of this package: sha256 over the domain label, the action
// discriminator, the separator and the body re-encoded with sorted keys.
func TestV2IdempotencyFingerprint_StableAcrossReleases(t *testing.T) {
	body := `{"description":"golden","asset":"USD","amount":"100","debits":[{"alias":"@src","amount":"100"}],"credits":[{"alias":"@dst","amount":"100"}]}`

	assert.Equal(t, "c10ae8a79edae205b7ab419002800f8341531efbf15bc53e53940b0bf8f3bf0f", mustV2IdempotencyFingerprint(t, body, false, ""), "direct")
	assert.Equal(t, "614e5c4bac4b5b71c60fc6b376d3bb72c8fa52a2edfd00c2b94a3e0e4effb13d", mustV2IdempotencyFingerprint(t, body, true, ""), "hold")
}

func TestV2IdempotencyFingerprint_RejectsMalformedBody(t *testing.T) {
	for _, body := range []string{`{"description":`, `{"a":1} {"b":2}`, ``} {
		_, err := v2IdempotencyFingerprint([]byte(body), false, "")
		assert.Error(t, err, "body %q must not produce a fingerprint", body)
	}
}
