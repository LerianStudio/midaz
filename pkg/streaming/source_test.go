// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestRequireRosterSource locks the fail-closed contract: only the roster name
// passes. Grammar-legality is NOT enough — the tenant-manager grants a producer
// WRITE+DESCRIBE on literal topic names derived from the roster name alone, so any
// other source names a topic that neither exists nor is granted. Under midaz's
// IMPORTANT posture that loses every event behind a single Warn, which is why this
// has to be a startup refusal rather than a runtime error.
func TestRequireRosterSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		configured string
		roster     string
		wantErr    bool
	}{
		{
			name:       "roster name passes",
			configured: "ledger",
			roster:     "ledger",
		},
		{
			// The case the review caught: legal grammar, unprovisionable name.
			name:       "grammar-legal non-roster name is refused",
			configured: "midaz-ledger",
			roster:     "ledger",
			wantErr:    true,
		},
		{
			// A prefixed ACL grant would have authorized this; the literal grant
			// deliberately does not, so it must be refused here too.
			name:       "roster name with a suffix is refused",
			configured: "ledgerx",
			roster:     "ledger",
			wantErr:    true,
		},
		{
			name:       "stale pre-v3 dotted source is refused",
			configured: "lerian.midaz.ledger",
			roster:     "ledger",
			wantErr:    true,
		},
		{
			name:       "stale pre-v3 URI source is refused",
			configured: "//lerian.midaz/ledger",
			roster:     "ledger",
			wantErr:    true,
		},
		{
			name:       "empty source is refused",
			configured: "",
			roster:     "ledger",
			wantErr:    true,
		},
		{
			name:       "tracer roster name passes",
			configured: "tracer",
			roster:     "tracer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := pkgStreaming.RequireRosterSource(tc.configured, tc.roster)
			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			require.ErrorIs(t, err, pkgStreaming.ErrSourceNotRoster)
			require.Contains(t, err.Error(), tc.roster,
				"the error must name the value the operator has to set")
		})
	}
}
