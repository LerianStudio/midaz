// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
)

// This file answers the "where" half of every authorization question the ledger
// asks. The resource/action tuple says WHAT a caller wants to do; the scope
// dimensions say WHICH instance — which organization, which ledger — the request
// points at. Without the second half a credential confined to one organization
// reaches every other one, because the confinement is never evaluated.

// Scope field names, as the AUTHORIZATION SERVICE knows them. They are the
// midaz product catalog's declared fields: organizationId is required, ledgerId
// optional and multi-valued. A name the catalog does not know matches nothing —
// and a dimension nobody matches never denies — so sending an invented field
// WIDENS access instead of narrowing it. Adding a third dimension is therefore a
// change on both sides, never here alone.
const (
	scopeFieldOrganizationID = "organizationId"
	scopeFieldLedgerID       = "ledgerId"
)

// Path parameter names, as the ROUTES spell them in their own Fiber paths.
//
// These are deliberately NOT the constants above, and neither side renames to
// match the other: the authorization catalog owns its field names, the routes own
// their parameter names, and Dimension.At is what bridges the two.
const (
	pathParamOrganizationID = "organization_id"
	pathParamLedgerID       = "ledger_id"
)

// midazScopeDims derives the instance dimensions a Fiber route path carries.
//
// Deriving from the path — rather than keeping a per-route list — is what makes
// the declaration impossible to forget: a route mounted tomorrow under
// /organizations/:organization_id/ledgers/:ledger_id is scoped the moment it is
// registered, with nobody having to remember to add it anywhere.
//
// Three properties are load-bearing, each with its own test:
//
//  1. Segments are compared WHOLE. A substring test would accept
//     :organization_identifier as the organization parameter and forward, as the
//     organization, a value that names something else.
//  2. The ":" marker is REQUIRED. A literal segment spelled organization_id is
//     not a parameter; declaring a dimension for it resolves to an empty value,
//     and a declared dimension that resolves empty denies every caller on the route.
//  3. The order is FIXED — organization, then ledger — not the order the segments
//     appear in. What a route declares is a function of its path alone.
func midazScopeDims(path string) []middleware.Dimension {
	var hasOrganization, hasLedger bool

	for _, segment := range strings.Split(path, "/") {
		param, isParam := strings.CutPrefix(segment, ":")
		if !isParam {
			continue
		}

		switch param {
		case pathParamOrganizationID:
			hasOrganization = true
		case pathParamLedgerID:
			hasLedger = true
		}
	}

	dims := make([]middleware.Dimension, 0, 2)

	if hasOrganization {
		dims = append(dims, middleware.Dim(scopeFieldOrganizationID, middleware.FromPath).At(pathParamOrganizationID))
	}

	if hasLedger {
		dims = append(dims, middleware.Dim(scopeFieldLedgerID, middleware.FromPath).At(pathParamLedgerID))
	}

	return dims
}

// midazScopeDeclarations wraps the derived dimensions in the variadic argument
// Authorize takes, and returns NIL for a path that names no instance.
//
// Nil rather than an empty declaration is the point. A route whose path cannot say
// which instance a request points at must be indistinguishable from one that never
// declared a scope at all — that is the state in which a partner-bound credential
// is refused. An empty declaration IS a declaration, and the next reader of the
// code would take it to mean "this route is scoped", which it is not.
func midazScopeDeclarations(path string) []middleware.ScopeDeclaration {
	dims := midazScopeDims(path)
	if len(dims) == 0 {
		return nil
	}

	return []middleware.ScopeDeclaration{middleware.RequireScope(midazName, dims...)}
}
