// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger_test

import (
	"context"
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

// wantDimensionCovers is the confinement each scope dimension extends over the
// collections whose items belong to its instances. A partner credential scoped on
// one of these dimensions is accepted on a covered collection only when the
// request names the dimension, so dropping a name here silently widens what such
// a credential can reach.
var wantDimensionCovers = map[string][]string{
	"accountId":   {"balances", "transactions", "operations", "fee-debts", "dashboard", "account-block-exceptions"},
	"ledgerId":    {"holders", "instruments", "protection"},
	"portfolioId": {"accounts"},
	"segmentId":   {"accounts"},
}

// TestMidazManifest_ScopeDimensionsDeclareCovers parses the embedded manifest as
// the publisher does and pins the covers of every scope dimension: the four above
// carry exactly their list, and no other dimension covers anything.
func TestMidazManifest_ScopeDimensionsDeclareCovers(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest),
		"embedded manifest must parse as a declaration manifest")
	require.NotNil(t, manifest.Scope, "the manifest declares a scope catalog")

	got := make(map[string][]string, len(manifest.Scope.Dimensions))

	for _, dim := range manifest.Scope.Dimensions {
		if len(dim.Covers) > 0 {
			got[dim.Name] = dim.Covers
		}
	}

	require.Equal(t, wantDimensionCovers, got, "scope dimension covers drifted from the declared confinement")
}

// wantScopeRoutes is every place a route reads a scope dimension from the request
// beyond its path, keyed by "METHOD path" and spelled "name<-from:field", with a
// trailing "?" when the request may leave it out. Each one widens or narrows what
// a partner credential is asked about, so an entry dropped here is a value that
// stops being checked.
var wantScopeRoutes = map[string][]string{
	"POST /v2/transactions/direct":  v2CreateBodyCarriers,
	"POST /v2/transactions/hold":    v2CreateBodyCarriers,
	"POST /v2/transactions/block":   v2CreateBodyCarriers,
	"POST /v2/transactions/unblock": v2CreateBodyCarriers,
	"POST /v2/transactions/batch": {
		"organizationId<-body:transactions[].debits[].organizationId",
		"ledgerId<-body:transactions[].debits[].ledgerId",
		"accountId<-body:transactions[].debits[].alias=>accountByAlias",
		"organizationId<-body:transactions[].credits[].organizationId",
		"ledgerId<-body:transactions[].credits[].ledgerId",
		"accountId<-body:transactions[].credits[].alias=>accountByAlias",
	},
	"POST /v2/organizations/:organization_id/holders/:holder_id/instruments": {
		"ledgerId<-body:ledgerId",
		"accountId<-body:accountId",
	},
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/accounts": {
		"portfolioId<-body:portfolioId?",
		"segmentId<-body:segmentId?",
		"accountId<-body:parentAccountId?",
	},
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/accounts": {
		"portfolioId<-body:portfolioId?",
		"segmentId<-body:segmentId?",
		"holderId<-body:holderId?",
		"accountId<-body:parentAccountId?",
	},
	"PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id": accountUpdateBodyCarriers,
	"PATCH /v2/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id": accountUpdateBodyCarriers,
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/holders/:holder_id/accounts": {
		"portfolioId<-body:portfolioId?",
		"segmentId<-body:segmentId?",
		"accountId<-body:parentAccountId?",
	},
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/billing-packages": {
		"segmentId<-body:accountTarget.segmentId?",
		"portfolioId<-body:accountTarget.portfolioId?",
		"accountId<-body:accountTarget.aliases[]?=>accountByAlias",
		"accountId<-body:maintenanceCreditAccount?=>accountByAlias",
	},
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/packages": {
		"segmentId<-body:segmentId?",
	},
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts": accountListQueryCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/accounts": accountListQueryCarriers,
	"GET /v2/organizations/:organization_id/instruments": {
		"holderId<-query:holder_id?",
		"accountId<-query:account_id?",
		"ledgerId<-query:ledger_id?",
	},
	"GET /v2/organizations/:organization_id/holders/:holder_id":    holderLedgersCarriers,
	"PATCH /v2/organizations/:organization_id/holders/:holder_id":  holderLedgersCarriers,
	"DELETE /v2/organizations/:organization_id/holders/:holder_id": holderLedgersCarriers,
	"GET /v2/organizations/:organization_id/holders/:holder_id/accounts": {
		"ledgerId<-query:ledger_id?",
	},
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/packages": {
		"segmentId<-query:segmentId?",
	},
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json":                                      v1CreateAliasCarriers,
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/annotation":                                v1CreateAliasCarriers,
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/block":                                     v1CreateAliasCarriers,
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/unblock":                                   v1CreateAliasCarriers,
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/inflow":                                    {"accountId<-body:send.distribute.to[].accountAlias=>accountByAlias"},
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/outflow":                                   {"accountId<-body:send.source.from[].accountAlias=>accountByAlias"},
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id":                            transactionAccountsCarriers,
	"PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id":                          transactionAccountsCarriers,
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit":                    transactionAccountsCarriers,
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel":                    transactionAccountsCarriers,
	"POST /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert":                    transactionAccountsCarriers,
	"PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id": transactionAccountsCarriers,
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id":                                    balanceAccountCarriers,
	"PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id":                                  balanceAccountCarriers,
	"DELETE /v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id":                                 balanceAccountCarriers,
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id/history":                            balanceAccountCarriers,
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias":                                   aliasAccountCarriers,
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code":                                 externalAccountCarriers,
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances":                          aliasAccountCarriers,
	"GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances":                        externalAccountCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id":                            transactionAccountsCarriers,
	"PATCH /v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id":                          transactionAccountsCarriers,
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit":                    transactionAccountsCarriers,
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel":                    transactionAccountsCarriers,
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert":                    transactionAccountsCarriers,
	"PATCH /v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id": transactionAccountsCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id":                                    balanceAccountCarriers,
	"PATCH /v2/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id":                                  balanceAccountCarriers,
	"DELETE /v2/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id":                                 balanceAccountCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id/history":                            balanceAccountCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias":                                   aliasAccountCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code":                                 externalAccountCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/accounts/alias/:alias/balances":                          aliasAccountCarriers,
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/accounts/external/:code/balances":                        externalAccountCarriers,
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/accounts/block-exceptions":                              {"accountId<-body:exceptions[].accountAlias=>accountByAlias"},
	"POST /v2/organizations/:organization_id/ledgers/:ledger_id/fee-debts/collect":                                      {"accountId<-body:accountAlias=>accountByAlias"},
	"GET /v2/organizations/:organization_id/ledgers/:ledger_id/fee-debts":                                               {"accountId<-query:account_alias?=>accountByAlias"},
}

