package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestDeleteAssetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:   mockAssetRepo,
		AccountRepo: mockAccountRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		assetID        uuid.UUID
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Delete asset and external account",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{{ID: uuid.New().String()}}, nil)
				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				mockAssetRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Asset not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to list external accounts",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			assetID:        uuid.New(),
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("error listing accounts"))
			},
			expectErr: true,
		},
		// {
		// 	name:           "Error - Failure to delete asset",
		// 	organizationID: uuid.New(),
		// 	ledgerID:       uuid.New(),
		// 	assetID:        uuid.New(),
		// 	mockSetup: func() {
		// 		mockAssetRepo.EXPECT().
		// 			Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		// 			Return(&mmodel.Asset{ID: uuid.New().String(), Code: "asset123"}, nil)
		// 		mockAccountRepo.EXPECT().
		// 			ListAccountsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		// 			Return([]mmodel.Account{}, nil)
		// 		mockAssetRepo.EXPECT().
		// 			Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		// 			Return(fmt.Errorf("error deleting asset"))
		// 	},
		// 	expectErr: true,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			err := uc.DeleteAssetByID(ctx, tt.organizationID, tt.ledgerID, tt.assetID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// func TestDeleteAssetByID(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()
//
// 	// Mocks
// 	mockAssetRepo := asset.NewMockRepository(ctrl)
// 	mockAccountRepo := account.NewMockRepository(ctrl)
//
// 	uc := &UseCase{
// 		AssetRepo:   mockAssetRepo,
// 		AccountRepo: mockAccountRepo,
// 	}
//
// 	ctx := context.Background()
// 	organizationID := uuid.New()
// 	ledgerID := uuid.New()
// 	assetID := uuid.New()
//
// 	tests := []struct {
// 		name        string
// 		setupMocks  func()
// 		expectedErr error
// 	}{
// 		{
// 			name: "success - asset deleted with external account",
// 			setupMocks: func() {
// 				// Simula encontrar o asset
// 				mockAssetRepo.EXPECT().
// 					Find(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(&mmodel.Asset{ID: assetID.String(), Code: "USD"}, nil).
// 					Times(1)
//
// 				// Simula encontrar conta externa associada
// 				mockAccountRepo.EXPECT().
// 					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"EXT-USD"}).
// 					Return([]mmodel.Account{
// 						{ID: uuid.New().String()},
// 					}, nil).
// 					Times(1)
//
// 				// Simula exclusão da conta externa
// 				mockAccountRepo.EXPECT().
// 					Delete(gomock.Any(), organizationID, ledgerID, nil, gomock.Any()).
// 					Return(nil).
// 					Times(1)
//
// 				// Simula exclusão do asset
// 				mockAssetRepo.EXPECT().
// 					Delete(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(nil).
// 					Times(1)
// 			},
// 			expectedErr: nil,
// 		},
// 		{
// 			name: "success - asset deleted without external account",
// 			setupMocks: func() {
// 				// Simula encontrar o asset
// 				mockAssetRepo.EXPECT().
// 					Find(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(&mmodel.Asset{ID: assetID.String(), Code: "USD"}, nil).
// 					Times(1)
//
// 				// Simula não encontrar contas externas
// 				mockAccountRepo.EXPECT().
// 					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"EXT-USD"}).
// 					Return([]mmodel.Account{}, nil).
// 					Times(1)
//
// 				// Simula exclusão do asset
// 				mockAssetRepo.EXPECT().
// 					Delete(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(nil).
// 					Times(1)
// 			},
// 			expectedErr: nil,
// 		},
// 		{
// 			name: "failure - asset not found",
// 			setupMocks: func() {
// 				mockAssetRepo.EXPECT().
// 					Find(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(nil, services.ErrDatabaseItemNotFound).
// 					Times(1)
// 			},
// 			expectedErr: constant.ErrAssetIDNotFound,
// 		},
// 		{
// 			name: "failure - delete external account error",
// 			setupMocks: func() {
// 				mockAssetRepo.EXPECT().
// 					Find(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(&mmodel.Asset{ID: assetID.String(), Code: "USD"}, nil).
// 					Times(1)
//
// 				mockAccountRepo.EXPECT().
// 					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"EXT-USD"}).
// 					Return([]mmodel.Account{
// 						{ID: uuid.New().String()},
// 					}, nil).
// 					Times(1)
//
// 				mockAccountRepo.EXPECT().
// 					Delete(gomock.Any(), organizationID, ledgerID, nil, gomock.Any()).
// 					Return(errors.New("failed to delete external account")).
// 					Times(1)
// 			},
// 			expectedErr: errors.New("failed to delete external account"),
// 		},
// 		{
// 			name: "failure - delete asset error",
// 			setupMocks: func() {
// 				mockAssetRepo.EXPECT().
// 					Find(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(&mmodel.Asset{ID: assetID.String(), Code: "USD"}, nil).
// 					Times(1)
//
// 				mockAccountRepo.EXPECT().
// 					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"EXT-USD"}).
// 					Return([]mmodel.Account{}, nil).
// 					Times(1)
//
// 				mockAssetRepo.EXPECT().
// 					Delete(gomock.Any(), organizationID, ledgerID, assetID).
// 					Return(errors.New("failed to delete asset")).
// 					Times(1)
// 			},
// 			expectedErr: errors.New("failed to delete asset"),
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// Configura os mocks
// 			tt.setupMocks()
//
// 			// Executa a função
// 			err := uc.DeleteAssetByID(ctx, organizationID, ledgerID, assetID)
//
// 			// Validações
// 			if tt.expectedErr != nil {
// 				assert.Error(t, err)
// 				assert.Equal(t, tt.expectedErr.Error(), err.Error())
// 			} else {
// 				assert.NoError(t, err)
// 			}
// 		})
// 	}
// }

// TestDeleteAssetByIDSuccess is responsible to test DeleteAssetByID with success
func TestDeleteAssetByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)
	err := uc.AssetRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.Nil(t, err)
}

// TestDeleteAssetByIDError is responsible to test DeleteAssetByID with error
func TestDeleteAssetByIDError(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	organizationID := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AssetRepo: asset.NewMockRepository(gomock.NewController(t)),
	}

	uc.AssetRepo.(*asset.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.AssetRepo.Delete(context.TODO(), organizationID, ledgerID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
