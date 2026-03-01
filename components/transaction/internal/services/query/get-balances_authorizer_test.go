// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

type stubAuthorizer struct {
	enabled bool
	err     error

	authorizeResponses []*authorizerv1.AuthorizeResponse
	authorizeCalls     int

	loadCalls       int
	loadErr         error
	lastLoadRequest *authorizerv1.LoadBalancesRequest
}

func (s *stubAuthorizer) Enabled() bool {
	return s.enabled
}

func (s *stubAuthorizer) Authorize(_ context.Context, _ *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	s.authorizeCalls++

	if s.err != nil {
		return nil, s.err
	}

	if len(s.authorizeResponses) == 0 {
		return &authorizerv1.AuthorizeResponse{Authorized: true}, nil
	}

	index := s.authorizeCalls - 1
	if index >= len(s.authorizeResponses) {
		index = len(s.authorizeResponses) - 1
	}

	return s.authorizeResponses[index], nil
}

func (s *stubAuthorizer) LoadBalances(_ context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error) {
	s.loadCalls++
	s.lastLoadRequest = req

	if s.loadErr != nil {
		return nil, s.loadErr
	}

	return &authorizerv1.LoadBalancesResponse{BalancesLoaded: 1}, nil
}

func TestGetBalancesUsesAuthorizerWhenEnabled(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo:       mockRedisRepo,
		BalanceCacheTTL: 30 * time.Second,
		Authorizer: &stubAuthorizer{
			enabled: true,
			authorizeResponses: []*authorizerv1.AuthorizeResponse{
				{
					Authorized: true,
					Balances: []*authorizerv1.BalanceSnapshot{
						{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", BalanceId: "b1", AccountId: "a1", AssetCode: "USD", Available: 9000, OnHold: 0, Scale: 2, Version: 2, AllowSending: true, AllowReceiving: true, AccountType: "deposit"},
						{OperationAlias: "1#@bob#default", AccountAlias: "@bob", BalanceKey: "default", BalanceId: "b2", AccountId: "a2", AssetCode: "USD", Available: 1000, OnHold: 0, Scale: 2, Version: 2, AllowSending: true, AllowReceiving: true, AccountType: "deposit"},
					},
				},
			},
		},
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	validate := &pkgTransaction.Responses{
		Aliases: []string{"@alice#default", "@bob#default"},
		From: map[string]pkgTransaction.Amount{
			"0#@alice#default": {
				Asset:     "USD",
				Value:     decimal.NewFromInt(10),
				Operation: constant.DEBIT,
			},
		},
		To: map[string]pkgTransaction.Amount{
			"1#@bob#default": {
				Asset:     "USD",
				Value:     decimal.NewFromInt(10),
				Operation: constant.CREDIT,
			},
		},
	}

	aliceRedis, err := json.Marshal(mmodel.BalanceRedis{ID: "b1", AccountID: "a1", Available: decimal.NewFromInt(100), OnHold: decimal.Zero, Version: 1, AccountType: "deposit", AllowSending: 1, AllowReceiving: 1, AssetCode: "USD"})
	require.NoError(t, err)
	bobRedis, err := json.Marshal(mmodel.BalanceRedis{ID: "b2", AccountID: "a2", Available: decimal.Zero, OnHold: decimal.Zero, Version: 1, AccountType: "deposit", AllowSending: 1, AllowReceiving: 1, AssetCode: "USD"})
	require.NoError(t, err)

	keyAlice := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#default")
	keyBob := utils.BalanceInternalKey(organizationID, ledgerID, "@bob#default")

	mockRedisRepo.EXPECT().Get(gomock.Any(), keyAlice).Return(string(aliceRedis), nil).Times(1)
	mockRedisRepo.EXPECT().Get(gomock.Any(), keyBob).Return(string(bobRedis), nil).Times(1)
	mockRedisRepo.EXPECT().Set(gomock.Any(), keyAlice, gomock.Any(), 30*time.Second).Return(nil).Times(1)
	mockRedisRepo.EXPECT().Set(gomock.Any(), keyBob, gomock.Any(), 30*time.Second).Return(nil).Times(1)

	balances, err := uc.GetBalances(ctx, organizationID, ledgerID, transactionID, nil, validate, constant.CREATED)
	require.NoError(t, err)
	require.Len(t, balances, 2)
	require.Equal(t, "0#@alice#default", balances[0].Alias)
	require.True(t, balances[0].Available.Equal(decimal.NewFromInt(90)))
	require.Equal(t, "1#@bob#default", balances[1].Alias)
	require.True(t, balances[1].Available.Equal(decimal.NewFromInt(10)))
}

func TestMapAuthorizerRejectionInternalError(t *testing.T) {
	t.Parallel()

	err := mapAuthorizerRejection("INTERNAL_ERROR")
	require.Error(t, err)
	require.Equal(t, pkg.ValidateBusinessError(constant.ErrInternalServer, "authorizer").Error(), err.Error())
}

func TestProcessAuthorizerAtomicOperation_RetryOnBalanceNotFound(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balance := &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      "account-1",
		Alias:          "@alice",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	operationAlias := "0#@alice#default"
	balanceOperations := []mmodel.BalanceOperation{
		{
			Alias:   operationAlias,
			Balance: balance,
			Amount: pkgTransaction.Amount{
				Asset:     "USD",
				Value:     decimal.NewFromInt(10),
				Operation: constant.DEBIT,
			},
		},
	}

	stub := &stubAuthorizer{
		enabled: true,
		authorizeResponses: []*authorizerv1.AuthorizeResponse{
			{Authorized: false, RejectionCode: "BALANCE_NOT_FOUND"},
			{
				Authorized: true,
				Balances: []*authorizerv1.BalanceSnapshot{
					{
						OperationAlias: operationAlias,
						AccountAlias:   "@alice",
						BalanceKey:     "default",
						BalanceId:      "balance-1",
						AccountId:      "account-1",
						AssetCode:      "USD",
						AccountType:    "deposit",
						AllowSending:   true,
						AllowReceiving: true,
						Available:      9000,
						OnHold:         0,
						Scale:          2,
						Version:        2,
					},
				},
			},
		},
	}

	uc := &UseCase{
		Authorizer:  stub,
		ShardRouter: shard.NewRouter(8),
	}

	newBalances, err := uc.processAuthorizerAtomicOperation(
		context.Background(),
		organizationID,
		ledgerID,
		transactionID,
		constant.CREATED,
		false,
		balanceOperations,
		map[string]*mmodel.Balance{operationAlias: balance},
	)
	require.NoError(t, err)
	require.Len(t, newBalances, 1)
	require.Equal(t, 2, stub.authorizeCalls)
	require.Equal(t, 1, stub.loadCalls)
	require.NotNil(t, stub.lastLoadRequest)
	require.Equal(t, organizationID.String(), stub.lastLoadRequest.GetOrganizationId())
	require.Equal(t, ledgerID.String(), stub.lastLoadRequest.GetLedgerId())
	require.NotEmpty(t, stub.lastLoadRequest.GetShardIds())
}

func TestProcessAuthorizerAtomicOperation_LagFenceEnabledBlocksStaleAuthorizerLoad(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balance := &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      "account-1",
		Alias:          "@alice",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	operationAlias := "0#@alice#default"
	balanceOperations := []mmodel.BalanceOperation{{
		Alias:   operationAlias,
		Balance: balance,
		Amount: pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromInt(10),
			Operation: constant.DEBIT,
		},
	}}

	router := shard.NewRouter(8)
	partition := int32(router.ResolveBalance("@alice", "default"))

	stub := &stubAuthorizer{
		enabled: true,
		authorizeResponses: []*authorizerv1.AuthorizeResponse{
			{Authorized: false, RejectionCode: "BALANCE_NOT_FOUND"},
		},
	}

	lagChecker := &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: false}}

	uc := &UseCase{
		Authorizer:              stub,
		ShardRouter:             router,
		LagChecker:              lagChecker,
		ConsumerLagFenceEnabled: true,
		BalanceOperationsTopic:  "ledger.balance.operations",
	}

	_, err := uc.processAuthorizerAtomicOperation(
		context.Background(),
		organizationID,
		ledgerID,
		transactionID,
		constant.CREATED,
		false,
		balanceOperations,
		map[string]*mmodel.Balance{operationAlias: balance},
	)
	require.Error(t, err)

	var serviceUnavailableErr pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &serviceUnavailableErr)
	require.Equal(t, constant.ErrConsumerLagStaleBalance.Error(), serviceUnavailableErr.Code)
	require.Equal(t, 1, stub.authorizeCalls)
	require.Equal(t, 0, stub.loadCalls)
	require.Equal(t, 1, lagChecker.calls)
}