// holderLedgersCarriers is the carrier of every holder detail route: the ledgers
// the holder owns a live account in, any one of which may allow the request.
var holderLedgersCarriers = []string{"ledgerId<-path:holder_id=>holderLedgers[any]"}

var v1CreateAliasCarriers = []string{
	"accountId<-body:send.source.from[].accountAlias=>accountByAlias",
	"accountId<-body:send.distribute.to[].accountAlias=>accountByAlias",
}

var transactionAccountsCarriers = []string{"accountId<-path:transaction_id=>transactionAccounts"}

var balanceAccountCarriers = []string{"accountId<-path:balance_id=>balanceAccount"}

var aliasAccountCarriers = []string{"accountId<-path:alias=>accountByAlias"}

var externalAccountCarriers = []string{"accountId<-path:code=>externalAccount"}

var accountListQueryCarriers = []string{
	"portfolioId<-query:portfolio_id?",
	"segmentId<-query:segment_id?",
	"holderId<-query:holder_id?",
}

var v2CreateBodyCarriers = []string{
	"organizationId<-body:debits[].organizationId",
	"ledgerId<-body:debits[].ledgerId",
	"accountId<-body:debits[].alias=>accountByAlias",
	"organizationId<-body:credits[].organizationId",
	"ledgerId<-body:credits[].ledgerId",
	"accountId<-body:credits[].alias=>accountByAlias",
}

var accountUpdateBodyCarriers = []string{
	"portfolioId<-body:portfolioId?",
	"segmentId<-body:segmentId?",
}

