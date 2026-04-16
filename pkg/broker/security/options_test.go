// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package security

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildFranzGoOptions_RejectsMissingTLSCAFile(t *testing.T) {
	t.Parallel()

	_, err := BuildFranzGoOptions(Config{
		TLSEnabled: true,
		TLSCAFile:  filepath.Join(t.TempDir(), "missing-ca.pem"),
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "stat redpanda tls ca file")
}

func TestBuildFranzGoOptions_RejectsDirectoryAsTLSCAFile(t *testing.T) {
	t.Parallel()

	_, err := BuildFranzGoOptions(Config{
		TLSEnabled: true,
		TLSCAFile:  t.TempDir(),
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "must be a file path")
}

// TestSASLPLAIN_WithoutTLSInProductionRejected asserts the D6 gate: PLAIN
// without TLS in production-like envs must be rejected, because PLAIN sends
// credentials in cleartext and cannot be paired with any other cryptographic
// channel protection.
func TestSASLPLAIN_WithoutTLSInProductionRejected(t *testing.T) {
	t.Parallel()

	_, err := BuildFranzGoOptions(Config{
		TLSEnabled:    false,
		SASLEnabled:   true,
		SASLMechanism: "PLAIN",
		SASLUsername:  "midaz",
		SASLPassword:  "s3cret",
		Environment:   "production",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrSASLPlainRequiresTLSInProduction)
}

func TestSASLPLAIN_WithoutTLSInDevelopmentAllowed(t *testing.T) {
	t.Parallel()

	// Non-production allows PLAIN without TLS so dev/test loops keep working.
	opts, err := BuildFranzGoOptions(Config{
		TLSEnabled:    false,
		SASLEnabled:   true,
		SASLMechanism: "PLAIN",
		SASLUsername:  "midaz",
		SASLPassword:  "s3cret",
		Environment:   "development",
	})

	require.NoError(t, err)
	require.NotEmpty(t, opts)
}

func TestSASLPLAIN_WithTLSInProductionAllowed(t *testing.T) {
	t.Parallel()

	opts, err := BuildFranzGoOptions(Config{
		TLSEnabled:    true,
		SASLEnabled:   true,
		SASLMechanism: "PLAIN",
		SASLUsername:  "midaz",
		SASLPassword:  "s3cret",
		Environment:   "production",
	})

	require.NoError(t, err)
	require.NotEmpty(t, opts)
}
