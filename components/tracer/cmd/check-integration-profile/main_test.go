// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileCheckDetectsPrecisionAndEnvelopeDrift(t *testing.T) {
	dir := t.TempDir()
	ledger, tracer := filepath.Join(dir, "ledger.env"), filepath.Join(dir, "tracer.env")
	require.NoError(t, os.WriteFile(ledger, []byte("# defaults\n"), 0o600))
	for _, test := range []struct {
		input string
		valid bool
	}{
		{"# defaults\n", true},
		{"CONTEXT_MAX_FRACTION_DIGITS=128\n", true},
		{"CONTEXT_MAX_FRACTION_DIGITS=8\n", false},
		{"CONTEXT_MAX_FRACTION_DIGITS=0\n", false},
		{"CONTEXT_RESERVE_MAX_BODY_BYTES=10\n", false},
		{"CONTEXT_MAX_ACCOUNTS=0\n", false},
		{"CONTEXT_MAX_FRACTION_DIGITS=\n", false},
	} {
		require.NoError(t, os.WriteFile(tracer, []byte(test.input), 0o600))
		err := checkProfiles(ledger, tracer)
		if test.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	require.NoError(t, os.WriteFile(tracer, []byte("SECRET='private-test-material"), 0o600))
	err := checkProfiles(ledger, tracer)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-test-material")
}
