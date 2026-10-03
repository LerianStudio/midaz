// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer_test

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/LerianStudio/midaz/v4/components/tracer"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const tracerSlug = "tracer"

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
		Manifest:     tracer.TracerManifest,
		IdentityAddr: "http://identity.invalid",
		Auth:         stubTokenMinter{},
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
}

func TestTracerManifest_IsNonEmpty(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, tracer.TracerManifest, "embedded tracer manifest must not be empty")
}

// TestTracerManifest_SlugMatchesEmbeddedManifest self-verifies the embedded
// permissions.yaml in THIS package: declaration.New parses it, runs
// manifest.Validate, and enforces slug == manifest.service. The accept case proves
// the manifest declares "tracer"; the reject case proves New rejects a mismatched
// slug against this exact manifest. declaration.New performs no I/O and starts no
// goroutine, so the subtests are parallel-safe.
func TestTracerManifest_SlugMatchesEmbeddedManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		slug            string
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "wired tracer slug accepted",
			slug: tracerSlug,
		},
		{
			name:            "mismatched slug rejected",
			slug:            "not-a-match",
			wantErr:         true,
			wantErrContains: `manifest.service "tracer"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pub, err := declaration.New(newConfig(tt.slug))

			if tt.wantErr {
				require.Error(t, err, "New must reject a slug that does not equal manifest.service")
				require.ErrorContains(t, err, tt.wantErrContains, "manifest must declare service tracer")
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

// seedAdminRoles are the tracer roles the seed binds to lerian/admin-group
// (caradhras deploy/pam-seed/init_data.json on origin/develop). The remaining
// tiers bind to their product group alone, there and here.
var seedAdminRoles = []string{"editor", "validator", "audit-viewer"}

// TestTracerManifest_GrantsSeedAdminRolesToThePlatformAdmin parses the embedded
// permissions.yaml exactly as the publisher does — yaml.v3 into
// declaration.DeclarationManifest — and fails unless every role the seed binds to
// admin-group is granted to it here as well. The platform admin reaches the Tracer screens
// through the seed's admin-role rows today; those rows can retire only because
// this manifest names the group itself (decision of 2026-09-18), so a dropped
// grant would lock the console admin out the moment they go.
func TestTracerManifest_GrantsSeedAdminRolesToThePlatformAdmin(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(tracer.TracerManifest, &manifest),
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
			"role %q is granted to %v and not to %s: the platform admin loses the Tracer screens "+
				"when the seed's tracer admin rows retire", name, groups, adminGroup)
	}
}

// TestTracerManifest_DeclaresTheSeedReservationGrant locks the grant the
// reservation seam authorizes against: reservations/post to the tiers the
// central Access Manager seed binds it to. Dropping it here would let the
// reconciler's prune delete the seeded grant and lock the ledger out of the
// seam.
func TestTracerManifest_DeclaresTheSeedReservationGrant(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(tracer.TracerManifest, &manifest))

	var grants []declaration.DeclarationPermission

	for _, permission := range manifest.Permissions {
		if permission.Resource == "reservations" {
			grants = append(grants, permission)
		}
	}

	require.Len(t, grants, 1, "the manifest declares exactly one reservations grant")
	require.Equal(t, "post", grants[0].Action)
	require.Equal(t, "allow", grants[0].Effect)
	require.ElementsMatch(t, []string{"editor", "validator", "audit-viewer"}, grants[0].Roles)
}

// TestTracerManifest_OptsInToPartners pins the opt-in: without it the access
// manager grants no partner credential access to the tracer.
func TestTracerManifest_OptsInToPartners(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(tracer.TracerManifest, &manifest))
	require.NoError(t, manifest.Validate())
	require.True(t, manifest.Partners, "the tracer manifest must opt in to partners")
}

// wantTracerDimensions pins the catalog order: the rank of a dimension is one plus
// its index, and accountId must be the deepest so every scoped partner's narrowest
// dimension ranks at or above none of the levels it can be granted.
var wantTracerDimensions = []string{"ruleId", "limitId", "validationId", "auditEventId", "portfolioId", "segmentId", "merchantId", "accountId"}

// wantTracerLevels pins the level of every tracer permission: every write tenant
// but submitting a validation, which is accountId; every read tenant.
var wantTracerLevels = map[string]string{
	"rules post": "tenant", "rules get": "tenant", "rules patch": "tenant", "rules delete": "tenant",
	"limits post": "tenant", "limits get": "tenant", "limits patch": "tenant", "limits delete": "tenant",
	"validations get": "tenant", "validations post": "accountId",
	"audit-events get": "tenant", "streaming-manifest get": "tenant",
	"reservations post": "tenant", "dashboard get": "tenant",
}

func tracerManifest(t *testing.T) declaration.DeclarationManifest {
	t.Helper()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(tracer.TracerManifest, &manifest))
	require.NoError(t, manifest.Validate())
	require.NotNil(t, manifest.Scope)

	return manifest
}

// TestTracerManifest_DimensionsAndLevelsArePinned pins the catalog order and the level
// of every permission line.
func TestTracerManifest_DimensionsAndLevelsArePinned(t *testing.T) {
	t.Parallel()

	manifest := tracerManifest(t)

	names := make([]string, 0, len(manifest.Scope.Dimensions))
	for _, dim := range manifest.Scope.Dimensions {
		names = append(names, dim.Name)
	}

	require.Equal(t, wantTracerDimensions, names)

	got := make(map[string]string, len(manifest.Permissions))
	for _, permission := range manifest.Permissions {
		got[permission.Resource+" "+permission.Action] = permission.Level
	}

	require.Equal(t, wantTracerLevels, got)
}

// TestTracerManifest_PartnerWritesFollowTheRank checks the levels against the rule
// the access manager applies to a partner: a tenant-level write is never granted,
// and any other write is granted only when the partner's narrowest dimension ranks
// at or below the write's level, the rank of a dimension being
// one plus its catalog index and tenant ranking zero. A partner scoped on any one of
// portfolioId, segmentId, merchantId or accountId, or on none, may be granted
// submitting a validation and no rule or limit write.
func TestTracerManifest_PartnerWritesFollowTheRank(t *testing.T) {
	t.Parallel()

	manifest := tracerManifest(t)

	rank := map[string]int{"tenant": 0}
	for i, dim := range manifest.Scope.Dimensions {
		rank[dim.Name] = i + 1
	}

	grantable := func(level string, narrowest string) bool {
		if level == "tenant" {
			return false
		}

		return rank[narrowest] <= rank[level]
	}

	for _, permission := range manifest.Permissions {
		if permission.Action == "get" || permission.Action == "head" {
			continue
		}

		for _, narrowest := range []string{"tenant", "portfolioId", "segmentId", "merchantId", "accountId"} {
			want := permission.Resource == "validations"

			require.Equalf(t, want, grantable(permission.Level, narrowest),
				"%s %s for a partner scoped on %s", permission.Resource, permission.Action, narrowest)
		}
	}
}
