// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dimTriples flattens dimensions to name/key/source triples so a failure names the
// value that was wrong instead of printing an opaque struct.
func dimTriples(dims []middleware.Dimension) [][3]any {
	out := make([][3]any, 0, len(dims))
	for _, d := range dims {
		out = append(out, [3]any{d.Name(), d.Key(), d.Source()})
	}

	return out
}

// TestMidazScopeDims_DerivesFromPath pins the derivation table: which Fiber path
// yields which dimensions, in which order, read from which request key.
func TestMidazScopeDims_DerivesFromPath(t *testing.T) {
	t.Parallel()

	orgDim := [3]any{"organizationId", "organization_id", middleware.FromPath}
	ledgerDim := [3]any{"ledgerId", "ledger_id", middleware.FromPath}

	rows := []struct {
		name string
		path string
		want [][3]any
	}{
		{
			name: "organization and ledger",
			path: "/organizations/:organization_id/ledgers/:ledger_id/accounts",
			want: [][3]any{orgDim, ledgerDim},
		},
		{
			name: "organization only",
			path: "/organizations/:organization_id/holders",
			want: [][3]any{orgDim},
		},
		{
			name: "no identifier at all",
			path: "/settings/metadata-indexes",
			want: [][3]any{},
		},
		{
			name: "root",
			path: "/",
			want: [][3]any{},
		},
		{
			name: "deep path keeps organization first, ledger second",
			path: "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/operations/:operation_id",
			want: [][3]any{orgDim, ledgerDim},
		},
		{
			// ":id" is a RESOURCE's own identifier — an account's, an asset's — on
			// every surface that still spells it that way. Reading it as the
			// organization would forward an account id as the organization id.
			name: "a bare :id is never the organization",
			path: "/organizations/:id",
			want: [][3]any{},
		},
		{
			name: "a resource :id alongside the real parameters adds nothing",
			path: "/organizations/:organization_id/ledgers/:ledger_id/accounts/:id",
			want: [][3]any{orgDim, ledgerDim},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, row.want, dimTriples(midazScopeDims(row.path)))
		})
	}
}

// TestMidazScopeDims_SegmentMustMatchWhole is trap #1: a substring comparison would
// read :organization_identifier as the organization and forward a value that names a
// different thing entirely.
func TestMidazScopeDims_SegmentMustMatchWhole(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/organizations/:organization_identifier/things",
		"/organizations/:sub_organization_id/things",
		"/ledgers/:ledger_id_v2/things",
		"/ledgers/:parent_ledger_id/things",
	} {
		assert.Empty(t, midazScopeDims(path), "path %q must derive no dimension", path)
	}
}

// TestMidazScopeDims_ParameterMarkerIsRequired is trap #2: a LITERAL segment spelled
// organization_id is not a parameter. Treating it as one declares a dimension whose
// value resolves empty, and an empty declared value denies every caller on the route.
func TestMidazScopeDims_ParameterMarkerIsRequired(t *testing.T) {
	t.Parallel()

	assert.Empty(t, midazScopeDims("/organizations/organization_id/ledgers/ledger_id"))
	assert.Empty(t, midazScopeDims("/organization_id"))
	assert.Equal(t,
		[][3]any{{"organizationId", "organization_id", middleware.FromPath}},
		dimTriples(midazScopeDims("/organizations/:organization_id/ledgers/ledger_id")),
		"only the parameter segment counts; the literal twin must not add a dimension")
}

// TestMidazScopeDims_OrderIsFixedNotPositional is trap #3: what a route declares is a
// function of the path, never of the order the segments happen to appear in.
func TestMidazScopeDims_OrderIsFixedNotPositional(t *testing.T) {
	t.Parallel()

	natural := dimTriples(midazScopeDims("/organizations/:organization_id/ledgers/:ledger_id"))
	inverted := dimTriples(midazScopeDims("/ledgers/:ledger_id/organizations/:organization_id"))

	require.Len(t, natural, 2)
	assert.Equal(t, natural, inverted)
	assert.Equal(t, "organizationId", natural[0][0])
	assert.Equal(t, "ledgerId", natural[1][0])
}

// TestMidazScopeDeclarations_UnscopedPathDeclaresNothing is the fail-closed contract: a
// route whose path names no instance must be INDISTINGUISHABLE from one that never
// declared a scope. An empty declaration IS a declaration, and lib-auth honours a
// declaration that carries no dimension as a route that declared nothing anyway — but a
// future reader would take it for "this route is scoped".
func TestMidazScopeDeclarations_UnscopedPathDeclaresNothing(t *testing.T) {
	t.Parallel()

	assert.Nil(t, midazScopeDeclarations("/settings/metadata-indexes"))
	assert.Nil(t, midazScopeDeclarations("/organizations"))
	assert.Nil(t, midazScopeDeclarations("/transactions/json"))

	scoped := midazScopeDeclarations("/organizations/:organization_id/ledgers/:ledger_id/accounts")
	require.Len(t, scoped, 1, "a scoped path declares exactly one declaration")
}