func TestProcessAuthorizerAtomicOperation_LagFenceEnabledAndCaughtUpRetriesSuccessfully(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balance := &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      "account-1",
		Alias:          "@alice",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	operationAlias := "0#@alice#default"
	balanceOperations := []mmodel.BalanceOperation{{
		Alias:   operationAlias,
		Balance: balance,
		Amount: pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromInt(10),
			Operation: constant.DEBIT,
		},
	}}

	router := shard.NewRouter(8)
	partition := int32(router.ResolveBalance("@alice", "default"))

	stub := &stubAuthorizer{
		enabled: true,
		authorizeResponses: []*authorizerv1.AuthorizeResponse{
			{Authorized: false, RejectionCode: "BALANCE_NOT_FOUND"},
			{
				Authorized: true,
				Balances: []*authorizerv1.BalanceSnapshot{{
					OperationAlias: operationAlias,
					AccountAlias:   "@alice",
					BalanceKey:     "default",
					BalanceId:      "balance-1",
					AccountId:      "account-1",
					AssetCode:      "USD",
					AccountType:    "deposit",
					AllowSending:   true,
					AllowReceiving: true,
					Available:      9000,
					OnHold:         0,
					Scale:          2,
					Version:        2,
				}},
			},
		},
	}

	lagChecker := &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: true}}

	uc := &UseCase{
		Authorizer:              stub,
		ShardRouter:             router,
		LagChecker:              lagChecker,
		ConsumerLagFenceEnabled: true,
		BalanceOperationsTopic:  "ledger.balance.operations",
	}

	newBalances, err := uc.processAuthorizerAtomicOperation(
		context.Background(),
		organizationID,
		ledgerID,
		transactionID,
		constant.CREATED,
		false,
		balanceOperations,
		map[string]*mmodel.Balance{operationAlias: balance},
	)
	require.NoError(t, err)
	require.Len(t, newBalances, 1)
	require.Equal(t, 2, stub.authorizeCalls)
	require.Equal(t, 1, stub.loadCalls)
	require.Equal(t, 1, lagChecker.calls)
}

