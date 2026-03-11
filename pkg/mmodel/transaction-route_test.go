// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
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
						Action:        "direct",
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

			assert.NotNil(t, cache.Actions)

			// These tests use routes without an explicit Action, so they group under ""
			defaultAction := cache.Actions[""]
			assert.Len(t, defaultAction.Source, tt.wantSourceCount)
			assert.Len(t, defaultAction.Destination, tt.wantDestCount)
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
	defaultAction := cache.Actions[""]
	sourceRoute, exists := defaultAction.Source[routeIDStr]
	require.True(t, exists, "Source route should exist in cache")

	require.NotNil(t, sourceRoute.Account)
	assert.Equal(t, "alias", sourceRoute.Account.RuleType)
	assert.Equal(t, "@test_alias", sourceRoute.Account.ValidIf)
}

func TestTransactionRouteCache_FromMsgpack(t *testing.T) {
	t.Parallel()

	original := TransactionRouteCache{
		Actions: map[string]ActionRouteCache{
			"direct": {
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
				Bidirectional: map[string]OperationRouteCache{},
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

	directAction := restored.Actions["direct"]
	assert.Len(t, directAction.Source, 1)
	assert.Len(t, directAction.Destination, 1)

	sourceRoute, exists := directAction.Source["route-1"]
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
				Actions: map[string]ActionRouteCache{},
			},
			wantErr: false,
		},
		{
			name: "cache with data",
			cache: TransactionRouteCache{
				Actions: map[string]ActionRouteCache{
					"direct": {
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
						Bidirectional: map[string]OperationRouteCache{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cache with nil account",
			cache: TransactionRouteCache{
				Actions: map[string]ActionRouteCache{
					"direct": {
						Source: map[string]OperationRouteCache{
							"route": {Account: nil},
						},
						Destination:   map[string]OperationRouteCache{},
						Bidirectional: map[string]OperationRouteCache{},
					},
				},
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
		Actions: map[string]ActionRouteCache{
			"direct": {
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
				Bidirectional: map[string]OperationRouteCache{},
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

	directAction := restored.Actions["direct"]

	// Verify source
	assert.Len(t, directAction.Source, 1)
	sourceRoute := directAction.Source["source-route-uuid"]
	require.NotNil(t, sourceRoute.Account)
	assert.Equal(t, "alias", sourceRoute.Account.RuleType)
	assert.Equal(t, "@merchant_account", sourceRoute.Account.ValidIf)

	// Verify destinations
	assert.Len(t, directAction.Destination, 2)

	destRoute1 := directAction.Destination["dest-route-uuid-1"]
	require.NotNil(t, destRoute1.Account)
	assert.Equal(t, "account_type", destRoute1.Account.RuleType)

	destRoute2 := directAction.Destination["dest-route-uuid-2"]
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

func TestOperationRouteActionInput_Fields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           OperationRouteActionInput
		expectedAction  string
		expectedRouteID uuid.UUID
	}{
		{
			name: "valid direct action",
			input: OperationRouteActionInput{
				Action:           "direct",
				OperationRouteID: uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a"),
			},
			expectedAction:  "direct",
			expectedRouteID: uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a"),
		},
		{
			name: "valid hold action",
			input: OperationRouteActionInput{
				Action:           "hold",
				OperationRouteID: uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b"),
			},
			expectedAction:  "hold",
			expectedRouteID: uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b"),
		},
		{
			name: "valid commit action",
			input: OperationRouteActionInput{
				Action:           "commit",
				OperationRouteID: uuid.New(),
			},
			expectedAction: "commit",
		},
		{
			name: "valid cancel action",
			input: OperationRouteActionInput{
				Action:           "cancel",
				OperationRouteID: uuid.New(),
			},
			expectedAction: "cancel",
		},
		{
			name: "valid revert action",
			input: OperationRouteActionInput{
				Action:           "revert",
				OperationRouteID: uuid.New(),
			},
			expectedAction: "revert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expectedAction, tt.input.Action)

			if tt.expectedRouteID != uuid.Nil {
				assert.Equal(t, tt.expectedRouteID, tt.input.OperationRouteID)
			}
		})
	}
}

func TestTransactionRoute_ToCache_WithActions(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	sourceRouteID := uuid.New()
	destRouteID := uuid.New()
	bidiRouteID := uuid.New()

	transactionRoute := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Multi-Action Route",
		OperationRoutes: []OperationRoute{
			{
				ID:            sourceRouteID,
				OperationType: "source",
				Action:        "direct",
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@source_direct",
				},
			},
			{
				ID:            destRouteID,
				OperationType: "destination",
				Action:        "direct",
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@dest_direct",
				},
			},
			{
				ID:            bidiRouteID,
				OperationType: "bidirectional",
				Action:        "hold",
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@bidi_hold",
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	cache := transactionRoute.ToCache()

	// Verify Actions map is populated
	require.NotNil(t, cache.Actions, "Actions map should not be nil")

	// Verify "direct" action group
	directActions, exists := cache.Actions["direct"]
	require.True(t, exists, "Actions should contain 'direct' key")
	assert.Len(t, directActions.Source, 1, "direct action should have 1 source route")
	assert.Len(t, directActions.Destination, 1, "direct action should have 1 destination route")
	assert.Len(t, directActions.Bidirectional, 0, "direct action should have 0 bidirectional routes")

	// Verify source route in direct action
	srcRoute, srcExists := directActions.Source[sourceRouteID.String()]
	require.True(t, srcExists)
	require.NotNil(t, srcRoute.Account)
	assert.Equal(t, "alias", srcRoute.Account.RuleType)

	// Verify "hold" action group
	holdActions, exists := cache.Actions["hold"]
	require.True(t, exists, "Actions should contain 'hold' key")
	assert.Len(t, holdActions.Source, 0)
	assert.Len(t, holdActions.Destination, 0)
	assert.Len(t, holdActions.Bidirectional, 1, "hold action should have 1 bidirectional route")

	// Verify total action keys
	assert.Len(t, cache.Actions, 2, "should have 'direct' and 'hold' action groups")
}

func TestTransactionRoute_ToCache_SingleAction(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	routeID := uuid.New()

	transactionRoute := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Single Action Route",
		OperationRoutes: []OperationRoute{
			{
				ID:            routeID,
				OperationType: "source",
				Action:        "commit",
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@commit_source",
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	cache := transactionRoute.ToCache()

	require.NotNil(t, cache.Actions)
	assert.Len(t, cache.Actions, 1, "Actions map should have exactly 1 action group")

	commitActions, exists := cache.Actions["commit"]
	require.True(t, exists)
	assert.Len(t, commitActions.Source, 1)
	assert.Len(t, commitActions.Destination, 0)
	assert.Len(t, commitActions.Bidirectional, 0)
}

func TestTransactionRoute_ToCache_EmptyAction(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	transactionRoute := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Empty Action Route",
		OperationRoutes: []OperationRoute{
			{
				ID:            uuid.New(),
				OperationType: "source",
				Action:        "",
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	cache := transactionRoute.ToCache()

	// Empty action routes should still be grouped under empty string key
	require.NotNil(t, cache.Actions)
	emptyActions, exists := cache.Actions[""]
	require.True(t, exists)
	assert.Len(t, emptyActions.Source, 1)
}

func TestTransactionRouteCache_Actions_RoundTrip(t *testing.T) {
	t.Parallel()

	original := TransactionRouteCache{
		Actions: map[string]ActionRouteCache{
			"direct": {
				Source: map[string]OperationRouteCache{
					"route-1": {
						Account: &AccountCache{
							RuleType: "alias",
							ValidIf:  "@source",
						},
						OperationType: "source",
					},
				},
				Destination: map[string]OperationRouteCache{
					"route-2": {
						Account: &AccountCache{
							RuleType: "alias",
							ValidIf:  "@dest",
						},
						OperationType: "destination",
					},
				},
				Bidirectional: map[string]OperationRouteCache{},
			},
		},
	}

	// Serialize to msgpack
	data, err := original.ToMsgpack()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Deserialize from msgpack
	var restored TransactionRouteCache
	err = restored.FromMsgpack(data)
	require.NoError(t, err)

	// Verify Actions roundtrip
	require.NotNil(t, restored.Actions)
	assert.Len(t, restored.Actions, 1)

	directAction, exists := restored.Actions["direct"]
	require.True(t, exists)
	assert.Len(t, directAction.Source, 1)
	assert.Len(t, directAction.Destination, 1)

	srcRoute, srcExists := directAction.Source["route-1"]
	require.True(t, srcExists)
	require.NotNil(t, srcRoute.Account)
	assert.Equal(t, "alias", srcRoute.Account.RuleType)
}

func TestCreateTransactionRouteInput_OperationRoutesType(t *testing.T) {
	t.Parallel()

	input := CreateTransactionRouteInput{
		Title: "Test Route",
		OperationRoutes: []OperationRouteActionInput{
			{
				Action:           "direct",
				OperationRouteID: uuid.New(),
			},
			{
				Action:           "hold",
				OperationRouteID: uuid.New(),
			},
		},
	}

	assert.Len(t, input.OperationRoutes, 2)
	assert.Equal(t, "direct", input.OperationRoutes[0].Action)
	assert.Equal(t, "hold", input.OperationRoutes[1].Action)
}

func TestUpdateTransactionRouteInput_OperationRoutesType(t *testing.T) {
	t.Parallel()

	routes := []OperationRouteActionInput{
		{
			Action:           "commit",
			OperationRouteID: uuid.New(),
		},
	}

	input := UpdateTransactionRouteInput{
		Title:           "Updated Route",
		OperationRoutes: &routes,
	}

	require.NotNil(t, input.OperationRoutes)
	assert.Len(t, *input.OperationRoutes, 1)
	assert.Equal(t, "commit", (*input.OperationRoutes)[0].Action)
}

func TestCreateTransactionRouteInput_OperationRouteIDs(t *testing.T) {
	t.Parallel()

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	tests := []struct {
		name     string
		input    CreateTransactionRouteInput
		expected []uuid.UUID
	}{
		{
			name: "multiple routes",
			input: CreateTransactionRouteInput{
				Title: "Test",
				OperationRoutes: []OperationRouteActionInput{
					{Action: "direct", OperationRouteID: id1},
					{Action: "hold", OperationRouteID: id2},
					{Action: "commit", OperationRouteID: id3},
				},
			},
			expected: []uuid.UUID{id1, id2, id3},
		},
		{
			name: "single route",
			input: CreateTransactionRouteInput{
				Title: "Single",
				OperationRoutes: []OperationRouteActionInput{
					{Action: "direct", OperationRouteID: id1},
				},
			},
			expected: []uuid.UUID{id1},
		},
		{
			name: "empty routes",
			input: CreateTransactionRouteInput{
				Title:           "Empty",
				OperationRoutes: []OperationRouteActionInput{},
			},
			expected: []uuid.UUID{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ids := tt.input.OperationRouteIDs()
			assert.Equal(t, tt.expected, ids)
			assert.Len(t, ids, len(tt.expected))
		})
	}
}

func TestUpdateTransactionRouteInput_OperationRouteIDs(t *testing.T) {
	t.Parallel()

	id1 := uuid.New()
	id2 := uuid.New()

	routesWithTwo := []OperationRouteActionInput{
		{Action: "direct", OperationRouteID: id1},
		{Action: "hold", OperationRouteID: id2},
	}
	emptyRoutes := []OperationRouteActionInput{}

	tests := []struct {
		name     string
		input    UpdateTransactionRouteInput
		expected []uuid.UUID
	}{
		{
			name: "nil operation routes returns nil",
			input: UpdateTransactionRouteInput{
				Title:           "Nil Routes",
				OperationRoutes: nil,
			},
			expected: nil,
		},
		{
			name: "multiple routes",
			input: UpdateTransactionRouteInput{
				Title:           "Two Routes",
				OperationRoutes: &routesWithTwo,
			},
			expected: []uuid.UUID{id1, id2},
		},
		{
			name: "empty routes slice",
			input: UpdateTransactionRouteInput{
				Title:           "Empty",
				OperationRoutes: &emptyRoutes,
			},
			expected: []uuid.UUID{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ids := tt.input.OperationRouteIDs()

			if tt.expected == nil {
				assert.Nil(t, ids)
			} else {
				assert.Equal(t, tt.expected, ids)
				assert.Len(t, ids, len(tt.expected))
			}
		})
	}
}

func TestTransactionRoute_ToCache_MultipleRoutesPerAction(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	src1 := uuid.New()
	src2 := uuid.New()
	dst1 := uuid.New()

	transactionRoute := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Multiple per action",
		OperationRoutes: []OperationRoute{
			{ID: src1, OperationType: "source", Action: "direct"},
			{ID: src2, OperationType: "source", Action: "direct"},
			{ID: dst1, OperationType: "destination", Action: "direct"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	cache := transactionRoute.ToCache()

	directActions, exists := cache.Actions["direct"]
	require.True(t, exists)
	assert.Len(t, directActions.Source, 2, "direct action should have 2 source routes")
	assert.Len(t, directActions.Destination, 1, "direct action should have 1 destination route")
}

func TestTransactionRoute_ToCache_AllActionTypes(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	actions := []string{"direct", "hold", "commit", "cancel", "revert"}

	var routes []OperationRoute
	for _, action := range actions {
		routes = append(routes, OperationRoute{
			ID:            uuid.New(),
			OperationType: "source",
			Action:        action,
		})
	}

	transactionRoute := &TransactionRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		LedgerID:        uuid.New(),
		Title:           "All Actions",
		OperationRoutes: routes,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	cache := transactionRoute.ToCache()

	assert.Len(t, cache.Actions, 5, "should have all 5 action groups")

	for _, action := range actions {
		actionCache, exists := cache.Actions[action]
		assert.True(t, exists, "Actions should contain %q", action)
		assert.Len(t, actionCache.Source, 1, "action %q should have 1 source", action)
	}
}

func TestTransactionRoute_ToCache_NilOperationRoutes(t *testing.T) {
	t.Parallel()

	transactionRoute := &TransactionRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		LedgerID:        uuid.New(),
		Title:           "Nil Routes",
		OperationRoutes: nil,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	cache := transactionRoute.ToCache()

	assert.NotNil(t, cache.Actions)
	assert.Len(t, cache.Actions, 0)
}

func TestTransactionRoute_ToCache_BidirectionalWithAction(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	bidiID := uuid.New()

	transactionRoute := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Bidirectional Route",
		OperationRoutes: []OperationRoute{
			{
				ID:            bidiID,
				OperationType: "bidirectional",
				Action:        "hold",
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@bidi_account",
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	cache := transactionRoute.ToCache()

	// Actions map
	holdAction, exists := cache.Actions["hold"]
	require.True(t, exists)
	assert.Len(t, holdAction.Bidirectional, 1)
	assert.Len(t, holdAction.Source, 0)
	assert.Len(t, holdAction.Destination, 0)
}

func TestTransactionRoute_ToCache_UnknownOperationType_NoPhantomAction(t *testing.T) {
	t.Parallel()

	transactionRoute := &TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Unknown Type No Phantom",
		OperationRoutes: []OperationRoute{
			{
				ID:            uuid.New(),
				OperationType: "unknown",
				Action:        "direct",
				Account: &AccountRule{
					RuleType: "alias",
					ValidIf:  "@unknown",
				},
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	cache := transactionRoute.ToCache()

	assert.Len(t, cache.Actions, 0, "unknown operation type must not create a phantom action entry")
}

func TestOperationRouteActionInput_OneofMatchesValidActions(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(OperationRouteActionInput{})
	field, ok := typ.FieldByName("Action")
	require.True(t, ok, "OperationRouteActionInput must have an Action field")

	tag := field.Tag.Get("validate")
	require.NotEmpty(t, tag, "Action field must have a validate tag")

	// Extract oneof values from the validate tag (e.g. "required,oneof=direct hold commit cancel revert")
	var oneofValues []string

	for _, part := range strings.Split(tag, ",") {
		if strings.HasPrefix(part, "oneof=") {
			oneofValues = strings.Fields(strings.TrimPrefix(part, "oneof="))

			break
		}
	}

	require.NotEmpty(t, oneofValues, "validate tag must contain a oneof constraint")

	// Sort both slices for comparison
	sortedOneof := make([]string, len(oneofValues))
	copy(sortedOneof, oneofValues)
	sort.Strings(sortedOneof)

	sortedValid := make([]string, len(constant.ValidActions))
	copy(sortedValid, constant.ValidActions)
	sort.Strings(sortedValid)

	assert.Equal(t, sortedValid, sortedOneof,
		"oneof values in OperationRouteActionInput.Action validate tag must match constant.ValidActions")
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
