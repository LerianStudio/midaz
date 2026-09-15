// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

// storedDeductibleFlatFee is the package as it sits in storage: one deductible fee
// charged as a flat amount, which a payment at the package minimum must be able to
// cover.
func storedDeductibleFlatFee(value string) map[string]model.Fee {
	return map[string]model.Fee{
		"fee1": {
			FeeLabel:         "Taxa",
			ReferenceAmount:  "originalAmount",
			Priority:         1,
			CreditAccount:    "fee_account",
			IsDeductibleFrom: boolPtr(true),
			CalculationModel: &model.CalculationModel{
				ApplicationRule: "flatFee",
				Calculations:    []model.Calculation{{Type: "flat", Value: value}},
			},
		},
	}
}

// patchedCalculations is the smallest patch that restates a stored fee's amount: no
// credit account, so the update touches no other service.
func patchedCalculations(value string) map[string]model.Fee {
	return map[string]model.Fee{
		"fee1": {
			CalculationModel: &model.CalculationModel{
				Calculations: []model.Calculation{{Type: "flat", Value: value}},
			},
		},
	}
}

// Lowering the minimum under a deductible fee the package already carries would leave
// the package accepting payments it cannot charge the fee on, so the update is
// refused before anything is written.
func TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackageRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)

	amountData := &model.AmountData{
		MinAmount: decimal.NewFromInt(100),
		MaxAmount: decimal.NewFromInt(1000),
		Fees:      storedDeductibleFlatFee("25"),
		LedgerID:  uuid.New(),
	}

	mockPackageRepo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(amountData, nil)
	mockPackageRepo.EXPECT().
		FindList(gomock.Any(), gomock.Any()).
		Return([]*pack.Package{}, nil).
		AnyTimes()

	svc := &UseCase{packageRepo: mockPackageRepo, resolver: mockResolver}

	newMinimum := "1"
	input := &model.UpdatePackageInput{MinAmount: &newMinimum}

	err := svc.UpdatePackageByID(context.Background(), uuid.New(), uuid.New(), uuid.Nil, input)

	require.ErrorContains(t, err, constant.ErrCalculationValueFlatFee.Error())
}

// Dropping the minimum and the fee that stood in its way is one legitimate edit, so
// the update applies it instead of refusing the pair. A client that serialises the
// calculation model as an object writes the removal the second way, and the package
// lands in the same state.
func TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		patch map[string]model.Fee
	}{
		{name: "the entry carries no field", patch: map[string]model.Fee{"fee1": {}}},
		{
			name:  "the entry carries an empty calculation model",
			patch: map[string]model.Fee{"fee1": {CalculationModel: &model.CalculationModel{}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPackageRepo := pack.NewMockRepository(ctrl)
			mockResolver := feeshared.NewMockMidazResolver(ctrl)

			ledgerID := uuid.New()
			packageID := uuid.New()

			amountData := &model.AmountData{
				MinAmount: decimal.NewFromInt(100),
				MaxAmount: decimal.NewFromInt(1000),
				Fees:      storedDeductibleFlatFee("25"),
				LedgerID:  ledgerID,
			}

			mockPackageRepo.EXPECT().
				FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(amountData, nil)
			mockPackageRepo.EXPECT().
				FindList(gomock.Any(), gomock.Any()).
				Return([]*pack.Package{}, nil).
				AnyTimes()

			var written bson.M

			mockPackageRepo.EXPECT().
				Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, updateFields *bson.M) (*pack.Package, error) {
					written = *updateFields

					return &pack.Package{ID: packageID, LedgerID: ledgerID}, nil
				})

			svc := &UseCase{packageRepo: mockPackageRepo, resolver: mockResolver}

			newMinimum := "1"
			input := &model.UpdatePackageInput{MinAmount: &newMinimum, Fee: tt.patch}

			require.NoError(t, svc.UpdatePackageByID(context.Background(), packageID, uuid.New(), uuid.Nil, input))

			// The end state is what makes the acceptance safe: the package lands on
			// the lower minimum with the fee that exceeded it deleted, not kept.
			require.Equal(t, newMinimum, *written["$set"].(bson.M)["minimum_amount"].(*string))
			require.Contains(t, written["$unset"], "fees.fee1")
		})
	}
}

// A fee the patch restates is measured against the minimum the package will carry,
// not the one it carried before, in both directions.
func TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		newMinimum  string
		patchedFee  string
		wantErrCode string
	}{
		{
			name:        "minimum lowered under the fee the patch sets",
			newMinimum:  "1",
			patchedFee:  "50",
			wantErrCode: constant.ErrDeductibleCalculationValueFlatFee.Error(),
		},
		{
			name:       "minimum raised above the fee the patch sets",
			newMinimum: "900",
			patchedFee: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPackageRepo := pack.NewMockRepository(ctrl)
			mockResolver := feeshared.NewMockMidazResolver(ctrl)

			ledgerID := uuid.New()
			packageID := uuid.New()

			amountData := &model.AmountData{
				MinAmount: decimal.NewFromInt(100),
				MaxAmount: decimal.NewFromInt(1000),
				Fees:      storedDeductibleFlatFee("10"),
				LedgerID:  ledgerID,
			}

			mockPackageRepo.EXPECT().
				FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(amountData, nil)
			mockPackageRepo.EXPECT().
				FindList(gomock.Any(), gomock.Any()).
				Return([]*pack.Package{}, nil).
				AnyTimes()
			mockPackageRepo.EXPECT().
				Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
				Return(&pack.Package{ID: packageID, LedgerID: ledgerID}, nil).
				AnyTimes()

			svc := &UseCase{packageRepo: mockPackageRepo, resolver: mockResolver}

			input := &model.UpdatePackageInput{
				MinAmount: &tt.newMinimum,
				Fee:       patchedCalculations(tt.patchedFee),
			}

			err := svc.UpdatePackageByID(context.Background(), packageID, uuid.New(), uuid.Nil, input)

			if tt.wantErrCode == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.wantErrCode)
		})
	}
}
