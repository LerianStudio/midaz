//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type fixedLimitNormalizationRedisProvider struct {
	client redisclient.UniversalClient
}

func (p fixedLimitNormalizationRedisProvider) GetClient(context.Context) (redisclient.UniversalClient, error) {
	return p.client, nil
}

type limitNormalizationCASHook struct {
	mu        sync.Mutex
	seen      map[string]bool
	attempts  int
	key       string
	onAttempt func(context.Context, string, int) error
}

func (h *limitNormalizationCASHook) DialHook(next redisclient.DialHook) redisclient.DialHook {
	return next
}

func (h *limitNormalizationCASHook) ProcessHook(next redisclient.ProcessHook) redisclient.ProcessHook {
	return func(ctx context.Context, cmd redisclient.Cmder) error {
		args := cmd.Args()
		if !isLimitNormalizationCASCommand(cmd.Name(), args, h.key) {
			return next(ctx, cmd)
		}

		observed, ok := args[4].(string)
		if !ok {
			return next(ctx, cmd)
		}

		h.mu.Lock()
		if h.seen[observed] {
			h.mu.Unlock()
			return next(ctx, cmd)
		}
		h.seen[observed] = true
		h.attempts++
		attempt := h.attempts
		h.mu.Unlock()

		if err := h.onAttempt(ctx, observed, attempt); err != nil {
			return err
		}

		return next(ctx, cmd)
	}
}

func (h *limitNormalizationCASHook) ProcessPipelineHook(next redisclient.ProcessPipelineHook) redisclient.ProcessPipelineHook {
	return next
}

func isLimitNormalizationCASCommand(name string, args []any, key string) bool {
	if len(args) < 6 {
		return false
	}
	commandKey, ok := args[3].(string)
	if !ok || commandKey != key {
		return false
	}

	switch strings.ToLower(name) {
	case "evalsha":
		hash, ok := args[1].(string)
		return ok && hash == normalizeBalanceLimitScript.Hash()
	case "eval":
		script, ok := args[1].(string)
		return ok && script == normalizeBalanceLimitLua
	default:
		return false
	}
}

func newLimitNormalizationHookedClient(
	t *testing.T,
	infra *integrationTestInfra,
	hook *limitNormalizationCASHook,
) *redisclient.Client {
	t.Helper()
	if hook.seen == nil {
		hook.seen = make(map[string]bool)
	}

	client := redisclient.NewClient(infra.redisContainer.Client.Options())
	require.NoError(t, normalizeBalanceLimitScript.Load(t.Context(), client).Err())
	client.AddHook(hook)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	infra.repo.conn = fixedLimitNormalizationRedisProvider{client: client}

	return client
}

func newLimitNormalizationOperation(orgID, ledgerID uuid.UUID, alias string, amount int64) mmodel.BalanceOperation {
	op := redistestutil.CreateBalanceOperationWithAvailable(orgID, ledgerID, alias, "USD", constant.DEBIT,
		decimal.NewFromInt(amount), decimal.NewFromInt(100), "deposit")
	op.Balance.Available = decimal.NewFromInt(100)
	op.Balance.OnHold = decimal.Zero
	op.Balance.OverdraftUsed = decimal.Zero
	op.Balance.Version = 7
	op.Balance.Direction = "credit"
	op.Amount.RouteValidationEnabled = false
	limit := "1E+3"
	op.Balance.Settings = &mmodel.BalanceSettings{
		AllowOverdraft: true, OverdraftLimitEnabled: true,
		OverdraftLimit: &limit, BalanceScope: mmodel.BalanceScopeTransactional,
	}

	return op
}

func seedLimitNormalizationCache(t *testing.T, infra *integrationTestInfra, op mmodel.BalanceOperation, limit any) string {
	t.Helper()
	cached := map[string]any{
		"ID": op.Balance.ID, "AccountID": op.Balance.AccountID, "Alias": op.Alias,
		"Available": "120", "OnHold": "11", "OverdraftUsed": "0", "Version": op.Balance.Version,
		"AccountType": op.Balance.AccountType, "AssetCode": "USD", "Key": op.Balance.Key,
		"AllowSending": 1, "AllowReceiving": 1, "Direction": "credit",
		"AllowOverdraft": 1, "OverdraftLimitEnabled": 1, "OverdraftLimit": limit,
		"BalanceScope": mmodel.BalanceScopeTransactional,
	}
	raw, err := json.Marshal(cached)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(context.Background(), op.InternalKey, raw, time.Hour).Err())

	return string(raw)
}

