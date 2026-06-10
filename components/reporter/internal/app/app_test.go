// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRunMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    RunMode
		wantErr bool
	}{
		{name: "api", raw: "api", want: RunModeAPI},
		{name: "worker", raw: "worker", want: RunModeWorker},
		{name: "all", raw: "all", want: RunModeAll},
		{name: "blank defaults to all", raw: "", want: RunModeAll},
		{name: "whitespace defaults to all", raw: "   ", want: RunModeAll},
		{name: "padded api", raw: "  api  ", want: RunModeAPI},
		{name: "padded worker", raw: "\tworker\n", want: RunModeWorker},
		{name: "mixed case API", raw: "API", want: RunModeAPI},
		{name: "mixed case Worker", raw: "Worker", want: RunModeWorker},
		{name: "mixed case ALL", raw: "ALL", want: RunModeAll},
		{name: "typo wokrer is rejected", raw: "wokrer", wantErr: true},
		{name: "garbage is rejected", raw: "nonsense", wantErr: true},
		{name: "partial match api-ish is rejected", raw: "apis", wantErr: true},
		{name: "comma list is rejected", raw: "api,worker", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseRunMode(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, string(got), "rejected input must not yield a usable mode")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRunModeSurfaceGating locks the truth table that decides which surfaces a
// mode activates. A one-character flip (e.g. dropping RunModeAll from
// runsWorker) would silently ship the wrong surfaces; integration only covers
// the two single-surface modes, never "all".
func TestRunModeSurfaceGating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       RunMode
		wantAPI    bool
		wantWorker bool
	}{
		{name: "api activates only the API surface", mode: RunModeAPI, wantAPI: true, wantWorker: false},
		{name: "worker activates only the worker surface", mode: RunModeWorker, wantAPI: false, wantWorker: true},
		{name: "all activates both surfaces", mode: RunModeAll, wantAPI: true, wantWorker: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantAPI, tt.mode.runsAPI(), "runsAPI mismatch")
			assert.Equal(t, tt.wantWorker, tt.mode.runsWorker(), "runsWorker mismatch")
		})
	}
}

// TestRunModeGatingNoCrossActivation is an explicit guard for the invariant
// Fred called out: api must never start the worker, and worker must never start
// the API. Kept separate from the truth table so a regression names the exact
// cross-activation that broke.
func TestRunModeGatingNoCrossActivation(t *testing.T) {
	t.Parallel()

	assert.False(t, RunModeAPI.runsWorker(), "api mode must not activate the worker surface")
	assert.False(t, RunModeWorker.runsAPI(), "worker mode must not activate the API surface")
}
