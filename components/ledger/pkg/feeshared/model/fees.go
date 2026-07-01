// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
)

// FeeCalculate is the internal engine carrier for fee calculation. The fee
// engine mutates the embedded transaction in place, so it stays a mutable,
// full-transaction envelope. It is projected onto the wire as FeeEstimateResult
// and never appears in the API schema itself.
type FeeCalculate struct {
	SegmentID   *uuid.UUID              `json:"segmentId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID    uuid.UUID               `json:"ledgerId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	Transaction transaction.Transaction `json:"transaction"`
}

// FeeEstimateResponse is a struct designed to encapsulate response of estimate fee.
type FeeEstimateResponse struct {
	Message     string             `json:"message" example:"Successfully estimated fee."`
	FeesApplied *FeeEstimateResult `json:"feesApplied"`
}

// FeeEstimate is a struct designed to encapsulate request create payload data.
type FeeEstimate struct {
	PackageID   uuid.UUID               `json:"packageId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID    uuid.UUID               `json:"ledgerId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	Transaction transaction.Transaction `json:"transaction"` // Full transaction projection; rendered as TransactionInput in the API schema.
}
