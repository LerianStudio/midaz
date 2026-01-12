package in

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	balancepb "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestBalanceProto_CreateBalance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		req         *balancepb.BalanceRequest
		setupMocks  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository)
		wantErr     bool
		errContains string
		validate    func(t *testing.T, resp *balancepb.BalanceResponse)
	}{
		{
			name: "successful balance creation with default key",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				Alias:          "@user1",
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				RequestId:      uuid.New().String(),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {
				// For default key, no existence check needed
				balanceRepo.EXPECT().
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.DefaultBalanceKey).
					Return(false, nil).
					Times(1)
				balanceRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			wantErr: false,
			validate: func(t *testing.T, resp *balancepb.BalanceResponse) {
				assert.NotEmpty(t, resp.Id)
				assert.Equal(t, "@user1", resp.Alias)
				assert.Equal(t, constant.DefaultBalanceKey, resp.Key)
				assert.Equal(t, "USD", resp.AssetCode)
				assert.True(t, resp.AllowSending)
				assert.True(t, resp.AllowReceiving)
			},
		},
		{
			name: "successful balance creation with non-default key",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				Alias:          "@user1",
				Key:            "savings",
				AssetCode:      "BRL",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				RequestId:      uuid.New().String(),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {
				// For non-default key, check default balance exists first
				balanceRepo.EXPECT().
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.DefaultBalanceKey).
					Return(true, nil).
					Times(1)
				// Then check if the new key already exists
				balanceRepo.EXPECT().
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "savings").
					Return(false, nil).
					Times(1)
				balanceRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			wantErr: false,
			validate: func(t *testing.T, resp *balancepb.BalanceResponse) {
				assert.NotEmpty(t, resp.Id)
				assert.Equal(t, "savings", resp.Key)
				assert.Equal(t, "BRL", resp.AssetCode)
			},
		},
		{
			name: "invalid organization_id returns error",
			req: &balancepb.BalanceRequest{
				OrganizationId: "invalid-uuid",
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				Alias:          "@user1",
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
			},
			setupMocks:  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {},
			wantErr:     true,
			errContains: "organizationId",
		},
		{
			name: "invalid ledger_id returns error",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       "invalid-uuid",
				AccountId:      uuid.New().String(),
				Alias:          "@user1",
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
			},
			setupMocks:  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {},
			wantErr:     true,
			errContains: "ledgerId",
		},
		{
			name: "invalid account_id returns error",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      "invalid-uuid",
				Alias:          "@user1",
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
			},
			setupMocks:  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {},
			wantErr:     true,
			errContains: "accountId",
		},
		{
			name: "duplicate key returns error",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				Alias:          "@user1",
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
				AccountType:    "deposit",
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {
				balanceRepo.EXPECT().
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.DefaultBalanceKey).
					Return(true, nil).
					Times(1)
			},
			wantErr:     true,
			errContains: "key value already exists", // EntityConflictError doesn't include code in Error()
		},
		{
			name: "repository create error returns error",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				Alias:          "@user1",
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
				AccountType:    "deposit",
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {
				balanceRepo.EXPECT().
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.DefaultBalanceKey).
					Return(false, nil).
					Times(1)
				balanceRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(errors.New("database connection error")).
					Times(1)
			},
			wantErr:     true,
			errContains: "database connection error",
		},
		{
			name: "non-default key without default balance returns error",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				Alias:          "@user1",
				Key:            "savings",
				AssetCode:      "USD",
				AccountType:    "deposit",
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {
				balanceRepo.EXPECT().
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.DefaultBalanceKey).
					Return(false, nil).
					Times(1)
			},
			wantErr:     true,
			errContains: "Default balance must be created first", // EntityNotFoundError doesn't include code in Error()
		},
		{
			name: "external account type cannot have additional balance",
			req: &balancepb.BalanceRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				Alias:          "@external",
				Key:            "additional",
				AssetCode:      "USD",
				AccountType:    constant.ExternalAccountType,
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository) {
				balanceRepo.EXPECT().
					ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.DefaultBalanceKey).
					Return(true, nil).
					Times(1)
			},
			wantErr:     true,
			errContains: "0124", // ErrAdditionalBalanceNotAllowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo)

			uc := &command.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}

			handler := &BalanceProto{
				Command: uc,
			}

			resp, err := handler.CreateBalance(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			if tt.validate != nil {
				tt.validate(t, resp)
			}
		})
	}
}

