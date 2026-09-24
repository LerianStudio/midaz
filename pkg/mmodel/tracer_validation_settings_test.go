// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestTracerValidationModeSettings(t *testing.T) {
	for _, raw := range []string{`{}`, `{"tracer":{}}`, `{"tracer":{"validationMode":null}}`} {
		var values map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &values))
		require.NoError(t, ValidateSettings(values))
		settings := ParseLedgerSettings(values)
		require.Equal(t, string(tracercontract.ValidationLimits), settings.Tracer.ValidationMode)
		require.Equal(t, TracerModeOff, settings.Tracer.Mode)
	}
	for _, value := range []string{"limits", "rules-and-limits", "rules", "", "LIMITS", " rules-and-limits"} {
		t.Run(value, func(t *testing.T) {
			raw := map[string]any{"tracer": map[string]any{"validationMode": value}}
			err := ValidateSettings(raw)
			if value != "limits" && value != "rules-and-limits" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			settings := ParseLedgerSettings(raw)
			require.Equal(t, value, settings.Tracer.ValidationMode)
			require.Equal(t, settings, ParseLedgerSettings(LedgerSettingsToMap(settings)))
			encoded, err := json.Marshal(settings)
			require.NoError(t, err)
			var decoded LedgerSettings
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, settings, decoded)
			var input LedgerSettingsInput
			require.NoError(t, json.Unmarshal([]byte(`{"tracer":{"validationMode":"`+value+`"}}`), &input))
			require.Equal(t, raw, input.ToSparseMap())
		})
	}
	require.Error(t, ValidateSettings(map[string]any{"tracer": map[string]any{"validationMode": true}}))
	require.Error(t, ValidateSettings(map[string]any{"validationMode": "limits"}))
}

func TestTracerValidationModeMergeDoesNotAlterParticipation(t *testing.T) {
	initial := map[string]any{"tracer": map[string]any{"mode": "enforce", "failPosture": "closed", "timeoutMs": 750}, "overrides": map[string]any{"allowTracerSkip": true}}
	merged := DeepMergeSettings(initial, map[string]any{"tracer": map[string]any{"validationMode": "rules-and-limits"}})
	settings := ParseLedgerSettings(merged)
	require.Equal(t, "rules-and-limits", settings.Tracer.ValidationMode)
	require.Equal(t, "enforce", settings.Tracer.Mode)
	require.Equal(t, "closed", settings.Tracer.FailPosture)
	require.Equal(t, 750, settings.Tracer.TimeoutMs)
	require.True(t, settings.Overrides.AllowTracerSkip)
	require.Equal(t, "limits", ParseLedgerSettings(initial).Tracer.ValidationMode)
}
