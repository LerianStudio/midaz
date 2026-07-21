// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestNewFeeEstimateResult_ProjectsAndDropsRoute verifies the engine carrier
// (FeeCalculate) is projected into the wire-only DTO: scalars/ids copied,
// metadata (including injected keys) and rewritten Send legs preserved, the
// deprecated Route field structurally absent, and the source left unmutated.
func TestNewFeeEstimateResult_ProjectsAndDropsRoute(t *testing.T) {
	t.Parallel()

	ledgerID := uuid.New()
	segmentID := uuid.New()
	routeID := uuid.New().String()

	fc := &FeeCalculate{
		LedgerID:  ledgerID,
		SegmentID: &segmentID,
		Transaction: transaction.Transaction{
			ChartOfAccountsGroupName: "FUNDING",
			Description:              "fee estimate",
			Code:                     "FEE-001",
			Pending:                  true,
			// Deprecated route value: must NOT survive the projection (the DTO has no Route field).
			Route:   "11111111-1111-1111-1111-111111111111",
			RouteID: &routeID,
			Metadata: map[string]any{
				"packageAppliedID": "pkg-123",
				"feeExemption":     true,
			},
			Send: transaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(1700),
				Source: transaction.Source{
					From: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1600)},
						IsFrom: true,
					}},
				},
				Distribute: transaction.Distribute{
					To: []transaction.FromTo{{
						Amount: &transaction.Amount{Asset: "BRL", Value: decimal.NewFromInt(1700)},
					}},
				},
			},
		},
	}

	got := NewFeeEstimateResult(fc)

	// Scalars / ids copied verbatim.
	assert.Equal(t, ledgerID, got.LedgerID)
	assert.Equal(t, &segmentID, got.SegmentID)

	// Transaction projected field-by-field.
	assert.Equal(t, "FUNDING", got.Transaction.ChartOfAccountsGroupName)
	assert.Equal(t, "fee estimate", got.Transaction.Description)
	assert.Equal(t, "FEE-001", got.Transaction.Code)
	assert.True(t, got.Transaction.Pending)
	assert.Equal(t, &routeID, got.Transaction.RouteID)

	// Metadata passed through, including engine-injected keys.
	assert.Equal(t, "pkg-123", got.Transaction.Metadata["packageAppliedID"])
	assert.Equal(t, true, got.Transaction.Metadata["feeExemption"])

	// Rewritten Send legs preserved.
	assert.True(t, got.Transaction.Send.Value.Equal(decimal.NewFromInt(1700)))
	assert.Len(t, got.Transaction.Send.Source.From, 1)
	assert.Len(t, got.Transaction.Send.Distribute.To, 1)

	// Source carrier left unmutated (projection is a pure copy).
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", fc.Transaction.Route)
}

// TestNewFeeEstimateResult_NilMetadata verifies the projection tolerates the
// "no fee applied" path where the carrier transaction has nil metadata.
func TestNewFeeEstimateResult_NilMetadata(t *testing.T) {
	t.Parallel()

	ledgerID := uuid.New()

	fc := &FeeCalculate{
		LedgerID: ledgerID,
		Transaction: transaction.Transaction{
			Send: transaction.Send{Asset: "BRL", Value: decimal.NewFromInt(100)},
		},
	}

	got := NewFeeEstimateResult(fc)

	assert.Equal(t, ledgerID, got.LedgerID)
	assert.Nil(t, got.SegmentID)
	assert.Nil(t, got.Transaction.Metadata)
}
