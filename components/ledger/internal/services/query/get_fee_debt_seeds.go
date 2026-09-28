// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// GetFeeDebtSeeds reports no open fee debt for any balance.
func (uc *UseCase) GetFeeDebtSeeds(_ context.Context, _, _ uuid.UUID, _ []string) (map[string][]accounting.FeeDebtItem, error) {
	return map[string][]accounting.FeeDebtItem{}, nil
}
