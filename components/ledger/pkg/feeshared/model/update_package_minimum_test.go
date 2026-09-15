// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"context"
	"testing"

	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

// A patch that moves the minimum must leave every fee the package keeps within it.
// The fees the patch restates are validated as they are applied, against this same
// new minimum, so they are skipped here.
func TestUpdatePackageInputValidateStoredFeesAgainstMinimum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newMinimum *string
		storedFees map[string]Fee
		patch      map[string]Fee
		wantCode   string
	}{
		{
			name:       "minimum lowered below a stored deductible flat fee",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			wantCode:   constant.ErrCalculationValueFlatFee.Error(),
		},
		{
			name:       "minimum lowered to exactly the stored deductible flat fee",
			newMinimum: stringPtr("25"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
		},
		{
			name:       "stored flat fee is not deductible",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", false)},
		},
		{
			name:       "patch restates the calculations of the offending fee",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": deductibleFee(Flat, "1", true)},
		},
		{
			name:       "patch names the fee but changes only its label",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": {FeeLabel: "Novo rotulo"}},
			wantCode:   constant.ErrCalculationValueFlatFee.Error(),
		},
		{
			name:       "patch removes the offending fee in the same call",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": {}},
		},
		{
			name:       "patch removes the offending fee with an empty calculation model",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": {CalculationModel: &CalculationModel{}}},
		},
		{
			name:       "patch stops the offending fee being deducted from the payment",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": {IsDeductibleFrom: boolPtr(false)}},
		},
		{
			name:       "patch confirms the fee stays deducted from the payment",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": {IsDeductibleFrom: boolPtr(true)}},
			wantCode:   constant.ErrCalculationValueFlatFee.Error(),
		},
		{
			name:       "patch sets only a route on the offending fee, which keeps it",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"fee1": {RouteFrom: stringPtr("taxa_debito")}},
			wantCode:   constant.ErrCalculationValueFlatFee.Error(),
		},
		{
			name:       "patch key differs in case from the stored key",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"feeOne": deductibleFee(Flat, "25", true)},
			patch:      map[string]Fee{"FeeOne": deductibleFee(Flat, "1", true)},
		},
		{
			name:       "patch carries no minimum",
			storedFees: map[string]Fee{"fee1": deductibleFee(Flat, "25", true)},
		},
		{
			name:       "stored deductible percentage above 100",
			newMinimum: stringPtr("50"),
			storedFees: map[string]Fee{"fee1": deductibleFee(Percentage, "150", true)},
			wantCode:   constant.ErrCalculationValuePercentage.Error(),
		},
		{
			name:       "stored fee without a calculation model",
			newMinimum: stringPtr("1"),
			storedFees: map[string]Fee{"fee1": {FeeLabel: "Taxa", IsDeductibleFrom: boolPtr(true)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			up := &UpdatePackageInput{MinAmount: tt.newMinimum, Fee: tt.patch}

			err := up.ValidateStoredFeesAgainstMinimum(tt.storedFees)

			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.wantCode)
		})
	}
}

// The check that skips a fee its own patch settles and the write that applies that
// patch must agree on which entries delete the fee. They read one predicate, and
// this pins the agreement over every field the write looks at, including the two
// shapes that read as empty only to the writer: an empty calculation model, and the
// string "null".
func TestFeeRemovalPredicateAgreesWithTheApplyPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patch    Fee
		survives bool
	}{
		{name: "an entry with no field at all"},
		{name: "an entry whose calculation model carries nothing", patch: Fee{CalculationModel: &CalculationModel{}}},
		{name: "an entry whose only field the writer reads as empty", patch: Fee{FeeLabel: "null"}},
		{name: "an entry renaming the fee", patch: Fee{FeeLabel: "Novo rotulo"}, survives: true},
		{name: "an entry setting the route it debits", patch: Fee{RouteFrom: stringPtr("taxa_debito")}, survives: true},
		{name: "an entry setting the route it credits", patch: Fee{RouteTo: stringPtr("taxa_credito")}, survives: true},
		{name: "an entry setting a priority", patch: Fee{Priority: 3}, survives: true},
		{name: "an entry setting a reference amount", patch: Fee{ReferenceAmount: OriginalAmount}, survives: true},
		{name: "an entry stopping the deduction", patch: Fee{IsDeductibleFrom: boolPtr(false)}, survives: true},
		{name: "an entry setting a credit account", patch: Fee{CreditAccount: "fee_account"}, survives: true},
		{
			name:     "an entry restating the application rule",
			patch:    Fee{CalculationModel: &CalculationModel{ApplicationRule: FlatFee}},
			survives: true,
		},
		{
			name:     "an entry restating the calculations",
			patch:    Fee{CalculationModel: &CalculationModel{Calculations: []Calculation{{Type: Flat, Value: "1"}}}},
			survives: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			resolver := feeshared.NewMockMidazResolver(ctrl)
			resolver.EXPECT().
				AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil).
				AnyTimes()

			stored := map[string]Fee{"fee1": deductibleFee(Flat, "25", true)}
			patch := tt.patch

			survives, err := patch.SetAndValidateHasFieldsToUpdate(
				context.Background(), patch.IsDeductibleFrom, decimal.NewFromInt(1),
				stored, "fee1", uuid.New(), uuid.New(), bson.M{}, resolver,
			)

			require.NoError(t, err)
			require.Equal(t, tt.survives, survives, "the write kept the fee but the predicate calls it a removal")
			require.Equal(t, !tt.survives, patch.removesTheFee(), "the predicate disagrees with the write")
		})
	}
}

