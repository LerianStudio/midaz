// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestUpdateFeeMetadataIfNeeded_RealChargeSetsFeeApplied locks the charged-only
// contract: when the validation result grew (a fee was actually charged), both
// packageAppliedID and feeApplied=true are set.
func TestUpdateFeeMetadataIfNeeded_RealChargeSetsFeeApplied(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}
	packID := uuid.New()

	cf := &model.FeeCalculate{
		Transaction: transaction.Transaction{Metadata: nil},
	}

	// A real charge: the From map grew relative to the pre-calc size.
	validationResult := &transaction.Responses{
		From: map[string]transaction.Amount{"@fee_account": {}},
		To:   map[string]transaction.Amount{},
	}

	uc.updateFeeMetadataIfNeeded(cf, validationResult, 0, 0, packID)

	assert.NotNil(t, cf.Transaction.Metadata)
	assert.Equal(t, packID.String(), cf.Transaction.Metadata["packageAppliedID"])
	assert.Equal(t, "true", cf.Transaction.Metadata["feeApplied"],
		"feeApplied must be set on the real-charge branch")
}

// TestUpdateFeeMetadataIfNeeded_ExemptionOnlyOmitsFeeApplied locks the
// exemption fence: an exemption-only path sets packageAppliedID (existing
// behavior) but MUST NOT set feeApplied — no fee was actually charged.
func TestUpdateFeeMetadataIfNeeded_ExemptionOnlyOmitsFeeApplied(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}
	packID := uuid.New()

	cf := &model.FeeCalculate{
		Transaction: transaction.Transaction{
			Metadata: map[string]any{"feeExemption": map[string]any{}},
		},
	}

	// No charge: map sizes unchanged from the pre-calc sizes.
	validationResult := &transaction.Responses{
		From: map[string]transaction.Amount{},
		To:   map[string]transaction.Amount{},
	}

	uc.updateFeeMetadataIfNeeded(cf, validationResult, 0, 0, packID)

	assert.Equal(t, packID.String(), cf.Transaction.Metadata["packageAppliedID"])
	_, hasFeeApplied := cf.Transaction.Metadata["feeApplied"]
	assert.False(t, hasFeeApplied,
		"feeApplied must NOT be set on the exemption-only branch")
}
