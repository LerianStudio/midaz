// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	mongoPack "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/bsondecimal"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	http "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

func TestUpdatePackage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	flagTrue := true

	mockPackageRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)

	orgId := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
	enableFlag := true
	calculation := make([]model.Calculation, 1)
	calculation[0] = model.Calculation{
		Type:  "percentage",
		Value: "12",
	}

	feeRemove := make(map[string]model.Fee)
	feeRemove["fees"] = model.Fee{}

	calculationEntity := make([]mongoPack.Calculation, 1)
	calculationEntity[0] = mongoPack.Calculation{
		Type:  "percentage",
		Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(160)},
	}

	feeEntity := make(map[string]mongoPack.Fee)
	feeEntity["teste"] = mongoPack.Fee{
		FeeLabel: "atualizado",
		CalculationModel: mongoPack.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    calculationEntity,
		},
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: &flagTrue,
		CreditAccount:    "account",
	}

	fee := make(map[string]model.Fee)
	fee["fees"] = model.Fee{
		FeeLabel: "atualizado",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    calculation,
		},
		ReferenceAmount:  "originalAmount",
		Priority:         3,
		IsDeductibleFrom: &flagTrue,
		CreditAccount:    "account",
	}

	feeAmountData := make(map[string]model.Fee)
	feeAmountData["fees"] = model.Fee{
		FeeLabel: "atualizado",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    calculation,
		},
		ReferenceAmount:  "originalAmount",
		Priority:         2,
		IsDeductibleFrom: &flagTrue,
		CreditAccount:    "account",
	}

	amountData := &model.AmountData{
		MinAmount: decimal.NewFromInt(100),
		MaxAmount: decimal.NewFromInt(1000),
		Fees:      feeAmountData,
		LedgerID:  ledgerID,
		SegmentID: nil,
	}

	minAmount := "900"
	maxAmount := "1000"
	packToUpdate := &model.UpdatePackageInput{
		FeeGroupLabel:  "atualiza",
		Description:    "atualiza description",
		MinAmount:      &minAmount,
		MaxAmount:      &maxAmount,
		WaivedAccounts: &[]string{"acc01", "acc02"},
		Fee:            fee,
		EnablePackage:  &flagTrue,
	}

	packSvc := &UseCase{
		packageRepo: mockPackageRepo,
		resolver:    mockResolver,
	}

	filter := http.QueryHeader{
		OrganizationID: orgId,
		LedgerID:       ledgerID,
	}

	packEntity := []*mongoPack.Package{
		{
			ID:             packID,
			FeeGroupLabel:  "teste group label",
			Description:    nil,
			SegmentID:      nil,
			LedgerID:       ledgerID,
			MinimumAmount:  decimal.NewFromInt(100),
			MaximumAmount:  decimal.NewFromInt(1000),
			WaivedAccounts: &[]string{"acc01", "acc02"},
			Enable:         &enableFlag,
		},
	}

	updatedPkg := &mongoPack.Package{
		ID:            packID,
		FeeGroupLabel: "teste group label",
		LedgerID:      ledgerID,
		Enable:        &enableFlag,
	}

	tests := []struct {
		name        string
		packId      uuid.UUID
		orgId       uuid.UUID
		filter      http.QueryHeader
		packInput   *model.UpdatePackageInput
		mockSetup   func()
		expectErr   bool
		errContains string
	}{
		{
			name:      "Success - Update package by id",
			packId:    packID,
			packInput: &model.UpdatePackageInput{Fee: feeRemove},
			mockSetup: func() {
				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
					Return(updatedPkg, nil)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr: false,
		},
		{
			name:      "Success - Update package by id ",
			packId:    packID,
			orgId:     orgId,
			filter:    filter,
			packInput: packToUpdate,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packEntity, nil)

				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
					Return(updatedPkg, nil)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr: false,
		},
		{
			name:      "Error - Update package by id not found",
			packId:    packID,
			orgId:     orgId,
			filter:    filter,
			packInput: packToUpdate,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packEntity, nil)

				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
					Return(nil, ErrDatabaseItemNotFound)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr:   true,
			errContains: "No entity was found",
		},
		{
			name:      "Error - Update package by id",
			packId:    packID,
			orgId:     orgId,
			filter:    filter,
			packInput: packToUpdate,
			mockSetup: func() {
				mockResolver.EXPECT().
					AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockPackageRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(packEntity, nil)

				mockPackageRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
					Return(nil, constant.ErrBadRequest)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr:   true,
			errContains: "0047",
		},
		{
			name:      "Error - No fields to update package by id",
			packId:    uuid.New(),
			orgId:     orgId,
			packInput: &model.UpdatePackageInput{},
			mockSetup: func() {
				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(amountData, nil)
			},
			expectErr:   true,
			errContains: "0183",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			err := packSvc.UpdatePackageByID(ctx, tt.packId, tt.orgId, uuid.Nil, tt.packInput)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdatePackageByID_UpdatedAtFieldSet(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackageRepo := pack.NewMockRepository(ctrl)
	mockResolver := feeshared.NewMockMidazResolver(ctrl)

	orgId := uuid.New()
	packID := uuid.New()
	feeGroupLabel := "new label"
	amountData := &model.AmountData{
		MinAmount: decimal.NewFromInt(100),
		MaxAmount: decimal.NewFromInt(1000),
		Fees:      map[string]model.Fee{},
		LedgerID:  uuid.New(),
		SegmentID: nil,
	}

	packSvc := &UseCase{
		packageRepo: mockPackageRepo,
		resolver:    mockResolver,
	}

	input := &model.UpdatePackageInput{
		FeeGroupLabel: feeGroupLabel,
	}

	var capturedUpdateFields interface{}

	mockPackageRepo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(amountData, nil)

	mockPackageRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
		DoAndReturn(func(_ context.Context, id, _, _ uuid.UUID, updateFields interface{}) (*pack.Package, error) {
			capturedUpdateFields = updateFields
			return &pack.Package{ID: id, LedgerID: amountData.LedgerID}, nil
		})

	err := packSvc.UpdatePackageByID(context.Background(), packID, orgId, uuid.Nil, input)
	assert.NoError(t, err)

	// Assert that updated_at is set in the updateFields
	updateMap, ok := capturedUpdateFields.(*bson.M)
	if !ok {
		updateMap2, ok2 := capturedUpdateFields.(bson.M)
		if !ok2 {
			t.Fatalf("updateFields is not a bson.M: %T", capturedUpdateFields)
		}
		updateMap = &updateMap2
	}
	setFields, ok := (*updateMap)["$set"].(bson.M)
	assert.True(t, ok, "expected $set in updateFields")
	_, hasUpdatedAt := setFields["updated_at"]
	assert.True(t, hasUpdatedAt, "expected updated_at in $set fields")
}

// TestUpdatePackageByID_EmitsFeesPackageUpdated asserts a successful update emits
// the fee_packages.updated event, built from the entity returned by Update.
func TestUpdatePackageByID_EmitsFeesPackageUpdated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()
	enable := true

	amountData := &model.AmountData{
		MinAmount: decimal.NewFromInt(100),
		MaxAmount: decimal.NewFromInt(1000),
		Fees:      map[string]model.Fee{},
		LedgerID:  ledgerID,
	}

	persisted := &pack.Package{
		ID:            packID,
		FeeGroupLabel: "updated",
		LedgerID:      ledgerID,
		Enable:        &enable,
	}

	mockPackRepo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(amountData, nil)
	mockPackRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(uuid.Nil), gomock.Any()).
		Return(persisted, nil)

	svc := &UseCase{
		packageRepo: mockPackRepo,
		Streaming:   mockEmitter,
	}

	newLabel := "updated"
	input := &model.UpdatePackageInput{FeeGroupLabel: newLabel}

	err := svc.UpdatePackageByID(context.Background(), packID, orgID, uuid.Nil, input)
	require.NoError(t, err)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "fee_packages", "updated")

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)
	req := emitted[0]
	assert.Equal(t, packID.String(), req.Subject)

	payload := unmarshalPayload(t, req.Payload)
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
}