func readLimitNormalizationCache(t *testing.T, infra *integrationTestInfra, key string) map[string]json.RawMessage {
	t.Helper()
	raw, err := infra.redisContainer.Client.Get(context.Background(), key).Bytes()
	require.NoError(t, err)
	var cached map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &cached))

	return cached
}

type limitNormalizationRedisState struct {
	Payload string
	Expiry  int64
}

func captureLimitNormalizationRedisState(t *testing.T, infra *integrationTestInfra) map[string]limitNormalizationRedisState {
	t.Helper()
	client := infra.redisContainer.Client
	ctx := context.Background()
	keys, err := client.Keys(ctx, "*").Result()
	require.NoError(t, err)
	state := make(map[string]limitNormalizationRedisState, len(keys))
	for _, key := range keys {
		dump, err := client.Dump(ctx, key).Result()
		require.NoError(t, err)
		expiry, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		state[key] = limitNormalizationRedisState{Payload: dump, Expiry: expiry}
	}

	return state
}

func TestIntegration_OverdraftLimitNormalization_ColdAndWarmBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	for _, warm := range []bool{false, true} {
		for _, excess := range []bool{false, true} {
			t.Run(fmt.Sprintf("warm=%t/excess=%t", warm, excess), func(t *testing.T) {
				infra := setupRedisIntegrationInfra(t)
				orgID, ledgerID := uuid.New(), uuid.New()
				amount := int64(1100)
				if warm {
					amount = 1120
				}
				if excess {
					amount++
				}
				op := newLimitNormalizationOperation(orgID, ledgerID, "@limit-boundary", amount)
				if warm {
					seedLimitNormalizationCache(t, infra, op, "1E+3")
				}

				result, err := infra.repo.ProcessBalanceAtomicOperation(context.Background(), orgID, ledgerID,
					uuid.New(), "ACTIVE", false, []mmodel.BalanceOperation{op})
				if excess {
					require.Nil(t, result)
					require.Equal(t, pkg.ValidateBusinessError(constant.ErrOverdraftLimitExceeded, "validateBalance"), err)
					if warm {
						cached := readLimitNormalizationCache(t, infra, op.InternalKey)
						require.JSONEq(t, `"120"`, string(cached["Available"]))
						require.JSONEq(t, `"11"`, string(cached["OnHold"]))
						require.JSONEq(t, `"0"`, string(cached["OverdraftUsed"]))
						require.JSONEq(t, `7`, string(cached["Version"]))
						require.JSONEq(t, `"1000"`, string(cached["OverdraftLimit"]))
					}
					return
				}

				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, result.Before, 1)
				require.Len(t, result.After, 1)
				expectedBefore := int64(100)
				if warm {
					expectedBefore = 120
					require.True(t, result.After[0].OnHold.Equal(decimal.NewFromInt(11)))
				}
				require.True(t, result.Before[0].Available.Equal(decimal.NewFromInt(expectedBefore)))
				require.True(t, result.After[0].Available.IsZero())
				require.True(t, result.After[0].OverdraftUsed.Equal(decimal.NewFromInt(1000)))
				require.EqualValues(t, 8, result.After[0].Version)
				cached := readLimitNormalizationCache(t, infra, op.InternalKey)
				require.JSONEq(t, `"1000"`, string(cached["OverdraftLimit"]))
				require.True(t, op.Balance.Available.Equal(decimal.NewFromInt(100)), "caller snapshot remains unchanged")
			})
		}
	}
}

func TestIntegration_OverdraftLimitNormalization_RepairsWholeBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	infra := setupRedisIntegrationInfra(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	ops := make([]mmodel.BalanceOperation, 0, 5)
	for index, limit := range []string{"1E+3", "1000.000", "+1000", "01000", "10e2"} {
		op := newLimitNormalizationOperation(orgID, ledgerID, fmt.Sprintf("@batch-limit-%d", index), 1120)
		seedLimitNormalizationCache(t, infra, op, limit)
		ops = append(ops, op)
	}

	result, err := infra.repo.ProcessBalanceAtomicOperation(context.Background(), orgID, ledgerID,
		uuid.New(), "ACTIVE", false, ops)
	require.NoError(t, err, "more anomalous keys than repair passes must still repair together")
	require.Len(t, result.After, len(ops))
	for _, op := range ops {
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		require.JSONEq(t, `"1000"`, string(cached["OverdraftLimit"]))
		require.JSONEq(t, `"1000"`, string(cached["OverdraftUsed"]))
		require.JSONEq(t, `8`, string(cached["Version"]))
	}
}

