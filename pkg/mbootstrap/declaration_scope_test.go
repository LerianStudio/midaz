// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mbootstrap

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/LerianStudio/lib-auth/v5/auth/obs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTokenMinter struct{}

func (stubTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func TestWireManifestScope_NoRoutesClientWiresNothing(t *testing.T) {
	var typedNil *middleware.AuthClient

	for name, minter := range map[string]declaration.TokenMinter{
		"nil":                nil,
		"typed nil client":   typedNil,
		"not an auth client": stubTokenMinter{},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, WireManifestScope(minter, []byte("service: [unterminated")),
				"without a routes client there is no scope to wire, so the manifest is never read")
		})
	}
}

func TestWireManifestScope_InvalidManifestFailsTheBoot(t *testing.T) {
	auth := &middleware.AuthClient{Address: "http://auth.invalid", Enabled: true, Logger: obs.Nop()}

	err := WireManifestScope(auth, []byte("service: [unterminated"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wire manifest scope")
}
