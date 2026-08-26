// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestCreateAssetExternalAccountCarriesNoHolder locks the implicit external account that
// asset creation opens: it is built as an mmodel.Account and handed straight to
// AccountRepo.Create, bypassing UseCase.CreateAccount, so it reaches NO holder seam on
// EITHER contract — no self-holder default, no requireHolder gate, no skip resolution.
// The row persists holder_id = NULL and holder_check_skipped = false.
//
// That is also what the seam itself would produce for this account: resolveHolderID
// returns nil for the external type. The two paths agree, so the asset route needs no
// RouteHolderPolicy of its own — and this test is the guard that keeps them agreeing if
// the asset path is ever refactored onto CreateAccount.
//
// Deliberate non-assertion: no SettingsReader or HolderReader is wired on the use case
// here. A holder seam reaching this path would nil-panic or fall back rather than fail an
// assertion, which is a louder signal than a boolean check.
func TestCreateAssetExternalAccountCarriesNoHolder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:   mockAssetRepo,
		AccountRepo: mockAccountRepo,
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockAssetRepo.EXPECT().
		FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "US Dollar", "USD").
		Return(false, nil).Times(1)

	mockAssetRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, a *mmodel.Asset) (*mmodel.Asset, error) {
			out := *a
			out.ID = uuid.New().String()

			return &out, nil
		}).Times(1)

	// No external account exists yet, so the implicit-create branch runs.
	mockAccountRepo.EXPECT().
		ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, nil).Times(1)

	var captured *mmodel.Account

	mockAccountRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			out.ID = uuid.New().String()
			captured = &out

			return &out, nil
		}).Times(1)

	mockBalanceRepo.EXPECT().
		ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).Times(1)

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			return b, nil
		}).Times(1)

	_, err := uc.CreateAsset(ctx, organizationID, ledgerID, &mmodel.CreateAssetInput{
		Name:   "US Dollar",
		Type:   "currency",
		Code:   "USD",
		Status: mmodel.Status{Code: "ACTIVE"},
	}, "Bearer test")
	require.NoError(t, err)

	require.NotNil(t, captured, "asset creation must open the implicit external account")

	// Guard against vacuity: assert we captured the EXTERNAL account, then that it is unowned.
	assert.Equal(t, constant.ExternalAccountType, captured.Type,
		"the captured account must be the implicit external account")
	assert.Nil(t, captured.HolderID,
		"the asset-created external account must persist holder_id = NULL")
	assert.False(t, captured.HolderCheckSkipped,
		"the asset-created external account records no honored holder skip")
}
