package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUpdateAccountByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name         string
		balance      *mmodel.Balance
		setupMocks   func()
		expectedErr  error
		expectedAcct *mmodel.Account
	}{
		{
			name: "success - account updated",
			balance: &mmodel.Balance{
				Available: ptr.Float64Ptr(100.0),
				OnHold:    ptr.Float64Ptr(10.0),
				Scale:     ptr.Float64Ptr(0.01),
			},
			setupMocks: func() {
				updatedAccount := &mmodel.Account{
					ID: accountID.String(),
				}

				mockAccountRepo.EXPECT().
					UpdateAccountByID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(updatedAccount, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedAcct: &mmodel.Account{
				ID: accountID.String(),
			},
		},
		{
			name: "failure - account not found",
			balance: &mmodel.Balance{
				Available: ptr.Float64Ptr(50.0),
				OnHold:    ptr.Float64Ptr(5.0),
				Scale:     ptr.Float64Ptr(0.01),
			},
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					UpdateAccountByID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr:  errors.New("The provided account ID does not exist in our records. Please verify the account ID and try again."),
			expectedAcct: nil,
		},
		{
			name: "failure - repository error",
			balance: &mmodel.Balance{
				Available: ptr.Float64Ptr(200.0),
				OnHold:    ptr.Float64Ptr(20.0),
				Scale:     ptr.Float64Ptr(0.01),
			},
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					UpdateAccountByID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(nil, errors.New("failed to update account")).
					Times(1)
			},
			expectedErr:  errors.New("failed to update account"),
			expectedAcct: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.UpdateAccountByID(ctx, organizationID, ledgerID, accountID, tt.balance)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedAcct.ID, result.ID)
			}
		})
	}
}
