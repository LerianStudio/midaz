package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mpointers"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestCreateAsset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRepo:   mockAssetRepo,
		AccountRepo: mockAccountRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name        string
		input       *mmodel.CreateAssetInput
		mockSetup   func()
		expectedErr error
		expectedRes *mmodel.Asset
	}{
		{
			name: "success - asset created",
			input: &mmodel.CreateAssetInput{
				Name: "USD Dollar",
				Type: "currency",
				Code: "USD",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: mpointers.String("Active asset"),
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "USD Dollar", "USD").
					Return(false, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Asset{
						ID:        uuid.New().String(),
						Name:      "USD Dollar",
						Type:      "currency",
						Code:      "USD",
						Status:    mmodel.Status{Code: "ACTIVE"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, gomock.Any()).
					Return(nil, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{}, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedRes: &mmodel.Asset{
				Name: "USD Dollar",
				Type: "currency",
				Code: "USD",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
		},
		{
			name: "failure - invalid type",
			input: &mmodel.CreateAssetInput{
				Name: "Invalid Asset",
				Type: "invalidType",
				Code: "INV",
			},
			mockSetup:   func() {},
			expectedErr: errors.New("0040 - The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type."),
			expectedRes: nil,
		},
		{
			name: "failure - error creating asset",
			input: &mmodel.CreateAssetInput{
				Name: "USD Dollar",
				Type: "currency",
				Code: "USD",
			},
			mockSetup: func() {
				mockAssetRepo.EXPECT().
					FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "USD Dollar", "USD").
					Return(false, nil).
					Times(1)

				mockAssetRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to create asset")).
					Times(1)
			},
			expectedErr: errors.New("failed to create asset"),
			expectedRes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CreateAsset(ctx, organizationID, ledgerID, tt.input)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedRes.Name, result.Name)
				assert.Equal(t, tt.expectedRes.Code, result.Code)
				assert.Equal(t, tt.expectedRes.Type, result.Type)
			}
		})
	}
}