func TestBalanceProto_DeleteAllBalancesByAccountID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		req         *balancepb.DeleteAllBalancesByAccountIDRequest
		setupMocks  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful deletion when no balances exist",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				RequestId:      uuid.New().String(),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Balance{}, nil).
					Times(1)
			},
			wantErr: false,
		},
		{
			name: "successful deletion with zero-balance accounts",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				RequestId:      uuid.New().String(),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balanceID := uuid.New()
				balances := []*mmodel.Balance{
					{
						ID:             balanceID.String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@user1",
						Key:            constant.DefaultBalanceKey,
						AssetCode:      "USD",
						Available:      decimal.Zero,
						OnHold:         decimal.Zero,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					},
				}
				balanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(balances, nil).
					Times(1)
				redisRepo.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@user1#default").
					Return(nil, nil).
					Times(1)
				balanceRepo.EXPECT().
					UpdateAllByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
				balanceRepo.EXPECT().
					DeleteAllByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			wantErr: false,
		},
		{
			name: "invalid organization_id returns error",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: "invalid-uuid",
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				RequestId:      uuid.New().String(),
			},
			setupMocks:  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {},
			wantErr:     true,
			errContains: "organizationId",
		},
		{
			name: "invalid ledger_id returns error",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       "invalid-uuid",
				AccountId:      uuid.New().String(),
				RequestId:      uuid.New().String(),
			},
			setupMocks:  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {},
			wantErr:     true,
			errContains: "ledgerId",
		},
		{
			name: "invalid account_id returns error",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      "invalid-uuid",
				RequestId:      uuid.New().String(),
			},
			setupMocks:  func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {},
			wantErr:     true,
			errContains: "accountId",
		},
		{
			name: "repository list error returns error",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				RequestId:      uuid.New().String(),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database connection error")).
					Times(1)
			},
			wantErr:     true,
			errContains: "database connection error",
		},
		{
			name: "balance with non-zero available cannot be deleted",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				RequestId:      uuid.New().String(),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balances := []*mmodel.Balance{
					{
						ID:             uuid.New().String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@user1",
						Key:            constant.DefaultBalanceKey,
						AssetCode:      "USD",
						Available:      decimal.NewFromInt(100),
						OnHold:         decimal.Zero,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					},
				}
				balanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(balances, nil).
					Times(1)
				redisRepo.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@user1#default").
					Return(nil, nil).
					Times(1)
			},
			wantErr:     true,
			errContains: "0093", // ErrBalancesCantBeDeleted
		},
		{
			name: "balance with non-zero on-hold cannot be deleted",
			req: &balancepb.DeleteAllBalancesByAccountIDRequest{
				OrganizationId: uuid.New().String(),
				LedgerId:       uuid.New().String(),
				AccountId:      uuid.New().String(),
				RequestId:      uuid.New().String(),
			},
			setupMocks: func(balanceRepo *balance.MockRepository, redisRepo *redis.MockRedisRepository, orgID, ledgerID, accountID uuid.UUID) {
				balances := []*mmodel.Balance{
					{
						ID:             uuid.New().String(),
						OrganizationID: orgID.String(),
						LedgerID:       ledgerID.String(),
						AccountID:      accountID.String(),
						Alias:          "@user1",
						Key:            constant.DefaultBalanceKey,
						AssetCode:      "USD",
						Available:      decimal.Zero,
						OnHold:         decimal.NewFromInt(50),
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					},
				}
				balanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(balances, nil).
					Times(1)
				redisRepo.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@user1#default").
					Return(nil, nil).
					Times(1)
			},
			wantErr:     true,
			errContains: "0093", // ErrBalancesCantBeDeleted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			// Parse IDs for mock setup (use request IDs if valid, otherwise use dummy UUIDs)
			orgID, _ := uuid.Parse(tt.req.GetOrganizationId())
			ledgerID, _ := uuid.Parse(tt.req.GetLedgerId())
			accountID, _ := uuid.Parse(tt.req.GetAccountId())

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockBalanceRepo, mockRedisRepo, orgID, ledgerID, accountID)

			uc := &command.UseCase{
				BalanceRepo: mockBalanceRepo,
				RedisRepo:   mockRedisRepo,
			}

			handler := &BalanceProto{
				Command: uc,
			}

			resp, err := handler.DeleteAllBalancesByAccountID(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.IsType(t, &balancepb.Empty{}, resp)
		})
	}
}
