// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	fees "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TestFeeExemptionMetadataCompletes drives a posting whose fee package waives the
// payer through the real fee use case and the real completer: the feeExemption
// value is flat, so the executed transaction is persisted with its metadata.
func TestFeeExemptionMetadataCompletes(t *testing.T) {
	ctrl := gomock.NewController(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	notDeductible := false
	waived := []string{"@payer"}

	repo := pack.NewMockRepository(ctrl)
	repo.EXPECT().FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).Return([]*pack.Package{{
		ID: uuid.New(), MinimumAmount: decimal.NewFromInt(1), MaximumAmount: decimal.NewFromInt(100000),
		WaivedAccounts: &waived,
		Fees: map[string]model.Fee{"flat": {
			FeeLabel: "flat", ReferenceAmount: "originalAmount", Priority: 1, IsDeductibleFrom: &notDeductible, CreditAccount: "@fee_account",
			CalculationModel: &model.CalculationModel{ApplicationRule: "flatFee", Calculations: []model.Calculation{{Type: "flat", Value: "10"}}},
		}},
	}}, nil)

	resolver := feeshared.NewMockMidazResolver(ctrl)
	resolver.EXPECT().GetAccountByAlias(gomock.Any(), orgID, ledgerID, gomock.Any()).Return(nil, nil).AnyTimes()

	applier, err := fees.NewUseCase(repo, resolver)
	require.NoError(t, err)

	amount := decimal.NewFromInt(1000)
	input := mtransaction.Transaction{Send: mtransaction.Send{
		Asset: "BRL", Value: amount,
		Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "@payer", Amount: &mtransaction.Amount{Asset: "BRL", Value: amount}}}},
		Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "@payee", Amount: &mtransaction.Amount{Asset: "BRL", Value: amount}}}},
	}}

	uc := &UseCase{FeeApplier: applier}
	require.NoError(t, uc.applyFees(context.Background(), &input, orgID, ledgerID, false, false))

	raw, ok := input.Metadata["feeExemption"].(string)
	require.True(t, ok, "feeExemption must be a flat string: %#v", input.Metadata["feeExemption"])

	var exemption map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &exemption))
	assert.Equal(t, map[string]any{"exempt": true, "reason": "all_source_accounts_exempt", "message": "All source accounts are exempt from fees."}, exemption)

	payload, result := recoveryContractFixture(t)
	payload.TransactionInput.Metadata = input.Metadata
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	require.NoError(t, validateTransactionCompletionPlan(payload))

	envelope := recoveryContractEnvelope(t, payload, result)
	ctx, _ := finalizationFixture(t)
	finalizer, store, metadata, _ := finalizationDependencies()

	require.NoError(t, completionError(finalizer.Complete(ctx, &envelope)))
	require.NoError(t, completionError(finalizer.Complete(ctx, &envelope)), "a recovery replay completes too")

	require.Len(t, store.records, 2)
	stored := metadata.data[constant.EntityTransaction+":"+payload.TransactionID.String()]
	require.NotNil(t, stored)
	assert.Equal(t, raw, stored.Data["feeExemption"])
}
