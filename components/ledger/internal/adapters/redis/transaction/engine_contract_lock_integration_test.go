//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type accountingGoldenState struct {
	Available     string `json:"available"`
	OnHold        string `json:"onHold"`
	OverdraftUsed string `json:"overdraftUsed"`
	Version       int64  `json:"version"`
	Destination   bool   `json:"destination"`
}

type accountingGoldenCachedBalance struct {
	accountingGoldenState
	ID                    string `json:"id"`
	AccountID             string `json:"accountId"`
	Alias                 string `json:"alias"`
	Key                   string `json:"key"`
	AssetCode             string `json:"assetCode"`
	AccountType           string `json:"accountType"`
	Direction             string `json:"direction"`
	AllowSending          int    `json:"allowSending"`
	AllowReceiving        int    `json:"allowReceiving"`
	AllowOverdraft        int    `json:"allowOverdraft"`
	OverdraftLimitEnabled int    `json:"overdraftLimitEnabled"`
	OverdraftLimit        string `json:"overdraftLimit"`
	BalanceScope          string `json:"balanceScope"`
}

type accountingGoldenLeg struct {
	Operation   string `json:"operation"`
	Amount      string `json:"amount"`
	Cap         string `json:"cap"`
	Destination bool   `json:"destination"`
}

type accountingGoldenScenario struct {
	Name  string `json:"name"`
	Input struct {
		Initial            []accountingGoldenState `json:"initial"`
		Operations         []accountingGoldenLeg   `json:"operations"`
		Status             string                  `json:"status"`
		Direction          string                  `json:"direction"`
		Limit              string                  `json:"limit"`
		Pending            bool                    `json:"pending"`
		Route              bool                    `json:"route"`
		Overdraft          bool                    `json:"overdraft"`
		External           bool                    `json:"external"`
		Deleted            bool                    `json:"deleted"`
		DeletedDestination bool                    `json:"deletedDestination,omitempty"`
		Cold               bool                    `json:"cold"`
		Stale              bool                    `json:"stale"`
		Companion          bool                    `json:"companion"`
	} `json:"input"`
	Expected struct {
		Before            []accountingGoldenState         `json:"before"`
		After             []accountingGoldenState         `json:"after"`
		Cache             []accountingGoldenCachedBalance `json:"cache"`
		ScheduledBalances []string                        `json:"scheduledBalances"`
		ErrorCode         string                          `json:"errorCode"`
	} `json:"expected"`
}

func loadAccountingGoldens(t *testing.T) []accountingGoldenScenario {
	t.Helper()
	file, err := os.Open("testdata/engine_contract/scenarios.json")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var fixture struct {
		SchemaVersion int                        `json:"schemaVersion"`
		Scenarios     []accountingGoldenScenario `json:"scenarios"`
	}
	require.NoError(t, decoder.Decode(&fixture))
	require.Equal(t, 1, fixture.SchemaVersion, "unsupported accounting fixture schema")
	var trailing any
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF, "unexpected trailing fixture content")
	require.Len(t, fixture.Scenarios, 35, "fixture inventory changed")
	names := make(map[string]bool, len(fixture.Scenarios))
	for _, scenario := range fixture.Scenarios {
		require.NotEmpty(t, scenario.Name)
		require.False(t, names[scenario.Name], "duplicate scenario %q", scenario.Name)
		names[scenario.Name] = true
		require.Len(t, scenario.Input.Initial, 2, scenario.Name)
		require.Len(t, scenario.Expected.Cache, 2, scenario.Name)
		require.NotNil(t, scenario.Expected.Before, scenario.Name)
		require.NotNil(t, scenario.Expected.After, scenario.Name)
		require.NotNil(t, scenario.Expected.ScheduledBalances, scenario.Name)
		require.Len(t, scenario.Expected.Before, len(scenario.Expected.After), scenario.Name)
		for _, balance := range scenario.Expected.ScheduledBalances {
			require.Contains(t, []string{"source", "destination"}, balance, scenario.Name)
		}
	}
	return fixture.Scenarios
}

