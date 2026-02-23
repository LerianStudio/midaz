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
