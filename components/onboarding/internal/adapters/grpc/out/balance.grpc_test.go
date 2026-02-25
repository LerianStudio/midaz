// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"errors"
	"fmt"
	"testing"

	libConstant "github.com/LerianStudio/lib-commons/v3/commons/constants"
	proto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"
)

func TestExtractAuthToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name: "token_present",
			ctx: metadata.NewOutgoingContext(
				context.Background(),
				metadata.Pairs(libConstant.MetadataAuthorization, "Bearer test-token-123"),
			),
			expected: "Bearer test-token-123",
		},
		{
			name:     "token_absent",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name: "empty_metadata",
			ctx: metadata.NewOutgoingContext(
				context.Background(),
				metadata.MD{},
			),
			expected: "",
		},
		{
			name: "other_metadata_no_auth",
			ctx: metadata.NewOutgoingContext(
				context.Background(),
				metadata.Pairs("x-request-id", "req-123"),
			),
			expected: "",
		},
		{
			name: "multiple_auth_values",
			ctx: metadata.NewOutgoingContext(
				context.Background(),
				metadata.Pairs(
					libConstant.MetadataAuthorization, "Bearer first-token",
					libConstant.MetadataAuthorization, "Bearer second-token",
				),
			),
			expected: "Bearer first-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractAuthToken(tt.ctx)

			assert.Equal(t, tt.expected, result)
		})
	}
}

// testableBalanceAdapter is a test-specific version of BalanceAdapter that accepts
// the Repository interface instead of the concrete *BalanceGRPCRepository type.
// This allows us to inject mocks for testing.
type testableBalanceAdapter struct {
	repo Repository
}

// createBalanceSync mirrors BalanceAdapter.CreateBalanceSync for testing
func (a *testableBalanceAdapter) createBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	req := &proto.BalanceRequest{
		OrganizationId: input.OrganizationID.String(),
		LedgerId:       input.LedgerID.String(),
		AccountId:      input.AccountID.String(),
		Alias:          input.Alias,
		Key:            input.Key,
		AssetCode:      input.AssetCode,
		AccountType:    input.AccountType,
		AllowSending:   input.AllowSending,
		AllowReceiving: input.AllowReceiving,
		RequestId:      input.RequestID,
	}

	token := extractAuthToken(ctx)

	resp, err := a.repo.CreateBalance(ctx, token, req)
	if err != nil {
		return nil, err
	}

	available, err := decimal.NewFromString(resp.Available)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Available for balance %s: %w", resp.Id, err)
	}

	onHold, err := decimal.NewFromString(resp.OnHold)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OnHold for balance %s: %w", resp.Id, err)
	}

	return &mmodel.Balance{
		ID:             resp.Id,
		Alias:          resp.Alias,
		Key:            resp.Key,
		AssetCode:      resp.AssetCode,
		Available:      available,
		OnHold:         onHold,
		AllowSending:   resp.AllowSending,
		AllowReceiving: resp.AllowReceiving,
	}, nil
}

// deleteAllBalancesByAccountID mirrors BalanceAdapter.DeleteAllBalancesByAccountID for testing
func (a *testableBalanceAdapter) deleteAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, requestID string) error {
	req := &proto.DeleteAllBalancesByAccountIDRequest{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		AccountId:      accountID.String(),
		RequestId:      requestID,
	}

	token := extractAuthToken(ctx)

	return a.repo.DeleteAllBalancesByAccountID(ctx, token, req)
}