func TestIntegration_OverdraftLimitNormalization_ContinuousCASConflictsExhaustRepairPasses(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	op := newLimitNormalizationOperation(orgID, ledgerID, "@continuous-limit-conflict", 0)
	raw := seedLimitNormalizationCache(t, infra, op, "1E+3")
	raw = strings.TrimSuffix(raw, "}") + `,"UnknownConflict":"0"}`
	client := infra.redisContainer.Client
	require.NoError(t, client.Set(t.Context(), op.InternalKey, raw, time.Minute).Err())
	expires, err := client.Do(t.Context(), "PEXPIRETIME", op.InternalKey).Int64()
	require.NoError(t, err)

	hook := &limitNormalizationCASHook{
		key: op.InternalKey,
		onAttempt: func(ctx context.Context, observed string, attempt int) error {
			concurrent := strings.Replace(
				observed,
				fmt.Sprintf(`"UnknownConflict":"%d"`, attempt-1),
				fmt.Sprintf(`"UnknownConflict":"%d"`, attempt),
				1,
			)
			_, err := client.Eval(ctx, `return redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')`, []string{op.InternalKey}, concurrent).Result()

			return err
		},
	}
	newLimitNormalizationHookedClient(t, infra, hook)

	result, err := infra.repo.ProcessBalanceAtomicOperation(
		t.Context(), orgID, ledgerID, uuid.New(), "ACTIVE", false, []mmodel.BalanceOperation{op},
	)
	require.Nil(t, result)
	require.EqualError(t, err, "balance limit normalization did not converge after 3 passes")
	require.Equal(t, 3, hook.attempts)

	stored := readLimitNormalizationCache(t, infra, op.InternalKey)
	require.JSONEq(t, `"120"`, string(stored["Available"]))
	require.JSONEq(t, `"11"`, string(stored["OnHold"]))
	require.JSONEq(t, `"0"`, string(stored["OverdraftUsed"]))
	require.JSONEq(t, `7`, string(stored["Version"]))
	require.JSONEq(t, `"1E+3"`, string(stored["OverdraftLimit"]))
	require.JSONEq(t, `"3"`, string(stored["UnknownConflict"]))
	require.NotContains(t, stored, "overdraftLimit")
	require.NotContains(t, stored, "SchemaVersion")
	actualExpires, err := client.Do(t.Context(), "PEXPIRETIME", op.InternalKey).Int64()
	require.NoError(t, err)
	require.Equal(t, expires, actualExpires)
}

func TestIntegration_OverdraftLimitNormalization_InvalidBatchIsReadOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	for _, scenario := range []string{"persisted malformed limit", "cached malformed limit", "cached malformed JSON", "cached numeric limit"} {
		t.Run(scenario, func(t *testing.T) {
			infra := setupRedisIntegrationInfra(t)
			orgID, ledgerID := uuid.New(), uuid.New()
			cold := newLimitNormalizationOperation(orgID, ledgerID, "@cold-first", 1)
			repairable := newLimitNormalizationOperation(orgID, ledgerID, "@repairable-middle", 1)
			seedLimitNormalizationCache(t, infra, repairable, "1E+3")
			invalid := newLimitNormalizationOperation(orgID, ledgerID, "@invalid-last", 1)
			switch scenario {
			case "persisted malformed limit":
				*invalid.Balance.Settings.OverdraftLimit = "invalid"
			case "cached malformed limit":
				seedLimitNormalizationCache(t, infra, invalid, "invalid")
			case "cached malformed JSON":
				require.NoError(t, infra.redisContainer.Client.Set(context.Background(), invalid.InternalKey,
					`{"OverdraftLimit":`, time.Hour).Err())
			case "cached numeric limit":
				seedLimitNormalizationCache(t, infra, invalid, 1000)
			}
			before := captureLimitNormalizationRedisState(t, infra)

			result, err := infra.repo.ProcessBalanceAtomicOperation(context.Background(), orgID, ledgerID,
				uuid.New(), "ACTIVE", false, []mmodel.BalanceOperation{cold, repairable, invalid})
			require.Error(t, err)
			require.Nil(t, result)
			require.NotEqual(t, pkg.ValidateBusinessError(constant.ErrOverdraftLimitExceeded, "validateBalance"), err)
			require.NotEqual(t, pkg.ValidateBusinessError(constant.ErrStaleBalanceVersion, "validateBalance"), err)
			require.Equal(t, before, captureLimitNormalizationRedisState(t, infra), "no cold seed, money, expiry, backup or schedule mutation")
		})
	}
}

