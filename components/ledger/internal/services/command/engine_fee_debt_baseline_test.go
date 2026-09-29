// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const feeDebtFreeGoldenPath = "testdata/fee_debt_free_translation.golden.json"

// TestFeeDebtFreeTranslationMatchesBaseline proves an execution that owes
// nothing translates byte-identically to the request before fee debts, on /v1
// and on /v2. The deferrable cases stay /v1 or outside direct, where the pair
// token is ignored.
func TestFeeDebtFreeTranslationMatchesBaseline(t *testing.T) {
	t.Parallel()

	golden := readFeeDebtFreeGolden(t)

	for name, input := range feeDebtFreeCases() {
		for _, v2 := range []bool{false, true} {
			if v2 && name == "direct_deferrable_v1" {
				continue
			}

			input.FeeDebtEligible = v2

			require.JSONEq(t, string(golden[name]), string(translateFeeDebtFreeCase(t, input)), "%s (v2=%v)", name, v2)
		}
	}
}

func translateFeeDebtFreeCase(t *testing.T, input EngineTranslationInput) []byte {
	t.Helper()

	transaction, projection, err := TranslateEngineTransaction(input)
	require.NoError(t, err)

	encoded, err := json.Marshal(map[string]any{"transaction": transaction, "projection": projection})
	require.NoError(t, err)

	return encoded
}

func readFeeDebtFreeGolden(t *testing.T) map[string]json.RawMessage {
	t.Helper()

	raw, err := os.ReadFile(feeDebtFreeGoldenPath)
	require.NoError(t, err)

	golden := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal(raw, &golden))
	require.Len(t, golden, len(feeDebtFreeCases()))

	return golden
}
