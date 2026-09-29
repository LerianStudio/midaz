// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// feeDebtListVersion is the only version of the live fee-debt list value.
const feeDebtListVersion = 1

// GetFeeDebtSeeds reads the live fee-debt list of every balance ref in one MGET,
// behind the same tenant prefix as the balance keys, and returns the open debts
// of each ref that has any. The engine re-reads the live keys; this is a seed.
func (uc *UseCase) GetFeeDebtSeeds(ctx context.Context, organizationID, ledgerID uuid.UUID, balanceRefs []string) (map[string][]accounting.FeeDebtItem, error) {
	keys := make([]string, 0, len(balanceRefs))
	refsByKey := make(map[string]string, len(balanceRefs))

	for _, ref := range balanceRefs {
		key := utils.FeeDebtInternalKey(organizationID, ledgerID, ref)
		keys = append(keys, key)
		refsByKey[key] = ref
	}

	values, err := uc.TransactionRedisRepo.MGet(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("read fee debt lists: %w", err)
	}

	seeds := make(map[string][]accounting.FeeDebtItem)

	for key, value := range values {
		var list struct {
			V     int                      `json:"v"`
			Items []accounting.FeeDebtItem `json:"items"`
		}

		if err := json.Unmarshal([]byte(value), &list); err != nil {
			return nil, fmt.Errorf("decode fee debt list of %s: %w", refsByKey[key], err)
		}

		if list.V != feeDebtListVersion {
			return nil, fmt.Errorf("fee debt list of %s has version %d", refsByKey[key], list.V)
		}

		if len(list.Items) > 0 {
			seeds[refsByKey[key]] = list.Items
		}
	}

	return seeds, nil
}