// TestSetAmountsDataToUpdate_KeepsStoredSelector pins that a band update on a
// selector-scoped package is checked within its own scope: an unscoped package
// on the same band is a different scope and must not collide with it.
func TestSetAmountsDataToUpdate_KeepsStoredSelector(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockPackRepo := pack.NewMockRepository(ctrl)
	uc := &UseCase{packageRepo: mockPackRepo}

	packageID := uuid.New()
	ledgerID := uuid.New()
	minAmount, maxAmount := "100", "1000"

	unscopedSibling := &pack.Package{
		ID:            uuid.New(),
		LedgerID:      ledgerID,
		MinimumAmount: decimal.NewFromInt(100),
		MaximumAmount: decimal.NewFromInt(1000),
	}

	mockPackRepo.EXPECT().
		FindList(gomock.Any(), gomock.Any()).
		Return([]*pack.Package{unscopedSibling}, nil)

	stored := &model.AmountData{
		MinAmount:        decimal.NewFromInt(10),
		MaxAmount:        decimal.NewFromInt(50),
		LedgerID:         ledgerID,
		MetadataSelector: map[string]string{"fee_context": "ted_salario"},
	}

	err := uc.SetAmountsDataToUpdate(context.Background(), nil,
		&model.UpdatePackageInput{MinAmount: &minAmount, MaxAmount: &maxAmount},
		stored, uuid.New(), &packageID, bson.M{})
	require.NoError(t, err, "a selector-scoped package must not collide with an unscoped one on its band")
}

