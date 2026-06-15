// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// fakeFeeApplier records invocations and applies a scripted mutation/error so
// the seam's contract can be asserted without the real fee engine.
type fakeFeeApplier struct {
	calls   int
	lastOrg uuid.UUID
	mutate  func(cf *model.FeeCalculate)
	err     error
}

func (f *fakeFeeApplier) CalculateFee(_ context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) error {
	f.calls++
	f.lastOrg = organizationID

	if f.err != nil {
		return f.err
	}

	if f.mutate != nil {
		f.mutate(cf)
	}

	return nil
}

func baseTransaction() mtransaction.Transaction {
	return mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{AccountAlias: "@payer"}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{AccountAlias: "@payee"}},
			},
		},
	}
}

func TestApplyFees_NoOpOnRevert(t *testing.T) {
	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		cf.Transaction.Send.Value = decimal.NewFromInt(999) // would corrupt if ever run
	}}
	handler := &TransactionHandler{FeeApplier: applier}

	input := baseTransaction()
	orgID, ledgerID := uuid.New(), uuid.New()

	err := handler.applyFees(context.Background(), &input, orgID, ledgerID, true /* isRevert */, false /* isAnnotation */, false /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.Equal(t, 0, applier.calls, "fee engine must not run on the revert path (no re-charge)")
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "revert input must be untouched")
}

func TestApplyFees_NoOpOnAnnotation(t *testing.T) {
	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		cf.Transaction.Send.Value = decimal.NewFromInt(999) // would corrupt if ever run
	}}
	handler := &TransactionHandler{FeeApplier: applier}

	input := baseTransaction()
	orgID, ledgerID := uuid.New(), uuid.New()

	err := handler.applyFees(context.Background(), &input, orgID, ledgerID, false /* isRevert */, true /* isAnnotation */, false /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.Equal(t, 0, applier.calls, "fee engine must not run on the annotation path (NOTED is one-sided, no fee)")
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "annotation input must be untouched")
}

func TestApplyFees_NoOpWhenApplierNil(t *testing.T) {
	handler := &TransactionHandler{FeeApplier: nil}

	input := baseTransaction()

	err := handler.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false, false /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)))
}

func TestApplyFees_NoOpWhenSkipHonored(t *testing.T) {
	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		cf.Transaction.Send.Value = decimal.NewFromInt(999) // would corrupt if ever run
	}}
	handler := &TransactionHandler{FeeApplier: applier}

	input := baseTransaction()

	err := handler.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false, true /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.Equal(t, 0, applier.calls,
		"an honored fee skip must bypass the engine entirely — zero CalculateFee, hence zero FindByOrganizationIDAndLedgerID package lookup")
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "honored fee skip must leave the transaction unmutated")
}

func TestApplyFees_SkipHonoredTouchesNoFeeDependency(t *testing.T) {
	// nil FeeApplier AND nil FeesMongoManager: an honored skip must return before
	// reaching either, proving the bypass touches no fee dependency (no engine
	// call, no tenant Mongo resolution). A non-skip path would nil-deref the
	// applier or fall through to resolution; this returns nil cleanly.
	handler := &TransactionHandler{FeeApplier: nil, FeesMongoManager: nil, MultiTenantEnabled: true}

	input := baseTransaction()

	err := handler.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false, true /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "honored fee skip must leave the transaction unmutated")
}

func TestApplyFees_FoldsMutatedSendBack(t *testing.T) {
	ledgerID := uuid.New()
	orgID := uuid.New()

	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		// Engine moves Send.Value (deductible fee) and appends a fee leg.
		require.Equal(t, ledgerID, cf.LedgerID, "applyFees must pass the ledger ID into the envelope")
		cf.Transaction.Send.Value = decimal.NewFromInt(950)
		cf.Transaction.Send.Distribute.To = append(cf.Transaction.Send.Distribute.To,
			mtransaction.FromTo{AccountAlias: "@fee_collector"})
		if cf.Transaction.Metadata == nil {
			cf.Transaction.Metadata = map[string]any{}
		}
		cf.Transaction.Metadata["packageAppliedID"] = "pkg-1"
	}}
	handler := &TransactionHandler{FeeApplier: applier}

	input := baseTransaction()

	err := handler.applyFees(context.Background(), &input, orgID, ledgerID, false, false, false /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.Equal(t, 1, applier.calls)
	assert.Equal(t, orgID, applier.lastOrg)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(950)), "moved Send.Value must fold back into the caller's input")
	require.Len(t, input.Send.Distribute.To, 2, "appended fee leg must fold back into the caller's input")
	assert.Equal(t, "@fee_collector", input.Send.Distribute.To[1].AccountAlias)
	assert.Equal(t, "pkg-1", input.Metadata["packageAppliedID"])
}

func TestApplyFees_PropagatesBusinessError(t *testing.T) {
	applier := &fakeFeeApplier{err: pkg.ValidationError{
		Code:    "0199",
		Title:   "Package amount range overlap",
		Message: "transaction value is outside the package range",
	}}
	handler := &TransactionHandler{FeeApplier: applier}

	input := baseTransaction()

	err := handler.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false, false /* honoredFeeSkip */)

	require.Error(t, err)

	var businessErr pkg.ValidationError

	require.True(t, errors.As(err, &businessErr),
		"the fee engine returns a canonical pkg business error that must reach http.WithError unchanged so it surfaces as a 4xx, not a 500")
	assert.Equal(t, "0199", businessErr.Code)
	assert.Equal(t, "transaction value is outside the package range", businessErr.Message)
}