// TestMidazManifest_ScopeRoutesDeclareTheirCarriers pins every route-level scope
// declaration of the embedded manifest: the exact routes, and on each the exact
// dimensions with where they are read and whether they may be left out.
func TestMidazManifest_ScopeRoutesDeclareTheirCarriers(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest),
		"embedded manifest must parse as a declaration manifest")
	require.NotNil(t, manifest.Scope, "the manifest declares a scope catalog")

	got := make(map[string][]string, len(manifest.Scope.Routes))

	for _, route := range manifest.Scope.Routes {
		key := route.Method + " " + route.Path
		require.NotContainsf(t, got, key, "route %s is declared twice", key)

		if len(route.Dimensions) == 0 {
			continue
		}

		carriers := make([]string, 0, len(route.Dimensions))

		for _, dim := range route.Dimensions {
			carrier := dim.Name + "<-" + dim.From + ":" + dim.Field
			if dim.Optional {
				carrier += "?"
			}

			if dim.Resolve != "" {
				carrier += "=>" + dim.Resolve
			}

			if dim.Match != "" {
				carrier += "[" + dim.Match + "]"
			}

			carriers = append(carriers, carrier)
		}

		got[key] = carriers
	}

	require.Equal(t, wantScopeRoutes, got, "route-level scope carriers drifted from the declaration")
}

// TestMidazManifest_OptsInToPartners pins the opt-in: without it the access
// manager grants no partner credential access to the ledger.
func TestMidazManifest_OptsInToPartners(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest))
	require.NoError(t, manifest.Validate())
	require.True(t, manifest.Partners, "the ledger manifest must opt in to partners")
}

// wantPermissionLevels pins how wide one instance of each resource is, which the
// access manager uses to refuse a partner a write wider than its scope.
var wantPermissionLevels = map[string]string{
	"organizations":            "tenant",
	"settings":                 "tenant",
	"streaming-manifest":       "tenant",
	"ledgers":                  "organization",
	"holders":                  "organization",
	"encryption":               "organization",
	"protection":               "organization",
	"operation-routes":         "organization",
	"transaction-routes":       "organization",
	"account-types":            "ledger",
	"assets":                   "ledger",
	"asset-rates":              "ledger",
	"portfolios":               "ledger",
	"segments":                 "ledger",
	"packages":                 "ledger",
	"billing-packages":         "ledger",
	"billing-calculate":        "ledger",
	"estimates":                "ledger",
	"dashboard":                "ledger",
	"accounts":                 "accountId",
	"balances":                 "accountId",
	"transactions":             "accountId",
	"operations":               "accountId",
	"fee-debts":                "accountId",
	"account-block-exceptions": "accountId",
	"instruments":              "accountId",
}

// TestMidazManifest_EveryPermissionDeclaresItsLevel requires a level on every
// permission line, the same for every action of a resource, and pins it.
func TestMidazManifest_EveryPermissionDeclaresItsLevel(t *testing.T) {
	t.Parallel()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest))
	require.NoError(t, manifest.Validate())

	got := make(map[string]string)

	for _, permission := range manifest.Permissions {
		require.NotEmptyf(t, permission.Level, "%s %s declares no level", permission.Resource, permission.Action)

		if previous, seen := got[permission.Resource]; seen {
			require.Equalf(t, previous, permission.Level, "%s declares two levels", permission.Resource)
		}

		got[permission.Resource] = permission.Level
	}

	require.Equal(t, wantPermissionLevels, got)
}

// wantDimensionParents pins the parent of every scope dimension, the dimension
// whose single instance holds one of its own; a root maps to "". Siblings share a
// parent and so rank the same when the access manager orders scope levels.
var wantDimensionParents = map[string]string{
	"organizationId":     "",
	"ledgerId":           "organizationId",
	"holderId":           "organizationId",
	"accountTypeId":      "ledgerId",
	"accountId":          "ledgerId",
	"assetId":            "ledgerId",
	"portfolioId":        "ledgerId",
	"segmentId":          "ledgerId",
	"transactionId":      "ledgerId",
	"operationRouteId":   "ledgerId",
	"transactionRouteId": "ledgerId",
	"billingPackageId":   "ledgerId",
	"packageId":          "ledgerId",
	"feeDebtId":          "ledgerId",
	"balanceId":          "accountId",
	"operationId":        "accountId",
	"instrumentId":       "holderId",
	"relatedPartyId":     "instrumentId",
}