// A patch entry that leaves priority out keeps the stored one, so two stored fees
// patched without priority do not collide with each other.
func TestValidationFeesSetUnset_PriorityComesFromStoreWhenOmitted(t *testing.T) {
	t.Parallel()

	existing := map[string]model.Fee{"feeA": storedFee(1), "feeB": storedFee(2)}

	tests := []struct {
		name    string
		patch   map[string]model.Fee
		wantErr error
	}{
		{"two stored fees relabelled without priority", map[string]model.Fee{"feeA": {FeeLabel: "a"}, "feeB": {FeeLabel: "b"}}, nil},
		{"patched priority taken by another stored fee", map[string]model.Fee{"feeA": {Priority: 2}}, constant.ErrPriorityInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := (&UseCase{}).validationFeesSetUnset(context.Background(), decimal.NewFromInt(100), uuid.New(), uuid.New(),
				existing, tt.patch, bson.M{}, bson.M{})

			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tt.wantErr.Error())
		})
	}
}

// A fee added under a key the converter normalizes is stored whole under that
// normalized key: the key is normalized once, so the lookup lands on the value.
func TestValidationFeesSetUnset_NewFeeStoredUnderNormalizedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	resolver := feeshared.NewMockMidazResolver(ctrl)
	resolver.EXPECT().AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), "@fees").Return(nil)

	patch := map[string]model.Fee{"_tarifa": {
		FeeLabel:         "Tarifa TED",
		CalculationModel: &model.CalculationModel{ApplicationRule: "flatFee", Calculations: []model.Calculation{{Type: model.Flat, Value: "2"}}},
		ReferenceAmount:  "originalAmount",
		Priority:         2,
		IsDeductibleFrom: boolPtr(false),
		CreditAccount:    "@fees",
	}}
	setFields := bson.M{}

	err := (&UseCase{resolver: resolver}).validationFeesSetUnset(context.Background(), decimal.NewFromInt(100), uuid.New(), uuid.New(),
		map[string]model.Fee{"feeA": storedFee(1)}, patch, setFields, bson.M{})
	require.NoError(t, err)

	stored, ok := setFields["fees.Tarifa"].(mongoPack.Fee)
	require.True(t, ok, "fee must be set under its normalized key, got %v", setFields)
	assert.Equal(t, "Tarifa TED", stored.FeeLabel)
	assert.Equal(t, 2, stored.Priority)
	assert.Equal(t, "originalAmount", stored.ReferenceAmount)
	assert.Equal(t, "@fees", stored.CreditAccount)
	assert.Equal(t, "flatFee", stored.CalculationModel.ApplicationRule)
	require.Len(t, stored.CalculationModel.Calculations, 1)
	assert.Equal(t, model.Flat, stored.CalculationModel.Calculations[0].Type)
	assert.True(t, stored.CalculationModel.Calculations[0].Value.Equal(decimal.NewFromInt(2)))
}

func storedFee(priority int) model.Fee {
	return model.Fee{
		FeeLabel:         "stored",
		CalculationModel: &model.CalculationModel{ApplicationRule: "flatFee", Calculations: []model.Calculation{{Type: "flat", Value: "1"}}},
		ReferenceAmount:  "originalAmount",
		Priority:         priority,
		IsDeductibleFrom: boolPtr(false),
		CreditAccount:    "account",
	}
}

func boolPtr(b bool) *bool {
	return &b
}
