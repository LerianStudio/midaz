// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
)

// An authorization question has two halves. The resource and the action say WHAT is being
// done; the instance identifiers say WHERE — which organization, which ledger. A
// credential allowed to act on some ledgers and not others can only be honoured when the
// second half is sent, and the route is the only place that knows where those identifiers
// sit in its own request.
//
// The names below are the authorization service's, for the "midaz" product, and they are
// exactly two. An identifier sent under a name the service does not know for this product
// matches nothing — and a dimension nobody matches never denies — so a name invented here
// would WIDEN access rather than fail. Adding a third name is a change on both sides.
const (
	// scopeFieldOrganizationID is the declared field name of the organization dimension.
	scopeFieldOrganizationID = "organizationId"
	// scopeFieldLedgerID is the declared field name of the ledger dimension.
	scopeFieldLedgerID = "ledgerId"
)

// The Fiber path parameters those two dimensions are read from. They differ from the
// declared field names because the route names its own parameters and the authorization
// service names its own fields; neither side renames for the other.
const (
	scopeParamOrganizationID = "organization_id"
	scopeParamLedgerID       = "ledger_id"
)

// midazScopeDims derives, from a route's Fiber path, the instance identifiers that
// route's requests carry.
//
// Derivation from the path — rather than a per-route list someone maintains by hand — is
// what makes the declaration impossible to forget: a new route under
// /organizations/:organization_id/ledgers/:ledger_id is scoped the moment it is mounted,
// with nothing to remember.
//
// The match is on WHOLE segments beginning with ':'. A substring match would treat
// ":organization_identifier" as the organization parameter and read an unrelated value as
// the organization the request points at; a match ignoring the colon would treat the
// literal segment "organization_id" as a parameter, whose Params lookup returns empty and
// whose empty value denies every caller on that route.
//
// The order is fixed — organization first, then ledger — so what a route declares is a
// function of its path alone and not of the order segments happen to appear in.
func midazScopeDims(path string) []middleware.Dimension {
	var (
		hasOrganization bool
		hasLedger       bool
	)

	for _, segment := range strings.Split(path, "/") {
		switch segment {
		case ":" + scopeParamOrganizationID:
			hasOrganization = true
		case ":" + scopeParamLedgerID:
			hasLedger = true
		}
	}

	var dims []middleware.Dimension

	if hasOrganization {
		dims = append(dims, middleware.Dim(scopeFieldOrganizationID, middleware.FromPath).At(scopeParamOrganizationID))
	}

	if hasLedger {
		dims = append(dims, middleware.Dim(scopeFieldLedgerID, middleware.FromPath).At(scopeParamLedgerID))
	}

	return dims
}

// midazScopeDeclarations wraps what the path names into the optional argument
// Authorize takes: one declaration when the path names at least one identifier, and NONE
// when it names none.
//
// The empty case returns no declaration rather than an empty one on purpose. A route that
// cannot say which instance a request points at must stay indistinguishable from a route
// that never declared anything, because that is the state lib-auth refuses a
// partner-bound credential on. An empty declaration would be a declaration, and a later
// reader — here or there — could take it for "this route is scoped".
func midazScopeDeclarations(path string) []middleware.ScopeDeclaration {
	dims := midazScopeDims(path)
	if len(dims) == 0 {
		return nil
	}

	return []middleware.ScopeDeclaration{middleware.RequireScope(midazName, dims...)}
}
