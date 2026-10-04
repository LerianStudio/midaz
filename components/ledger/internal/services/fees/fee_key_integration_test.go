//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A conforming key is stored, patched and read back exactly as the client sent it, and
// a case variant of it cannot add a second fee.
func TestIntegration_FeeKeyKeptVerbatimAcrossCreatePatchGet(t *testing.T) {
	svc, _ := newLivePackageUseCase(t)
	ctx := context.Background()

	resolver := feeshared.NewMockMidazResolver(gomock.NewController(t))
	resolver.EXPECT().AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), "@fees").Return(nil).AnyTimes()
	svc.resolver = resolver

	orgID, ledgerID := uuid.New(), uuid.New()

	create := &model.CreatePackageInput{
		FeeGroupLabel: "Pacote TED",
		MinAmount:     "100",
		MaxAmount:     "1000",
		Enable:        boolPtr(true),
		Fee: map[string]model.Fee{"tarifaTED2": {
			FeeLabel:         "Tarifa TED",
			CalculationModel: &model.CalculationModel{ApplicationRule: model.FlatFee, Calculations: []model.Calculation{{Type: model.Flat, Value: "2"}}},
			ReferenceAmount:  model.OriginalAmount,
			Priority:         1,
			IsDeductibleFrom: boolPtr(false),
			CreditAccount:    "@fees",
		}},
	}
	require.NoError(t, create.ValidateFees())

	created, err := svc.CreatePackage(ctx, create, orgID, ledgerID, uuid.Nil)
	require.NoError(t, err)

	require.NoError(t, svc.UpdatePackageByID(ctx, created.ID, orgID, ledgerID,
		&model.UpdatePackageInput{Fee: map[string]model.Fee{"tarifaTED2": {FeeLabel: "Tarifa TED 2"}}}))

	// A case variant of the stored key is that fee misspelled: refused, not added beside it.
	errVariant := svc.UpdatePackageByID(ctx, created.ID, orgID, ledgerID,
		&model.UpdatePackageInput{Fee: map[string]model.Fee{"tarifaTed2": create.Fee["tarifaTED2"]}})

	var fieldsErr pkg.ValidationKnownFieldsError
	require.ErrorAs(t, errVariant, &fieldsErr)
	require.Contains(t, fieldsErr.Fields["fees.tarifaTed2"], "stored key tarifaTED2")

	got, err := svc.GetPackageByID(ctx, created.ID, orgID, ledgerID)
	require.NoError(t, err)
	require.Len(t, got.Fees, 1, "fees: %v", got.Fees)
	require.Equal(t, "Tarifa TED 2", got.Fees["tarifaTED2"].FeeLabel, "fees: %v", got.Fees)
}
