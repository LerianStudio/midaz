package mmodel

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionRoute_ToCache(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	transactionRouteID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	sourceRouteID := uuid.New()
	destRouteID := uuid.New()

	tests := []struct {
		name             string
		transactionRoute *TransactionRoute
		wantSourceCount  int
		wantDestCount    int
	}{
		{
			name: "empty operation routes",
			transactionRoute: &TransactionRoute{
				ID:              transactionRouteID,
				OrganizationID:  orgID,
				LedgerID:        ledgerID,
				Title:           "Empty Route",
				OperationRoutes: []OperationRoute{},
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			wantSourceCount: 0,
			wantDestCount:   0,
		},
		{
			name: "only source routes",
			transactionRoute: &TransactionRoute{
				ID:             transactionRouteID,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "Source Only Route",
				OperationRoutes: []OperationRoute{
					{
						ID:            sourceRouteID,
						OperationType: "source",
						Account: &AccountRule{
							RuleType: "alias",
							ValidIf:  "@cash_account",
						},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantSourceCount: 1,
			wantDestCount:   0,
		},
		{
			name: "only destination routes",
			transactionRoute: &TransactionRoute{
				ID:             transactionRouteID,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "Destination Only Route",
				OperationRoutes: []OperationRoute{
					{
						ID:            destRouteID,
						OperationType: "destination",
						Account: &AccountRule{
							RuleType: "account_type",
							ValidIf:  []string{"deposit", "savings"},
						},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantSourceCount: 0,
			wantDestCount:   1,
		},
		{
			name: "mixed source and destination routes",
			transactionRoute: &TransactionRoute{
				ID:             transactionRouteID,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "Mixed Routes",
				OperationRoutes: []OperationRoute{
					{
						ID:            sourceRouteID,
						OperationType: "source",
						Account: &AccountRule{
							RuleType: "alias",
							ValidIf:  "@source_account",
						},
					},
					{
						ID:            destRouteID,
						OperationType: "destination",
						Account: &AccountRule{
							RuleType: "alias",
							ValidIf:  "@destination_account",
						},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantSourceCount: 1,
			wantDestCount:   1,
		},
		{
			name: "multiple source and destination routes",
			transactionRoute: &TransactionRoute{
				ID:             transactionRouteID,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "Multiple Routes",
				OperationRoutes: []OperationRoute{
					{
						ID:            uuid.New(),
						OperationType: "source",
						Account: &AccountRule{
							RuleType: "alias",
							ValidIf:  "@source1",
						},
					},
					{
						ID:            uuid.New(),
						OperationType: "source",
						Account: &AccountRule{
							RuleType: "alias",
							ValidIf:  "@source2",
						},
					},
					{
						ID:            uuid.New(),
						OperationType: "destination",
						Account: &AccountRule{
							RuleType: "alias",
							ValidIf:  "@dest1",
						},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantSourceCount: 2,
			wantDestCount:   1,
		},
		{
			name: "operation route without account rule",
			transactionRoute: &TransactionRoute{
				ID:             transactionRouteID,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "No Account Rule",
				OperationRoutes: []OperationRoute{
					{
						ID:            sourceRouteID,
						OperationType: "source",
						Account:       nil,
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantSourceCount: 1,
			wantDestCount:   0,
		},
		{
			name: "unknown operation type is ignored",
			transactionRoute: &TransactionRoute{
				ID:             transactionRouteID,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "Unknown Type",
				OperationRoutes: []OperationRoute{
					{
						ID:            uuid.New(),
						OperationType: "unknown",
						Account: &AccountRule{
							RuleType: "alias",
							ValidIf:  "@unknown",
						},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantSourceCount: 0,
			wantDestCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := tt.transactionRoute.ToCache()

			assert.NotNil(t, cache.Source)
			assert.NotNil(t, cache.Destination)
			assert.Len(t, cache.Source, tt.wantSourceCount)
			assert.Len(t, cache.Destination, tt.wantDestCount)
		})
	}
}

func TestTransactionRoute_ToCache_AccountRuleMapping(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	routeID := uuid.New()

	transactionRoute := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Account Rule Test",
		OperationRoutes: []OperationRoute{
			{
				ID:            routeID,
				OperationType: "source",
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@test_alias",
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	cache := transactionRoute.ToCache()

	routeIDStr := routeID.String()
	sourceRoute, exists := cache.Source[routeIDStr]
	require.True(t, exists, "Source route should exist in cache")

	require.NotNil(t, sourceRoute.Account)
	assert.Equal(t, "alias", sourceRoute.Account.RuleType)
	assert.Equal(t, "@test_alias", sourceRoute.Account.ValidIf)
}

func TestTransactionRouteCache_FromMsgpack(t *testing.T) {
	t.Parallel()

	original := TransactionRouteCache{
		Source: map[string]OperationRouteCache{
			"route-1": {
				Account: &AccountCache{
					RuleType: "alias",
					ValidIf:  "@source",
				},
			},
		},
		Destination: map[string]OperationRouteCache{
			"route-2": {
				Account: &AccountCache{
					RuleType: "account_type",
					ValidIf:  []any{"deposit"},
				},
			},
		},
	}

	// Serialize
	data, err := original.ToMsgpack()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Deserialize
	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err)

	assert.Len(t, restored.Source, 1)
	assert.Len(t, restored.Destination, 1)

	sourceRoute, exists := restored.Source["route-1"]
	require.True(t, exists)
	require.NotNil(t, sourceRoute.Account)
	assert.Equal(t, "alias", sourceRoute.Account.RuleType)
}

func TestTransactionRouteCache_ToMsgpack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cache   TransactionRouteCache
		wantErr bool
	}{
		{
			name: "empty cache",
			cache: TransactionRouteCache{
				Source:      map[string]OperationRouteCache{},
				Destination: map[string]OperationRouteCache{},
			},
			wantErr: false,
		},
		{
			name: "cache with data",
			cache: TransactionRouteCache{
				Source: map[string]OperationRouteCache{
					"src-1": {
						Account: &AccountCache{
							RuleType: "alias",
							ValidIf:  "@account",
						},
					},
				},
				Destination: map[string]OperationRouteCache{
					"dst-1": {
						Account: nil,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cache with nil account",
			cache: TransactionRouteCache{
				Source: map[string]OperationRouteCache{
					"route": {Account: nil},
				},
				Destination: map[string]OperationRouteCache{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := tt.cache.ToMsgpack()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, data)
		})
	}
}

func TestTransactionRouteCache_FromMsgpack_InvalidData(t *testing.T) {
	t.Parallel()

	var cache TransactionRouteCache
	err := cache.FromMsgpack([]byte("invalid msgpack data"))
	require.Error(t, err)
}

func TestTransactionRouteCache_RoundTrip(t *testing.T) {
	t.Parallel()

	original := TransactionRouteCache{
		Source: map[string]OperationRouteCache{
			"source-route-uuid": {
				Account: &AccountCache{
					RuleType: "alias",
					ValidIf:  "@merchant_account",
				},
			},
		},
		Destination: map[string]OperationRouteCache{
			"dest-route-uuid-1": {
				Account: &AccountCache{
					RuleType: "account_type",
					ValidIf:  []any{"checking", "savings"},
				},
			},
			"dest-route-uuid-2": {
				Account: nil,
			},
		},
	}

	// Serialize
	data, err := original.ToMsgpack()
	require.NoError(t, err)

	// Deserialize
	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err)

	// Verify source
	assert.Len(t, restored.Source, 1)
	sourceRoute := restored.Source["source-route-uuid"]
	require.NotNil(t, sourceRoute.Account)
	assert.Equal(t, "alias", sourceRoute.Account.RuleType)
	assert.Equal(t, "@merchant_account", sourceRoute.Account.ValidIf)

	// Verify destinations
	assert.Len(t, restored.Destination, 2)

	destRoute1 := restored.Destination["dest-route-uuid-1"]
	require.NotNil(t, destRoute1.Account)
	assert.Equal(t, "account_type", destRoute1.Account.RuleType)

	destRoute2 := restored.Destination["dest-route-uuid-2"]
	assert.Nil(t, destRoute2.Account)
}

func TestOperationRouteCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cache  OperationRouteCache
		hasAcc bool
	}{
		{
			name:   "nil account",
			cache:  OperationRouteCache{Account: nil},
			hasAcc: false,
		},
		{
			name: "with account",
			cache: OperationRouteCache{
				Account: &AccountCache{
					RuleType: "alias",
					ValidIf:  "@test",
				},
			},
			hasAcc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.hasAcc {
				assert.NotNil(t, tt.cache.Account)
			} else {
				assert.Nil(t, tt.cache.Account)
			}
		})
	}
}

func TestAccountCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cache    *AccountCache
		ruleType string
		validIf  any
	}{
		{
			name: "alias rule type",
			cache: &AccountCache{
				RuleType: "alias",
				ValidIf:  "@user_account",
			},
			ruleType: "alias",
			validIf:  "@user_account",
		},
		{
			name: "account_type rule type with string array",
			cache: &AccountCache{
				RuleType: "account_type",
				ValidIf:  []string{"deposit", "savings", "checking"},
			},
			ruleType: "account_type",
			validIf:  []string{"deposit", "savings", "checking"},
		},
		{
			name: "empty rule type",
			cache: &AccountCache{
				RuleType: "",
				ValidIf:  nil,
			},
			ruleType: "",
			validIf:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.ruleType, tt.cache.RuleType)
			assert.Equal(t, tt.validIf, tt.cache.ValidIf)
		})
	}
}
