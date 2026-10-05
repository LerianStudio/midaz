// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/google/uuid"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// Scope dimensions a list route may be confined on.
const (
	scopeDimensionAccount   = "accountId"
	scopeDimensionPortfolio = "portfolioId"
	scopeDimensionSegment   = "segmentId"
	scopeDimensionLedger    = "ledgerId"
	scopeDimensionHolder    = "holderId"
)

// accountListScopeDimensions are the dimensions the account list and its count
// are confined on.
var accountListScopeDimensions = []string{scopeDimensionAccount, scopeDimensionPortfolio, scopeDimensionSegment}

// holderAccountListScopeDimensions are the dimensions a holder's account list is
// confined on: those of the account list, plus the ledger, since a holder's
// accounts span every ledger of the organization.
var holderAccountListScopeDimensions = append([]string{scopeDimensionLedger}, accountListScopeDimensions...)

// instrumentListScopeDimensions are the dimensions the instrument list is
// confined on.
var instrumentListScopeDimensions = []string{scopeDimensionLedger, scopeDimensionAccount, scopeDimensionHolder}

// listScope reads, for each dimension the route filters on, the instances the
// authorization decision allows a partner credential to see. It returns nil for
// every other caller. A value that is not an id names no instance and is dropped,
// which confines the list further, never less.
func listScope(ctx context.Context, dimensions ...string) pkgHTTP.ScopeConfinement {
	scope, partner := middleware.ScopeFromContext(ctx)
	if !partner {
		return nil
	}

	var confinement pkgHTTP.ScopeConfinement

	for _, dimension := range dimensions {
		values, confined := scope.Allowed(dimension)
		if !confined {
			continue
		}

		ids := make([]uuid.UUID, 0, len(values))

		for _, value := range values {
			if id, err := uuid.Parse(value); err == nil {
				ids = append(ids, id)
			}
		}

		if confinement == nil {
			confinement = pkgHTTP.ScopeConfinement{}
		}

		confinement[dimension] = ids
	}

	return confinement
}
