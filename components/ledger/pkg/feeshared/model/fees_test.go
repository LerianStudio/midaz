// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeeCalculate_NonPayerLegsNeverReachTheWire(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(FeeCalculate{NonPayerLegs: []NonPayerLeg{{IsFrom: true, Index: 0}}})
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))

	for key := range fields {
		require.NotContains(t, strings.ToLower(key), "nonpayer", "unexpected wire field %q", key)
	}

	var decoded FeeCalculate
	require.NoError(t, json.Unmarshal([]byte(`{"nonPayerLegs":[{"IsFrom":true,"Index":0}],"NonPayerLegs":[{"IsFrom":true,"Index":0}]}`), &decoded))
	require.Empty(t, decoded.NonPayerLegs, "a request body must not set non-payer legs")
}
