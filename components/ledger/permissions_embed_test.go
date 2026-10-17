// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger_test

import (
	"context"
	"slices"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/midaz/v4/components/ledger"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
	t.Parallel()

	require.NotEmpty(t, ledger.MidazManifest, "embedded midaz manifest must not be empty")
}

// TestMidazManifest_SlugMatchesEmbeddedManifest self-verifies the embedded
// permissions.yaml in THIS package: declaration.New parses it, runs
// manifest.Validate, and enforces slug == manifest.service. The accept case proves
// the manifest declares "midaz"; the reject case proves New rejects a mismatched
// slug against this exact manifest. declaration.New performs no I/O and starts no
// goroutine, so the subtests are parallel-safe.
func TestMidazManifest_SlugMatchesEmbeddedManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		slug            string
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "wired midaz slug accepted",
			slug: midazSlug,
		},
		{
			name:            "mismatched slug rejected",
			slug:            "not-a-match",
			wantErr:         true,
			wantErrContains: `manifest.service "midaz"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pub, err := declaration.New(newConfig(tt.slug))

			if tt.wantErr {
				require.Error(t, err, "New must reject a slug that does not equal manifest.service")
				require.ErrorContains(t, err, tt.wantErrContains, "manifest must declare service midaz")
				require.Nil(t, pub)

				return
			}

			require.NoError(t, err, "embedded manifest must parse, validate, and declare service %q", tt.slug)
			require.NotNil(t, pub)
		})
	}
}

// adminGroup is the PLATFORM admin group, lerian/admin-group in the central
// Access Manager seed. It is written BARE here because the manifest writes every
// group bare and the reconciler composes the "lerian/" owner.
const adminGroup = "admin-group"

// seedAdminRoles are the midaz roles the seed binds to lerian/admin-group
// (caradhras deploy/pam-seed/init_data.json on origin/develop). The remaining
// tiers bind to their product group alone, there and here.
var seedAdminRoles = []string{"editor"}

// TestMidazManifest_GrantsSeedAdminRolesToThePlatformAdmin parses the embedded
// permissions.yaml exactly as the publisher does — yaml.v3 into
// declaration.DeclarationManifest — and fails unless every role the seed binds to
// admin-group is granted to it here as well. The platform admin reaches the ledger screens
// through the seed's admin-role rows today; those rows can retire only because
// this manifest names the group itself (decision of 2026-09-18), so a dropped
// grant would lock the console admin out the moment they go.
func TestMidazManifest_GrantsSeedAdminRolesToThePlatformAdmin(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest),
		"embedded manifest must parse as a declaration manifest")

	granted := make(map[string][]string, len(manifest.Roles))

	for _, role := range manifest.Roles {
		groups := make([]string, 0, len(role.GrantedTo))
		for _, grant := range role.GrantedTo {
			groups = append(groups, grant.Group)
		}

		granted[role.Name] = groups
	}

	for _, name := range seedAdminRoles {
		groups, declared := granted[name]
		require.True(t, declared, "manifest declares no %q role", name)
		require.Contains(t, groups, adminGroup,
			"role %q is granted to %v and not to %s: the platform admin loses the ledger screens "+
				"when the seed's midaz admin rows retire", name, groups, adminGroup)
	}
}

// TestMidazManifest_DeclaresTheTracerM2MEdge pins the import side of the M2M
// contract: the ledger reaches the tracer reservation seam with its application
// token, so the manifest must declare the tracer edge for the reconciler to grant
// tracer/reservations to the ledger's clients.
func TestMidazManifest_DeclaresTheTracerM2MEdge(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest),
		"embedded manifest must parse as a declaration manifest")

	require.NotNil(t, manifest.M2M, "the manifest declares an m2m contract")
	require.True(t, manifest.M2M.Exposed, "midaz keeps exposing its own M2M surface")
	require.Equal(t, []string{"tracer"}, manifest.M2M.Needs)
}

// ledgerCoveredRoutes are the accounting-route collections whose items belong to a
// ledger. They are written at the organization level, so their v2 paths name no
// ledger; covering them under ledgerId keeps a ledger-scoped partner off those
// organization-wide paths, while an organization-scoped partner keeps them.
var ledgerCoveredRoutes = []string{"operation-routes", "transaction-routes"}

// TestMidazManifest_LedgerDimensionCoversAccountingRoutes derives the scope
// catalog exactly as the publisher sends it to the access manager and fails
// unless the ledgerId dimension covers every accounting-route collection.
func TestMidazManifest_LedgerDimensionCoversAccountingRoutes(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest),
		"embedded manifest must parse as a declaration manifest")

	catalog, ok, err := declaration.ScopeCatalogFor(&manifest, midazSlug, nil)
	require.NoError(t, err)
	require.True(t, ok, "the manifest declares a scope catalog")

	var covers []string

	found := false

	for _, d := range catalog.Dimensions {
		if d.Name == "ledgerId" {
			covers, found = d.Covers, true
		}
	}

	require.True(t, found, "the catalog declares a ledgerId dimension")

	for _, collection := range ledgerCoveredRoutes {
		require.Contains(t, covers, collection,
			"ledgerId must cover %q: a ledger-scoped partner would otherwise be granted the "+
				"organization-wide %s paths", collection, collection)
	}
}

// explicitDimensions are the dimensions whose collection the ledger looks up by
// the item's own id alone: GET /v1/organizations/:organization_id/operation-routes/:operation_route_id
// names no ledger, so no ancestor dimension confines it. Explicit makes a partner
// whose scope names no id for the dimension be refused on the collection instead
// of being let through as unconfined.
var explicitDimensions = []string{"operationRouteId", "transactionRouteId"}

// TestMidazManifest_RouteDimensionsAreExplicit fails unless the parsed manifest
// still validates, and both the parsed declaration and the scope catalog the
// publisher sends carry explicit on exactly the route dimensions. An older
// lib-auth drops the unknown key silently, so the parsed value is what counts.
func TestMidazManifest_RouteDimensionsAreExplicit(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest),
		"embedded manifest must parse as a declaration manifest")
	require.NoError(t, manifest.Validate(), "embedded manifest must validate")
	require.NotNil(t, manifest.Scope, "the manifest declares a scope")

	declared := map[string]bool{}
	for _, d := range manifest.Scope.Dimensions {
		declared[d.Name] = d.Explicit
	}

	catalog, ok, err := declaration.ScopeCatalogFor(&manifest, midazSlug, nil)
	require.NoError(t, err)
	require.True(t, ok, "the manifest declares a scope catalog")

	published := map[string]bool{}
	for _, d := range catalog.Dimensions {
		published[d.Name] = d.Explicit
	}

	for name, explicit := range map[string]map[string]bool{"declaration": declared, "scope catalog": published} {
		for dim, isExplicit := range explicit {
			require.Equal(t, slices.Contains(explicitDimensions, dim), isExplicit,
				"%s: dimension %q explicit flag", name, dim)
		}

		for _, dim := range explicitDimensions {
			require.Contains(t, explicit, dim, "%s declares %q", name, dim)
		}
	}
}
