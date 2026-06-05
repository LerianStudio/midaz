// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
)

// FeeCalculate is a struct designed to encapsulate request create payload data.
//
// swagger:model FeeCalculate
//
//	@Description	FeeCalculate is the input payload to create a fee.
type FeeCalculate struct {
	SegmentID   *uuid.UUID              `json:"segmentId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID    uuid.UUID               `json:"ledgerId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	Transaction transaction.Transaction `json:"transaction"`
} //	@name	FeeCalculate

// FeeEstimateResponse is a struct designed to encapsulate response of estimate fee.
//
// swagger:model FeeEstimateResponse
//
//	@Description	FeeEstimateResponse is the response payload for estimate fee
type FeeEstimateResponse struct {
	Message     string        `json:"message" example:"Successfully estimated fee."`
	FeesApplied *FeeCalculate `json:"feesApplied"`
} //	@name	FeeEstimateResponse

// FeeEstimate is a struct designed to encapsulate request create payload data.
//
// swagger:model FeeEstimate
//
//	@Description	FeeEstimate is the input payload to create a fee estimate.
type FeeEstimate struct {
	PackageID   uuid.UUID               `json:"packageId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID    uuid.UUID               `json:"ledgerId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	Transaction transaction.Transaction `json:"transaction"`
} //	@name	FeeEstimate
