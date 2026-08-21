// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services_test

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/declaration"
	services "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	"github.com/stretchr/testify/require"
)

const feesSlug = "plugin-fees"

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
		Manifest:     services.FeesManifest,
		IdentityAddr: "http://identity.invalid",
		Auth:         stubTokenMinter{},
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
}

func TestFeesManifest_IsNonEmpty(t *testing.T) {
	require.NotEmpty(t, services.FeesManifest, "embedded plugin-fees manifest must not be empty")
}

func TestFeesManifest_ParsesAsValidFeesDeclaration(t *testing.T) {
	pub, err := declaration.New(newConfig(feesSlug))

	require.NoError(t, err, "embedded manifest must parse, validate, and declare service %q", feesSlug)
	require.NotNil(t, pub)
}

func TestFeesManifest_SlugIsPluginFees(t *testing.T) {
	_, err := declaration.New(newConfig("not-a-match"))

	require.Error(t, err, "New must reject a slug that does not equal manifest.service")
	require.ErrorContains(t, err, `manifest.service "plugin-fees"`, "manifest must declare service plugin-fees")
}
