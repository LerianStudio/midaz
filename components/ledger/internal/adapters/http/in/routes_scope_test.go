// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dimTriple is one dimension flattened to the three things that decide what the
// authorization service is asked: the field name it knows the dimension by, where in
// the request the value is read, and under which request key.
type dimTriple struct {
	Name   string
	Source middleware.Source
	Key    string
}

func flattenDims(dims []middleware.Dimension) []dimTriple {
	if len(dims) == 0 {
		return nil
	}

	out := make([]dimTriple, 0, len(dims))
	for _, d := range dims {
		out = append(out, dimTriple{Name: d.Name(), Source: d.Source(), Key: d.Key()})
	}

	return out
}

var (
	orgDim    = dimTriple{Name: "organizationId", Source: middleware.FromPath, Key: "organization_id"}
	ledgerDim = dimTriple{Name: "ledgerId", Source: middleware.FromPath, Key: "ledger_id"}
)

// TestMidazScopeDims_DerivesFromPath locks which instance identifiers each route shape
// declares. The declaration decides the "where" half of every authorization question, so
// a path that names a ledger and declares no ledgerId would ask a WIDER question than the
// route serves — a partner scoped to one ledger would reach every ledger. The reverse,
// declaring an identifier the path does not carry, denies every caller on that route:
// lib-auth refuses a declared dimension the request has no value for.
func TestMidazScopeDims_DerivesFromPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want []dimTriple
	}{
		{
			name: "organization and ledger both named",
			path: "/organizations/:organization_id/ledgers/:ledger_id/accounts",
			want: []dimTriple{orgDim, ledgerDim},
		},
		{
			name: "deep path under a ledger still names both",
			path: "/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/operations/:operation_id",
			want: []dimTriple{orgDim, ledgerDim},
		},
		{
			name: "ledger collection names the organization only",
			path: "/organizations/:organization_id/ledgers",
			want: []dimTriple{orgDim},
		},
		{
			name: "the ledger itself names both",
			path: "/organizations/:organization_id/ledgers/:ledger_id",
			want: []dimTriple{orgDim, ledgerDim},
		},
		{
			name: "organization-level surface names the organization only",
			path: "/organizations/:organization_id/holders",
			want: []dimTriple{orgDim},
		},
		{
			name: "organization collection names nothing",
			path: "/organizations",
			want: nil,
		},
		{
			name: "organization by id names nothing: the id is not the organization dimension key",
			path: "/organizations/:id",
			want: nil,
		},
		{
			name: "v2 transaction create names nothing: its scope lives in the body",
			path: "/transactions/direct",
			want: nil,
		},
		{
			name: "streaming manifest names nothing",
			path: "/v1/streaming/manifest",
			want: nil,
		},
		{
			name: "a segment that merely contains the parameter name is not the parameter",
			path: "/organizations/:organization_identifier/things",
			want: nil,
		},
		{
			name: "a literal segment spelled like the parameter without the colon is not a parameter",
			path: "/organizations/organization_id/ledgers/ledger_id/accounts",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, flattenDims(midazScopeDims(tc.path)))
		})
	}
}

// TestMidazScopeDeclarations_OnlyWhenThePathNamesSomething locks the shape handed to
// lib-auth's Authorize: a path that names an identifier produces exactly ONE declaration,
// and a path that names none produces ZERO — never an empty declaration. The difference
// is load-bearing: Authorize treats "no declaration" and "a declaration with no
// dimensions" identically only by accident today, and an empty declaration would still
// be a declaration a future check could read as "this route is scoped".
func TestMidazScopeDeclarations_OnlyWhenThePathNamesSomething(t *testing.T) {
	t.Parallel()

	scoped := midazScopeDeclarations("/organizations/:organization_id/ledgers/:ledger_id/accounts")
	require.Len(t, scoped, 1, "a path naming identifiers must produce exactly one declaration")

	unscoped := midazScopeDeclarations("/organizations")
	assert.Empty(t, unscoped, "a path naming no identifier must produce no declaration at all")
}
