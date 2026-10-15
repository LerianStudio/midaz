// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEntryKeyForms_SplitBackToAliasKey locks the round trip between the two key forms a
// leg goes through. MutateConcatAliases stamps every leg with FromTo.ConcatAlias, the
// "index#alias#balanceKey" entry key that keys Responses.From and Responses.To. The
// balance-operation builder then splits each entry key back with SplitAliasWithKey and
// looks the result up in a balance index keyed "alias#balanceKey", the form AliasKey
// spells. A change to the separator, the segment order or the empty-key default on
// either side would leave legs without a matching balance, so it has to fail here.
func TestEntryKeyForms_SplitBackToAliasKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		alias    string
		key      string
		index    int
		wantKey  string
		wantBack string
	}{
		{name: "bare alias with an empty key defaults the key", alias: "@acc", key: "", index: 0, wantKey: "0#@acc#default", wantBack: "@acc#default"},
		{name: "explicit key", alias: "@acc", key: "savings", index: 0, wantKey: "0#@acc#savings", wantBack: "@acc#savings"},
		{name: "explicit default key", alias: "@acc", key: "default", index: 0, wantKey: "0#@acc#default", wantBack: "@acc#default"},
		{name: "non-zero index", alias: "@acc", key: "wallet", index: 7, wantKey: "7#@acc#wallet", wantBack: "@acc#wallet"},
		{name: "multi-digit index", alias: "@acc", key: "", index: 12, wantKey: "12#@acc#default", wantBack: "@acc#default"},
		{name: "external account alias", alias: "@external/USD", key: "", index: 3, wantKey: "3#@external/USD#default", wantBack: "@external/USD#default"},
		{name: "empty alias", alias: "", key: "", index: 1, wantKey: "1##default", wantBack: "#default"},
		{name: "unicode alias", alias: "@conta-ção", key: "reserva", index: 2, wantKey: "2#@conta-ção#reserva", wantBack: "@conta-ção#reserva"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entryKey := FromTo{AccountAlias: tc.alias, BalanceKey: tc.key}.ConcatAlias(tc.index)

			assert.Equal(t, tc.wantKey, entryKey)
			assert.Equal(t, tc.wantBack, SplitAliasWithKey(entryKey))
			assert.Equal(t, AliasKey(tc.alias, tc.key), SplitAliasWithKey(entryKey),
				"the entry key must split back to the balance index key")
		})
	}
}

// FuzzEntryKeyForms holds the same round trip over arbitrary aliases, keys and indexes.
func FuzzEntryKeyForms(f *testing.F) {
	f.Add("@acc", "", uint8(0))
	f.Add("@acc", "savings", uint8(7))
	f.Add("", "", uint8(0))
	f.Add("@acc#savings", "savings", uint8(1))
	f.Add("@acc", "key#part", uint8(2))
	f.Add("12", "34", uint8(255))
	f.Add("@conta-ção", "日本語", uint8(9))

	// maxStringLen bounds fuzzer-generated strings to prevent resource exhaustion.
	const maxStringLen = 48

	f.Fuzz(func(t *testing.T, alias, key string, index uint8) {
		if len(alias) > maxStringLen {
			alias = alias[:maxStringLen]
		}

		if len(key) > maxStringLen {
			key = key[:maxStringLen]
		}

		i := int(index)

		entryKey := FromTo{AccountAlias: alias, BalanceKey: key}.ConcatAlias(i)
		splitBack := SplitAliasWithKey(entryKey)
		want := AliasKey(alias, key)

		if splitBack != want {
			t.Fatalf("SplitAliasWithKey(FromTo{%q, %q}.ConcatAlias(%d)) = %q, want AliasKey = %q", alias, key, i, splitBack, want)
		}
	})
}
