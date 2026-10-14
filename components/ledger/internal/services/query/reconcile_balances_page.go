// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// ReconcileBalancesPage reads one keyset page of balances after cursor, each paired
// with the head of its operation trail, for the balance reconciler. It only reads;
// classification is left to the caller. next is the last balance id of the page,
// and a page shorter than pageSize means the table ended.
func (uc *UseCase) ReconcileBalancesPage(ctx context.Context, cursor uuid.UUID, pageSize int) ([]mmodel.BalanceTrailPair, uuid.UUID, error) {
	pairs, next, err := uc.BalanceRepo.ListBalanceTrailPairs(ctx, cursor, pageSize)
	if err != nil {
		return nil, cursor, fmt.Errorf("list balance trail pairs: %w", err)
	}

	return pairs, next, nil
}