func TestIntegration_OverdraftLimitNormalization_CanonicalLimitMalformedBatchIsReadOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	for _, scenario := range []struct {
		name, extension string
	}{
		{name: "NaN extension", extension: `,"Extension":NaN`},
		{name: "infinity extension", extension: `,"Extension":Infinity`},
		{name: "hexadecimal extension", extension: `,"Extension":0x10`},
		{name: "leading zero extension", extension: `,"Extension":01`},
		{name: "raw newline", extension: ",\"Extension\":\"line\nbreak\""},
		{name: "duplicate limit", extension: `,"OverdraftLimit":"1000"`},
		{name: "escaped duplicate limit", extension: `,"Overdraft\u004cimit":"1000"`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			infra := setupRedisIntegrationInfra(t)
			orgID, ledgerID := uuid.New(), uuid.New()
			cold := newLimitNormalizationOperation(orgID, ledgerID, "@canonical-cold-first", 1)
			warm := newLimitNormalizationOperation(orgID, ledgerID, "@canonical-warm-middle", 1)
			invalid := newLimitNormalizationOperation(orgID, ledgerID, "@canonical-invalid-last", 1)
			ops := []mmodel.BalanceOperation{cold, warm, invalid}
			for i := range ops {
				ops[i].Amount.Operation = constant.CREDIT
				*ops[i].Balance.Settings.OverdraftLimit = "1000"
			}
			seedLimitNormalizationCache(t, infra, warm, "1000")
			raw := seedLimitNormalizationCache(t, infra, invalid, "1000")
			malformed := strings.TrimSuffix(raw, "}") + scenario.extension + "}"
			require.NoError(t, infra.redisContainer.Client.Set(t.Context(), invalid.InternalKey, malformed, time.Hour).Err())
			before := captureLimitNormalizationRedisState(t, infra)
			_, coldWasPresent := before[cold.InternalKey]
			require.False(t, coldWasPresent)

			result, err := infra.repo.ProcessBalanceAtomicOperation(t.Context(), orgID, ledgerID,
				uuid.New(), "ACTIVE", false, ops)
			require.Nil(t, result)
			require.Error(t, err)
			var serverError redisclient.Error
			require.ErrorAs(t, err, &serverError)
			require.EqualError(t, err, "ERR BALANCE_LIMIT_INVALID")
			for _, sentinel := range []error{constant.ErrInsufficientFunds, constant.ErrOverdraftLimitExceeded, constant.ErrStaleBalanceVersion} {
				require.NotEqual(t, pkg.ValidateBusinessError(sentinel, "validateBalance"), err)
			}
			require.Equal(t, before, captureLimitNormalizationRedisState(t, infra), "the complete batch must preserve bytes, expiries, key absence, backups and schedule")
		})
	}
}

func TestIntegration_OverdraftLimitNormalization_SettingsPatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	infra := setupRedisIntegrationInfra(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	op := newLimitNormalizationOperation(orgID, ledgerID, "@patch-limit", 1)
	cacheKey := "@patch-limit#default"
	op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	seedLimitNormalizationCache(t, infra, op, "100")

	err := infra.repo.UpdateBalanceCacheSettings(context.Background(), orgID, ledgerID, cacheKey, op.Balance.Settings)
	require.NoError(t, err)
	cached := readLimitNormalizationCache(t, infra, op.InternalKey)
	require.JSONEq(t, `"1000"`, string(cached["OverdraftLimit"]))
	require.JSONEq(t, `"120"`, string(cached["Available"]))
	require.JSONEq(t, `"11"`, string(cached["OnHold"]))
	require.JSONEq(t, `7`, string(cached["Version"]))

	before := captureLimitNormalizationRedisState(t, infra)
	*op.Balance.Settings.OverdraftLimit = "invalid"
	err = infra.repo.UpdateBalanceCacheSettings(context.Background(), orgID, ledgerID, cacheKey, op.Balance.Settings)
	require.Error(t, err)
	require.Equal(t, before, captureLimitNormalizationRedisState(t, infra))
}