func TestIntegration_AccountingExecutionGoldens(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	// Fixtures are read-only: updates require an explicit review of the saved expectations.
	cases := loadAccountingGoldens(t)

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.MustParse("f047d278-82c0-454d-a120-9192f522a21c")
	ledgerID := uuid.MustParse("f921655a-c653-46e3-ad43-b737dc604372")
	transactionID := uuid.MustParse("899928e3-726c-487b-b245-ff546634ca30")
	const scheduleKey = "schedule:{transactions}:balance-sync-v2"
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			mainKey := utils.BalanceInternalKey(orgID, ledgerID, "@source#default")
			destKey := utils.BalanceInternalKey(orgID, ledgerID, "@destination#default")
			if tc.Input.Companion {
				destKey = utils.BalanceInternalKey(orgID, ledgerID, "@source#overdraft")
			}
			_, err := infra.redisContainer.Client.Del(ctx, mainKey, destKey, mainKey+":deleted", destKey+":deleted", scheduleKey).Result()
			require.NoError(t, err)
			direction := tc.Input.Direction
			if direction == "" {
				direction = "credit"
			}
			accountType := "deposit"
			if tc.Input.External {
				accountType = "external"
			}
			limit := tc.Input.Limit
			if limit == "" {
				limit = "0"
			}
			states := make(map[bool]accountingGoldenState, 2)
			for _, initial := range tc.Input.Initial {
				states[initial.Destination] = initial
			}
			keys := map[bool]string{false: mainKey, true: destKey}
			for destination, initial := range states {
				if tc.Input.Cold && !destination {
					continue
				}
				seed := accountingGoldenCache(initial, direction, accountType, tc.Input.Overdraft, tc.Input.Limit != "", limit)
				if destination && tc.Input.Companion {
					seed["Alias"], seed["Key"], seed["Direction"] = "@source", "overdraft", "debit"
					seed["AllowOverdraft"], seed["BalanceScope"] = 0, "internal"
				}
				encoded, err := json.Marshal(seed)
				require.NoError(t, err)
				require.NoError(t, infra.redisContainer.Client.Set(ctx, keys[destination], encoded, time.Hour).Err())
			}
			if tc.Input.Deleted {
				markedKey := keys[tc.Input.DeletedDestination]
				require.NoError(t, infra.redisContainer.Client.Set(ctx, markedKey+":deleted", "1", time.Hour).Err())
			}
			var beforeDeletionRefusal map[string]limitNormalizationRedisState
			if tc.Input.Deleted {
				beforeDeletionRefusal = captureLimitNormalizationRedisState(t, infra)
			}
			operations := make([]mmodel.BalanceOperation, 0, len(tc.Input.Operations))
			for _, specification := range tc.Input.Operations {
				initial := states[specification.Destination]
				alias := "@source"
				if specification.Destination {
					alias = "@destination"
				}
				op := redistestutil.CreatePendingBalanceOperation(orgID, ledgerID, alias, "USD", specification.Operation,
					decimal.RequireFromString(specification.Amount), decimal.RequireFromString(initial.Available),
					decimal.RequireFromString(initial.OnHold), initial.Version, accountType, tc.Input.Route && !specification.Destination)
				op.Balance.ID = "fd114916-ea99-4eef-a754-146417d1de59"
				op.Balance.AccountID = "d2e6f19c-d14b-41c9-b59e-2f8ec4926e88"
				op.Balance.Key = "default"
				if specification.Destination {
					op.Balance.ID = "3746cd63-0a07-48c6-9df2-3f54630c60ed"
					op.Balance.AccountID = "c36d2ec0-934d-4b57-b9f6-4346ba308f2a"
				}
				if tc.Input.Stale {
					op.Balance.Version = 1
				}
				op.Balance.Direction = direction
				op.Balance.OverdraftUsed = decimal.RequireFromString(initial.OverdraftUsed)
				op.Balance.Settings = &mmodel.BalanceSettings{AllowOverdraft: tc.Input.Overdraft}
				op.Amount.OverdraftAmount = decimal.RequireFromString(specification.Cap)
				if specification.Destination && tc.Input.Companion {
					op.Alias, op.Balance.Alias, op.Balance.Key = "@source", "@source", "overdraft"
					op.Balance.Direction = "debit"
					op.InternalKey = destKey
				}
				operations = append(operations, op)
			}
			status := tc.Input.Status
			if status == "" {
				status = "APPROVED"
			}
			result, err := infra.processBalanceAtomicOperationWithoutBlockException(ctx, orgID, ledgerID, transactionID, status, tc.Input.Pending, operations)
			if tc.Input.Deleted {
				assert.Equal(t, beforeDeletionRefusal, captureLimitNormalizationRedisState(t, infra), "a deletion marker must reject before any bytes, expiry, schedule or backup changes")
			}
			if tc.Expected.ErrorCode != "" {
				require.Error(t, err)
				require.Nil(t, result)
				encoded, marshalErr := json.Marshal(err)
				require.NoError(t, marshalErr)
				var response struct {
					Code string `json:"code"`
				}
				require.NoError(t, json.Unmarshal(encoded, &response))
				assert.Equal(t, tc.Expected.ErrorCode, response.Code)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, result.Before, len(tc.Expected.After))
				require.Len(t, result.After, len(tc.Expected.After))
				for i, after := range tc.Expected.After {
					assertAccountingGoldenBalance(t, tc.Expected.Before[i], result.Before[i], tc.Input.Companion)
					assertAccountingGoldenBalance(t, after, result.After[i], tc.Input.Companion)
				}
			}
			for _, expected := range tc.Expected.Cache {
				destination := expected.Destination
				raw, err := infra.redisContainer.Client.Get(ctx, keys[destination]).Bytes()
				require.NoError(t, err)
				cached := decodeAccountingGoldenLegacyCache(t, raw)
				cached.Destination = expected.Destination
				assert.Equal(t, expected, cached)
			}
			wantMembers := make([]string, 0, len(tc.Expected.ScheduledBalances))
			for _, balance := range tc.Expected.ScheduledBalances {
				wantMembers = append(wantMembers, keys[balance == "destination"])
			}
			members, err := infra.redisContainer.Client.ZRange(ctx, scheduleKey, 0, -1).Result()
			require.NoError(t, err)
			assert.ElementsMatch(t, wantMembers, members)
		})
	}
}