func midazManifest(t *testing.T) declaration.DeclarationManifest {
	t.Helper()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledger.MidazManifest, &manifest))
	require.NoError(t, manifest.Validate())
	require.NotNil(t, manifest.Scope)

	return manifest
}

// TestMidazManifest_ScopeDimensionsDeclareTheirParent pins the parent of every
// dimension, so a dimension added without a decision about where it sits fails.
func TestMidazManifest_ScopeDimensionsDeclareTheirParent(t *testing.T) {
	t.Parallel()

	manifest := midazManifest(t)

	got := make(map[string]string, len(manifest.Scope.Dimensions))
	for _, dim := range manifest.Scope.Dimensions {
		got[dim.Name] = dim.Parent
	}

	require.Equal(t, wantDimensionParents, got)
}

// TestMidazManifest_BootRefusesABrokenHierarchy proves the hierarchy is checked at
// boot: declaration.New accepts the embedded manifest, and refuses the same
// manifest once its root is given a parent that closes a cycle.
func TestMidazManifest_BootRefusesABrokenHierarchy(t *testing.T) {
	t.Parallel()

	_, err := declaration.New(newConfig(midazSlug))
	require.NoError(t, err)

	manifest := midazManifest(t)

	for i := range manifest.Scope.Dimensions {
		if manifest.Scope.Dimensions[i].Name == "organizationId" {
			manifest.Scope.Dimensions[i].Parent = "accountId"
		}
	}

	cyclic, err := yaml.Marshal(manifest)
	require.NoError(t, err)

	config := newConfig(midazSlug)
	config.Manifest = cyclic

	_, err = declaration.New(config)
	require.ErrorContains(t, err, "makes a cycle")
}

// dimensionDepth is 1 for a root and its parent's depth plus 1 otherwise.
func dimensionDepth(t *testing.T, parents map[string]string, name string) int {
	t.Helper()

	depth := 1

	for parent := parents[name]; parent != ""; parent = parents[parent] {
		depth++
		require.LessOrEqualf(t, depth, len(parents), "following the parents of %s does not end at a root", name)
	}

	return depth
}

// TestMidazManifest_ScopeDimensionDepths pins the depth of every dimension: an
// account and a portfolio are siblings under the ledger, neither narrower than
// the other, so a partner confined to a portfolio may still be granted account
// writes.
func TestMidazManifest_ScopeDimensionDepths(t *testing.T) {
	t.Parallel()

	manifest := midazManifest(t)

	parents := make(map[string]string, len(manifest.Scope.Dimensions))
	for _, dim := range manifest.Scope.Dimensions {
		parents[dim.Name] = dim.Parent
	}

	tests := []struct {
		dimension string
		depth     int
	}{
		{"organizationId", 1},
		{"ledgerId", 2},
		{"holderId", 2},
		{"accountTypeId", 3},
		{"accountId", 3},
		{"assetId", 3},
		{"portfolioId", 3},
		{"segmentId", 3},
		{"transactionId", 3},
		{"operationRouteId", 3},
		{"transactionRouteId", 3},
		{"billingPackageId", 3},
		{"packageId", 3},
		{"feeDebtId", 3},
		{"instrumentId", 3},
		{"balanceId", 4},
		{"operationId", 4},
		{"relatedPartyId", 4},
	}

	require.Len(t, tests, len(parents), "every dimension has a row")

	for _, tt := range tests {
		require.Equalf(t, tt.depth, dimensionDepth(t, parents, tt.dimension), "depth of %s", tt.dimension)
	}

	require.Equal(t, dimensionDepth(t, parents, "portfolioId"), dimensionDepth(t, parents, "accountId"),
		"portfolio and account are siblings")
	require.Equal(t, parents["portfolioId"], parents["accountId"], "portfolio and account share their parent")
}