func TestProcessAuthorizerAtomicOperation_LagFenceDisabledIgnoresLag(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	balance := &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      "account-1",
		Alias:          "@alice",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	operationAlias := "0#@alice#default"
	balanceOperations := []mmodel.BalanceOperation{{
		Alias:   operationAlias,
		Balance: balance,
		Amount: pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromInt(10),
			Operation: constant.DEBIT,
		},
	}}

	router := shard.NewRouter(8)
	partition := int32(router.ResolveBalance("@alice", "default"))

	stub := &stubAuthorizer{
		enabled: true,
		authorizeResponses: []*authorizerv1.AuthorizeResponse{
			{Authorized: false, RejectionCode: "BALANCE_NOT_FOUND"},
			{
				Authorized: true,
				Balances: []*authorizerv1.BalanceSnapshot{{
					OperationAlias: operationAlias,
					AccountAlias:   "@alice",
					BalanceKey:     "default",
					BalanceId:      "balance-1",
					AccountId:      "account-1",
					AssetCode:      "USD",
					AccountType:    "deposit",
					AllowSending:   true,
					AllowReceiving: true,
					Available:      9000,
					OnHold:         0,
					Scale:          2,
					Version:        2,
				}},
			},
		},
	}

	lagChecker := &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: false}}

	uc := &UseCase{
		Authorizer:              stub,
		ShardRouter:             router,
		LagChecker:              lagChecker,
		ConsumerLagFenceEnabled: false,
		BalanceOperationsTopic:  "ledger.balance.operations",
	}

	newBalances, err := uc.processAuthorizerAtomicOperation(
		context.Background(),
		organizationID,
		ledgerID,
		transactionID,
		constant.CREATED,
		false,
		balanceOperations,
		map[string]*mmodel.Balance{operationAlias: balance},
	)
	require.NoError(t, err)
	require.Len(t, newBalances, 1)
	require.Equal(t, 2, stub.authorizeCalls)
	require.Equal(t, 1, stub.loadCalls)
	require.Equal(t, 0, lagChecker.calls)
}

func TestCacheAuthorizerBalances_SkipsNilBalances(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo:       mockRedisRepo,
		BalanceCacheTTL: 20 * time.Second,
	}

	balanceOps := []mmodel.BalanceOperation{{
		Alias:       "0#@alice#default",
		InternalKey: "balance:key:alice",
	}}

	balance := &mmodel.Balance{
		ID:             "balance-1",
		AccountID:      "account-1",
		Alias:          "0#@alice#default",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(10),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), "balance:key:alice", gomock.Any(), 20*time.Second).
		Return(nil).
		Times(1)

	uc.cacheAuthorizerBalances(context.Background(), balanceOps, []*mmodel.Balance{nil, balance})
}

func TestConvertAuthorizerSnapshots_SkipsNilEntries(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	result := convertAuthorizerSnapshots(
		[]*authorizerv1.BalanceSnapshot{
			nil,
			{
				OperationAlias: "0#@alice#default",
				BalanceKey:     "default",
				BalanceId:      "balance-1",
				AccountId:      "account-1",
				AssetCode:      "USD",
				Available:      9000,
				OnHold:         0,
				Scale:          2,
				Version:        2,
				AllowSending:   true,
				AllowReceiving: true,
				AccountType:    "deposit",
			},
		},
		organizationID,
		ledgerID,
		map[string]*mmodel.Balance{},
	)

	require.Len(t, result, 1)
	assert.Equal(t, "0#@alice#default", result[0].Alias)
}
