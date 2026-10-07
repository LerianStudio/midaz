// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

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

// linkIDs is the operation route IDs of links, in order.
func linkIDs(links []transactionroute.OperationRouteLink) []uuid.UUID {
	if len(links) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.OperationRouteID)
	}

	return ids
}
