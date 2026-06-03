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

	feeError "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
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

	err := handler.applyFees(context.Background(), &input, orgID, ledgerID, true /* isRevert */, false /* isAnnotation */)

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

	err := handler.applyFees(context.Background(), &input, orgID, ledgerID, false /* isRevert */, true /* isAnnotation */)

	require.NoError(t, err)
	assert.Equal(t, 0, applier.calls, "fee engine must not run on the annotation path (NOTED is one-sided, no fee)")
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "annotation input must be untouched")
}

func TestApplyFees_NoOpWhenApplierNil(t *testing.T) {
	handler := &TransactionHandler{FeeApplier: nil}

	input := baseTransaction()

	err := handler.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false)

	require.NoError(t, err)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)))
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

	err := handler.applyFees(context.Background(), &input, orgID, ledgerID, false, false)

	require.NoError(t, err)
	assert.Equal(t, 1, applier.calls)
	assert.Equal(t, orgID, applier.lastOrg)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(950)), "moved Send.Value must fold back into the caller's input")
	require.Len(t, input.Send.Distribute.To, 2, "appended fee leg must fold back into the caller's input")
	assert.Equal(t, "@fee_collector", input.Send.Distribute.To[1].AccountAlias)
	assert.Equal(t, "pkg-1", input.Metadata["packageAppliedID"])
}

func TestApplyFees_TranslatesBusinessError(t *testing.T) {
	applier := &fakeFeeApplier{err: feeError.ValidationError{
		Code:    "FEE-0001",
		Title:   "Fee out of range",
		Message: "transaction value is outside the package range",
	}}
	handler := &TransactionHandler{FeeApplier: applier}

	input := baseTransaction()

	err := handler.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false)

	require.Error(t, err)

	var translated pkg.ValidationError

	require.True(t, errors.As(err, &translated),
		"a feeshared.ValidationError must be translated to a ledger pkg.ValidationError so http.WithError returns a 4xx, not a 500")
	assert.Equal(t, "FEE-0001", translated.Code)
	assert.Equal(t, "transaction value is outside the package range", translated.Message)
}

func TestTranslateFeeError(t *testing.T) {
	t.Run("ValidationError", func(t *testing.T) {
		got := translateFeeError(feeError.ValidationError{Code: "C", Message: "m"})

		var want pkg.ValidationError
		require.True(t, errors.As(got, &want))
		assert.Equal(t, "C", want.Code)
	})

	t.Run("UnprocessableOperationError", func(t *testing.T) {
		got := translateFeeError(feeError.UnprocessableOperationError{Code: "C", Message: "m"})

		var want pkg.UnprocessableOperationError
		require.True(t, errors.As(got, &want))
		assert.Equal(t, "C", want.Code)
	})

	t.Run("EntityNotFoundError", func(t *testing.T) {
		got := translateFeeError(feeError.EntityNotFoundError{Code: "C", Message: "m"})

		var want pkg.EntityNotFoundError
		require.True(t, errors.As(got, &want))
		assert.Equal(t, "C", want.Code)
	})

	t.Run("unknown error wraps to internal", func(t *testing.T) {
		got := translateFeeError(errors.New("boom"))

		// ValidateInternalError returns a pkg.InternalServerError so the HTTP
		// layer returns 500 without leaking fee-internal detail.
		var want pkg.InternalServerError
		assert.True(t, errors.As(got, &want), "unknown fee error must wrap to an internal server error")
	})
}