func TestBalanceAdapter_CreateBalanceSync(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name        string
		input       mmodel.CreateBalanceInput
		setupMocks  func(t *testing.T, mockRepo *MockRepository)
		wantErr     bool
		errContains string
		validate    func(t *testing.T, result *mmodel.Balance)
	}{
		{
			name: "success",
			input: mmodel.CreateBalanceInput{
				RequestID:      "req-123",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@user1",
				Key:            "default",
				AssetCode:      "USD",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					CreateBalance(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, req *proto.BalanceRequest) (*proto.BalanceResponse, error) {
						// Verify request fields are correctly converted
						assert.Equal(t, orgID.String(), req.OrganizationId)
						assert.Equal(t, ledgerID.String(), req.LedgerId)
						assert.Equal(t, accountID.String(), req.AccountId)
						assert.Equal(t, "@user1", req.Alias)
						assert.Equal(t, "default", req.Key)
						assert.Equal(t, "USD", req.AssetCode)
						assert.Equal(t, "deposit", req.AccountType)
						assert.True(t, req.AllowSending)
						assert.True(t, req.AllowReceiving)
						assert.Equal(t, "req-123", req.RequestId)

						return &proto.BalanceResponse{
							Id:             "balance-id-123",
							Alias:          "@user1",
							Key:            "default",
							AssetCode:      "USD",
							Available:      "1000.50",
							OnHold:         "100.25",
							AllowSending:   true,
							AllowReceiving: true,
						}, nil
					})
			},
			wantErr: false,
			validate: func(t *testing.T, result *mmodel.Balance) {
				t.Helper()
				require.NotNil(t, result)
				assert.Equal(t, "balance-id-123", result.ID)
				assert.Equal(t, "@user1", result.Alias)
				assert.Equal(t, "default", result.Key)
				assert.Equal(t, "USD", result.AssetCode)
				assert.True(t, result.Available.Equal(mustParseDecimal("1000.50")))
				assert.True(t, result.OnHold.Equal(mustParseDecimal("100.25")))
				assert.True(t, result.AllowSending)
				assert.True(t, result.AllowReceiving)
			},
		},
		{
			name: "grpc_error",
			input: mmodel.CreateBalanceInput{
				RequestID:      "req-456",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@user2",
				Key:            "default",
				AssetCode:      "BRL",
				AccountType:    "savings",
				AllowSending:   false,
				AllowReceiving: true,
			},
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					CreateBalance(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("gRPC connection failed"))
			},
			wantErr:     true,
			errContains: "gRPC connection failed",
		},
		{
			name: "available_decimal_parse_error",
			input: mmodel.CreateBalanceInput{
				RequestID:      "req-789",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@user3",
				Key:            "default",
				AssetCode:      "EUR",
				AccountType:    "checking",
				AllowSending:   true,
				AllowReceiving: false,
			},
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					CreateBalance(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&proto.BalanceResponse{
						Id:             "balance-id-456",
						Alias:          "@user3",
						Key:            "default",
						AssetCode:      "EUR",
						Available:      "not-a-number",
						OnHold:         "50.00",
						AllowSending:   true,
						AllowReceiving: false,
					}, nil)
			},
			wantErr:     true,
			errContains: "failed to parse Available for balance balance-id-456",
		},
		{
			name: "onhold_decimal_parse_error",
			input: mmodel.CreateBalanceInput{
				RequestID:      "req-101",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@user4",
				Key:            "freeze",
				AssetCode:      "GBP",
				AccountType:    "external",
				AllowSending:   false,
				AllowReceiving: false,
			},
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					CreateBalance(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&proto.BalanceResponse{
						Id:             "balance-id-789",
						Alias:          "@user4",
						Key:            "freeze",
						AssetCode:      "GBP",
						Available:      "200.00",
						OnHold:         "invalid-decimal",
						AllowSending:   false,
						AllowReceiving: false,
					}, nil)
			},
			wantErr:     true,
			errContains: "failed to parse OnHold for balance balance-id-789",
		},
		{
			name: "zero_balances",
			input: mmodel.CreateBalanceInput{
				RequestID:      "req-zero",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@zero-user",
				Key:            "default",
				AssetCode:      "JPY",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					CreateBalance(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&proto.BalanceResponse{
						Id:             "balance-zero",
						Alias:          "@zero-user",
						Key:            "default",
						AssetCode:      "JPY",
						Available:      "0",
						OnHold:         "0",
						AllowSending:   true,
						AllowReceiving: true,
					}, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result *mmodel.Balance) {
				t.Helper()
				require.NotNil(t, result)
				assert.True(t, result.Available.IsZero())
				assert.True(t, result.OnHold.IsZero())
			},
		},
		{
			name: "empty_decimal",
			input: mmodel.CreateBalanceInput{
				RequestID:      "req-empty",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@empty-user",
				Key:            "default",
				AssetCode:      "EUR",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					CreateBalance(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&proto.BalanceResponse{
						Id:             "balance-empty-decimal",
						Alias:          "@empty-user",
						Key:            "default",
						AssetCode:      "EUR",
						Available:      "",
						OnHold:         "0",
						AllowSending:   true,
						AllowReceiving: true,
					}, nil)
			},
			wantErr:     true,
			errContains: "failed to parse Available for balance balance-empty-decimal",
		},
		{
			name: "high_precision",
			input: mmodel.CreateBalanceInput{
				RequestID:      "req-highprec",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@highprec-user",
				Key:            "default",
				AssetCode:      "BTC",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					CreateBalance(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&proto.BalanceResponse{
						Id:             "balance-highprec",
						Alias:          "@highprec-user",
						Key:            "default",
						AssetCode:      "BTC",
						Available:      "999999999999.123456789",
						OnHold:         "0.000000001",
						AllowSending:   true,
						AllowReceiving: true,
					}, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result *mmodel.Balance) {
				t.Helper()
				require.NotNil(t, result)
				assert.Equal(t, "balance-highprec", result.ID)
				assert.True(t, result.Available.Equal(mustParseDecimal("999999999999.123456789")))
				assert.True(t, result.OnHold.Equal(mustParseDecimal("0.000000001")))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockRepo := NewMockRepository(ctrl)
			tt.setupMocks(t, mockRepo)

			adapter := &testableBalanceAdapter{repo: mockRepo}

			result, err := adapter.createBalanceSync(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestBalanceAdapter_DeleteAllBalancesByAccountID(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name        string
		orgID       uuid.UUID
		ledgerID    uuid.UUID
		accountID   uuid.UUID
		requestID   string
		setupMocks  func(t *testing.T, mockRepo *MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name:      "success",
			orgID:     orgID,
			ledgerID:  ledgerID,
			accountID: accountID,
			requestID: "delete-req-123",
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, req *proto.DeleteAllBalancesByAccountIDRequest) error {
						// Verify request fields are correctly converted
						assert.Equal(t, orgID.String(), req.OrganizationId)
						assert.Equal(t, ledgerID.String(), req.LedgerId)
						assert.Equal(t, accountID.String(), req.AccountId)
						assert.Equal(t, "delete-req-123", req.RequestId)
						return nil
					})
			},
			wantErr: false,
		},
		{
			name:      "grpc_error",
			orgID:     orgID,
			ledgerID:  ledgerID,
			accountID: accountID,
			requestID: "delete-req-456",
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("gRPC delete operation failed"))
			},
			wantErr:     true,
			errContains: "gRPC delete operation failed",
		},
		{
			name:      "empty_request_id",
			orgID:     orgID,
			ledgerID:  ledgerID,
			accountID: accountID,
			requestID: "",
			setupMocks: func(t *testing.T, mockRepo *MockRepository) {
				t.Helper()
				mockRepo.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, req *proto.DeleteAllBalancesByAccountIDRequest) error {
						assert.Empty(t, req.RequestId)
						return nil
					})
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockRepo := NewMockRepository(ctrl)
			tt.setupMocks(t, mockRepo)

			adapter := &testableBalanceAdapter{repo: mockRepo}

			err := adapter.deleteAllBalancesByAccountID(context.Background(), tt.orgID, tt.ledgerID, tt.accountID, tt.requestID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestBalanceAdapter_CreateBalanceSync_WithAuthToken(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockRepo := NewMockRepository(ctrl)

	// Verify that the token is extracted and passed to the repository
	mockRepo.EXPECT().
		CreateBalance(gomock.Any(), "Bearer my-auth-token", gomock.Any()).
		Return(&proto.BalanceResponse{
			Id:             "balance-with-token",
			Alias:          "@token-user",
			Key:            "default",
			AssetCode:      "USD",
			Available:      "500.00",
			OnHold:         "0",
			AllowSending:   true,
			AllowReceiving: true,
		}, nil)

	adapter := &testableBalanceAdapter{repo: mockRepo}

	ctx := metadata.NewOutgoingContext(
		context.Background(),
		metadata.Pairs(libConstant.MetadataAuthorization, "Bearer my-auth-token"),
	)

	input := mmodel.CreateBalanceInput{
		RequestID:      "auth-req-123",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          "@token-user",
		Key:            "default",
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	result, err := adapter.createBalanceSync(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "balance-with-token", result.ID)
}

func TestBalanceAdapter_DeleteAllBalancesByAccountID_WithAuthToken(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockRepo := NewMockRepository(ctrl)

	// Verify that the token is extracted and passed to the repository
	mockRepo.EXPECT().
		DeleteAllBalancesByAccountID(gomock.Any(), "Bearer delete-token", gomock.Any()).
		Return(nil)

	adapter := &testableBalanceAdapter{repo: mockRepo}

	ctx := metadata.NewOutgoingContext(
		context.Background(),
		metadata.Pairs(libConstant.MetadataAuthorization, "Bearer delete-token"),
	)

	err := adapter.deleteAllBalancesByAccountID(ctx, orgID, ledgerID, accountID, "delete-auth-req")

	require.NoError(t, err)
}

// mustParseDecimal is a test helper that parses a decimal string or panics.
func mustParseDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}
