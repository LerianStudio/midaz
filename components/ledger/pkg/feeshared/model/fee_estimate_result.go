// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
)

// FeeAdjustedTransaction is the wire-only projection of the fee-adjusted
// transaction returned by the estimate endpoint. It mirrors the output-relevant
// fields of the transaction input shape but omits the deprecated Route field —
// callers consume routeId.
type FeeAdjustedTransaction struct {
	ChartOfAccountsGroupName string                       `json:"chartOfAccountsGroupName,omitempty" example:"FUNDING"`
	Description              string                       `json:"description,omitempty" example:"Description"`
	Code                     string                       `json:"code,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Pending                  bool                         `json:"pending,omitempty" example:"false"`
	Metadata                 map[string]any               `json:"metadata,omitempty"`
	RouteID                  *string                      `json:"routeId,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	TransactionDate          *transaction.TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z"`
	Send                     transaction.Send             `json:"send"`
}

// FeeEstimateResult is the response-only DTO for the fee-estimate endpoint. It
// projects the engine carrier (FeeCalculate) onto the wire, keeping the mutable
// full-transaction carrier out of the public schema.
type FeeEstimateResult struct {
	LedgerID    uuid.UUID              `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	SegmentID   *uuid.UUID             `json:"segmentId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Transaction FeeAdjustedTransaction `json:"transaction"`
}

// NewFeeEstimateResult projects the engine carrier into the wire DTO. It copies
// the scalar/id fields, carries Send/Metadata/RouteID/TransactionDate through,
// and drops the deprecated Route field. The carrier is not mutated.
func NewFeeEstimateResult(fc *FeeCalculate) FeeEstimateResult {
	return FeeEstimateResult{
		LedgerID:  fc.LedgerID,
		SegmentID: fc.SegmentID,
		Transaction: FeeAdjustedTransaction{
			ChartOfAccountsGroupName: fc.Transaction.ChartOfAccountsGroupName,
			Description:              fc.Transaction.Description,
			Code:                     fc.Transaction.Code,
			Pending:                  fc.Transaction.Pending,
			Metadata:                 fc.Transaction.Metadata,
			RouteID:                  fc.Transaction.RouteID,
			TransactionDate:          fc.Transaction.TransactionDate,
			Send:                     fc.Transaction.Send,
		},
	}
}