// Two patch entries whose keys fold to the same fee leave no way to tell which one
// the operator meant, and Go map order would otherwise pick one, so the request is
// refused and the refusal names the key they collide on.
func TestUpdatePackageInputRefusesAmbiguousFeeKeys(t *testing.T) {
	t.Parallel()

	ambiguous := func() *UpdatePackageInput {
		return &UpdatePackageInput{
			MinAmount: stringPtr("1"),
			Fee: map[string]Fee{
				"fee1":  deductibleFee(Flat, "1", true),
				"Fee_1": {FeeLabel: "Novo rotulo"},
			},
		}
	}

	stored := map[string]Fee{"fee1": deductibleFee(Flat, "25", true)}

	t.Run("at the request boundary", func(t *testing.T) {
		t.Parallel()

		err := ambiguous().ValidateFees()

		require.ErrorContains(t, err, constant.ErrDuplicateFeeKey.Error())
		require.ErrorContains(t, err, "fee1")
	})

	t.Run("when the stored fees are measured against the new minimum", func(t *testing.T) {
		t.Parallel()

		err := ambiguous().ValidateStoredFeesAgainstMinimum(stored)

		require.ErrorContains(t, err, constant.ErrDuplicateFeeKey.Error())
		require.ErrorContains(t, err, "fee1")
	})

	t.Run("a patch carrying no minimum is refused just the same", func(t *testing.T) {
		t.Parallel()

		up := ambiguous()
		up.MinAmount = nil

		require.ErrorContains(t, up.ValidateStoredFeesAgainstMinimum(stored), constant.ErrDuplicateFeeKey.Error())
	})

	// The defect this closes answered the same body two ways across runs, so one
	// call proves nothing: only a repeat can tell a refusal from a coin toss.
	t.Run("every call answers the same way", func(t *testing.T) {
		t.Parallel()

		for range 200 {
			require.ErrorContains(t, ambiguous().ValidateStoredFeesAgainstMinimum(stored), constant.ErrDuplicateFeeKey.Error())
		}
	})
}

// When more than one stored fee breaks the new minimum, the one the operator is told
// about is the same on every call, so a retry does not move the diagnostic to a
// different fee.
func TestUpdatePackageInputValidateStoredFeesAgainstMinimumNamesOneFee(t *testing.T) {
	t.Parallel()

	stored := map[string]Fee{
		"feeB": deductibleFee(Flat, "25", true),
		"feeA": deductibleFee(Flat, "30", true),
	}

	for range 200 {
		up := &UpdatePackageInput{MinAmount: stringPtr("1")}

		err := up.ValidateStoredFeesAgainstMinimum(stored)

		require.ErrorContains(t, err, constant.ErrCalculationValueFlatFee.Error())
		require.ErrorContains(t, err, "feeA")
	}
}

// The minimum a patched fee is measured against is the one the package will carry.
func TestUpdatePackageInputEffectiveMinimumAmount(t *testing.T) {
	t.Parallel()

	stored := decimal.NewFromInt(100)

	t.Run("patch carries a minimum", func(t *testing.T) {
		t.Parallel()

		up := &UpdatePackageInput{MinAmount: stringPtr("900")}

		effective, err := up.EffectiveMinimumAmount(stored)

		require.NoError(t, err)
		require.True(t, effective.Equal(decimal.NewFromInt(900)), "got %s", effective)
	})

	t.Run("patch carries no minimum", func(t *testing.T) {
		t.Parallel()

		up := &UpdatePackageInput{}

		effective, err := up.EffectiveMinimumAmount(stored)

		require.NoError(t, err)
		require.True(t, effective.Equal(stored), "got %s", effective)
	})

	t.Run("patch carries an unparseable minimum", func(t *testing.T) {
		t.Parallel()

		up := &UpdatePackageInput{MinAmount: stringPtr("100,00")}

		_, err := up.EffectiveMinimumAmount(stored)

		require.ErrorContains(t, err, constant.ErrConvertToDecimal.Error())
	})
}
