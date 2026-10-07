// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
)

// requiredLinkMap is the link map a repository answers when every link it
// holds is required.
func requiredLinkMap(ids map[uuid.UUID][]uuid.UUID) map[uuid.UUID][]transactionroute.OperationRouteLink {
	links := make(map[uuid.UUID][]transactionroute.OperationRouteLink, len(ids))

	for transactionRouteID, operationRouteIDs := range ids {
		for _, operationRouteID := range operationRouteIDs {
			links[transactionRouteID] = append(links[transactionRouteID], transactionroute.OperationRouteLink{OperationRouteID: operationRouteID})
		}
	}

	return links
}
