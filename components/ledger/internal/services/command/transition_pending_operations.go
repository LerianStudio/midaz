// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"

// mergeTransactionOperations joins the operations already associated with a
// transaction to the operations produced by its pending-state transition. The
// first occurrence of an operation wins, so the original hold remains before
// the commit or cancel legs and repeated IDs are not exposed by cached reads.
func mergeTransactionOperations(prior, next []*operation.Operation) []*operation.Operation {
	merged := make([]*operation.Operation, 0, len(prior)+len(next))
	seen := make(map[string]struct{}, len(prior)+len(next))

	for _, operations := range [][]*operation.Operation{prior, next} {
		for _, op := range operations {
			if op == nil {
				continue
			}

			if _, ok := seen[op.ID]; ok {
				continue
			}

			seen[op.ID] = struct{}{}
			merged = append(merged, op)
		}
	}

	return merged
}
