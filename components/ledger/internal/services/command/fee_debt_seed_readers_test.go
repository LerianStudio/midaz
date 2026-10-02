// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// The test readers below embed a nil TransactionReader; a v2 create reads fee-debt
// seeds, and none of them holds any debt.

func (reader *createEngineReader) GetFeeDebtSeeds(context.Context, uuid.UUID, uuid.UUID, []string) (map[string][]accounting.FeeDebtItem, error) {
	return nil, nil
}

func (reader *atomicTransactionBatchSettingsReader) GetFeeDebtSeeds(context.Context, uuid.UUID, uuid.UUID, []string) (map[string][]accounting.FeeDebtItem, error) {
	return nil, nil
}

func (reader enginePreparationReader) GetFeeDebtSeeds(context.Context, uuid.UUID, uuid.UUID, []string) (map[string][]accounting.FeeDebtItem, error) {
	return nil, nil
}

func (reader *revertProjectionReader) GetFeeDebtSeeds(context.Context, uuid.UUID, uuid.UUID, []string) (map[string][]accounting.FeeDebtItem, error) {
	return nil, nil
}

func (reader *transitionEngineReader) GetFeeDebtSeeds(context.Context, uuid.UUID, uuid.UUID, []string) (map[string][]accounting.FeeDebtItem, error) {
	return nil, nil
}
