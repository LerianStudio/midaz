// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeReserveJSONStrictWire(t *testing.T) {
	raw, err := os.ReadFile("testdata/reserve_request.json")
	require.NoError(t, err)
	limits := Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	for _, tc := range []struct {
		name  string
		body  string
		valid bool
	}{
		{"exact", string(raw), true},
		{"duplicate root", strings.Replace(string(raw), `"longLived": false`, `"longLived": false, "longLived": true`, 1), false},
		{"escaped duplicate", strings.Replace(string(raw), `"longLived": false`, `"longLived": false, "long\u004cived": true`, 1), false},
		{"unknown nested", strings.Replace(string(raw), `"blocked": false`, `"blocked": false, "organizationId": "injected"`, 1), false},
		{"wrong case", strings.Replace(string(raw), `"contextId"`, `"ContextId"`, 1), false},
		{"duplicate nested", strings.Replace(string(raw), `"blocked": false`, `"blocked": false, "blocked": true`, 1), false},
		{"number amount", strings.Replace(string(raw), `"9007199254740993.00000001"`, `9007199254740993.00000001`, 1), false},
		{"null boolean", strings.Replace(string(raw), `"longLived": false`, `"longLived": null`, 1), false},
		{"null accounts", strings.Replace(string(raw), `"accounts": [`, `"accounts": null, "unused": [`, 1), false},
		{"trailing object", string(raw) + " {}", false},
		{"invalid utf8", strings.Replace(string(raw), "deposit", string([]byte{0xff}), 1), false},
		{"escaped literal", strings.Replace(string(raw), "deposit", `\\ud800`, 1), true},
		{"high followed by plain text", strings.Replace(string(raw), "deposit", `\ud800text`, 1), false},
		{"high followed by high", strings.Replace(string(raw), "deposit", `\ud800\ud800`, 1), false},
		{"unpaired surrogate", strings.Replace(string(raw), "deposit", `\ud800`, 1), false},
		{"lone low surrogate", strings.Replace(string(raw), "deposit", `\udc00`, 1), false},
		{"valid surrogate pair", strings.Replace(string(raw), "deposit", `\ud83d\ude00`, 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DecodeReserveJSON(t.Context(), []byte(tc.body), len(raw)*2, limits)
			if !tc.valid {
				require.Error(t, err)
				require.Equal(t, ReserveRequest{}, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result.LongLived)
			require.False(t, *result.LongLived)
			require.Equal(t, Amount("9007199254740993.00000001"), result.Amount)
			require.NoError(t, result.Validate(t.Context(), "origin-a", limits))
		})
	}
	_, err = DecodeReserveJSON(t.Context(), raw, len(raw)-1, limits)
	require.Error(t, err)
	limits.MaxEntries = 1
	_, err = DecodeReserveJSON(t.Context(), raw, len(raw), limits)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = DecodeReserveJSON(ctx, raw, len(raw), limits)
	require.ErrorIs(t, err, context.Canceled)
}

func FuzzDecodeReserveJSON(f *testing.F) {
	raw, err := os.ReadFile("testdata/reserve_request.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(raw)
	f.Add([]byte(`{"context":{"entries":[[[[]]]]}}`))
	f.Add([]byte(`{"contextId":"\ud800"}`))
	limits := Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	f.Fuzz(func(t *testing.T, raw []byte) {
		result, err := DecodeReserveJSON(t.Context(), raw, 65536, limits)
		if err != nil {
			require.Equal(t, ReserveRequest{}, result)
		}
	})
}
