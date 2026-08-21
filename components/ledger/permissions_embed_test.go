// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger_test

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/declaration"
	"github.com/LerianStudio/midaz/v4/components/ledger"
	"github.com/stretchr/testify/require"
)

const midazSlug = "midaz"

// stubTokenMinter satisfies declaration.TokenMinter without any network I/O.
// declaration.New never mints a token (that happens at Publish time), but
// declaration.Config requires a non-nil Auth to clear config validation.
type stubTokenMinter struct{}

func (stubTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// newConfig builds a declaration.Config that clears validateConfig so New
// proceeds to parse the embedded manifest, run manifest.Validate, and enforce
// slug == manifest.service. Only Slug varies across cases; the remaining fields
// are inert dummies because New performs no I/O.
func newConfig(slug string) declaration.Config {
	return declaration.Config{
		Slug:         slug,
		Manifest:     ledger.MidazManifest,
		IdentityAddr: "http://identity.invalid",
		Auth:         stubTokenMinter{},
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
}

func TestMidazManifest_IsNonEmpty(t *testing.T) {
	require.NotEmpty(t, ledger.MidazManifest, "embedded midaz manifest must not be empty")
}

func TestMidazManifest_ParsesAsValidMidazDeclaration(t *testing.T) {
	pub, err := declaration.New(newConfig(midazSlug))

	require.NoError(t, err, "embedded manifest must parse, validate, and declare service %q", midazSlug)
	require.NotNil(t, pub)
}

func TestMidazManifest_SlugIsMidaz(t *testing.T) {
	_, err := declaration.New(newConfig("not-a-match"))

	require.Error(t, err, "New must reject a slug that does not equal manifest.service")
	require.ErrorContains(t, err, `manifest.service "midaz"`, "manifest must declare service midaz")
}