func decodeAccountingGoldenLegacyCache(t *testing.T, raw []byte) accountingGoldenCachedBalance {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	legacy := make(map[string]json.RawMessage, 18)
	for _, key := range []string{
		"ID", "AccountID", "Alias", "Key", "AssetCode", "AccountType", "Direction",
		"Available", "OnHold", "OverdraftUsed", "Version", "AllowSending", "AllowReceiving",
		"AllowOverdraft", "OverdraftLimitEnabled", "OverdraftLimit", "BalanceScope",
	} {
		if value, exists := fields[key]; exists {
			legacy[key] = value
		}
	}
	filtered, err := json.Marshal(legacy)
	require.NoError(t, err)
	var cached accountingGoldenCachedBalance
	require.NoError(t, json.Unmarshal(filtered, &cached))

	return cached
}

func accountingGoldenCache(state accountingGoldenState, direction, accountType string, overdraft, limited bool, limit string) map[string]any {
	bit := func(value bool) int {
		if value {
			return 1
		}
		return 0
	}
	alias := "@source"
	balanceID := "fd114916-ea99-4eef-a754-146417d1de59"
	accountID := "d2e6f19c-d14b-41c9-b59e-2f8ec4926e88"
	if state.Destination {
		alias = "@destination"
		balanceID = "3746cd63-0a07-48c6-9df2-3f54630c60ed"
		accountID = "c36d2ec0-934d-4b57-b9f6-4346ba308f2a"
	}
	return map[string]any{
		"ID": balanceID, "AccountID": accountID,
		"Alias": alias, "Key": "default", "AssetCode": "USD", "AccountType": accountType,
		"Available": state.Available, "OnHold": state.OnHold, "OverdraftUsed": state.OverdraftUsed, "Version": state.Version,
		"Direction": direction, "AllowSending": 1, "AllowReceiving": 1, "AllowOverdraft": bit(overdraft),
		"OverdraftLimitEnabled": bit(limited), "OverdraftLimit": limit, "BalanceScope": "transactional",
	}
}

func assertAccountingGoldenBalance(t *testing.T, expected accountingGoldenState, actual *mmodel.Balance, companion bool) {
	t.Helper()

	require.NotNil(t, actual)
	assert.Equal(t, expected.Available, actual.Available.String())
	assert.Equal(t, expected.OnHold, actual.OnHold.String())
	assert.Equal(t, expected.OverdraftUsed, actual.OverdraftUsed.String())
	assert.Equal(t, expected.Version, actual.Version)
	alias := "@source"
	if expected.Destination && !companion {
		alias = "@destination"
	}
	assert.Equal(t, alias, actual.Alias)
}
