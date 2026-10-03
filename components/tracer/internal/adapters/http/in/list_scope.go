// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/google/uuid"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// tracerListScopeDimensions are the dimensions the validation and audit-event
// lists are confined on.
var tracerListScopeDimensions = []string{"accountId", "segmentId", "portfolioId", "merchantId"}

// tracerListScope reads, for each dimension a list filters on, the values the
// authorization decision allows a partner credential to see. It returns nil for
// every other caller. A value that is not an id names nothing and is dropped,
// which confines the list further, never less.
func tracerListScope(ctx context.Context, dimensions ...string) pkgHTTP.ScopeConfinement {
	scope, partner := authMiddleware.ScopeFromContext(ctx)
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
