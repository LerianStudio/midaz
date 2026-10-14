//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// afterCommandHook runs after every successful command with the given name, with
// the count of such commands so far; it stands in for a writer that slips in
// between a read and the write that depends on it.
type afterCommandHook struct {
	name  string
	calls atomic.Int32
	after func(context.Context, int32) error
}

func (h *afterCommandHook) DialHook(next redisclient.DialHook) redisclient.DialHook { return next }

func (h *afterCommandHook) ProcessHook(next redisclient.ProcessHook) redisclient.ProcessHook {
	return func(ctx context.Context, cmd redisclient.Cmder) error {
		if err := next(ctx, cmd); err != nil {
			return err
		}

		if cmd.Name() != h.name {
			return nil
		}

		return h.after(ctx, h.calls.Add(1))
	}
}

func (h *afterCommandHook) ProcessPipelineHook(next redisclient.ProcessPipelineHook) redisclient.ProcessPipelineHook {
	return next
}

func newHookedConfigRepository(t *testing.T, infra *integrationTestInfra, hook redisclient.Hook) *RedisConsumerRepository {
	t.Helper()

	conn := redistestutil.CreateConnectionWithDB(t, infra.redisContainer.Addr, infra.redisContainer.DB)
	client, err := conn.GetClient(t.Context())
	require.NoError(t, err)
	client.AddHook(hook)

	return &RedisConsumerRepository{conn: conn}
}

type configKeyScope struct {
	orgID    uuid.UUID
	ledgerID uuid.UUID
	ref      string
}

func newConfigKeyScope() configKeyScope {
	return configKeyScope{orgID: uuid.New(), ledgerID: uuid.New(), ref: "@config#default"}
}

func (s configKeyScope) balanceKey() string {
	return utils.BalanceConfigKey(s.orgID, s.ledgerID, s.ref)
}

func (s configKeyScope) blobKey() string {
	return utils.BalanceInternalKey(s.orgID, s.ledgerID, s.ref)
}

func balanceConfigAt(balanceID uuid.UUID, version int64, allowOverdraft bool) accounting.BalanceConfig {
	return accounting.BalanceConfig{
		BalanceID:      balanceID,
		Version:        version,
		AllowSending:   true,
		AllowReceiving: true,
		AllowOverdraft: allowOverdraft,
		OverdraftLimit: decimal.Zero,
		BalanceScope:   mmodel.BalanceScopeTransactional,
	}
}

func storedBalanceConfig(t *testing.T, infra *integrationTestInfra, key string) accounting.BalanceConfig {
	t.Helper()

	raw, err := infra.redisContainer.Client.Get(t.Context(), key).Bytes()
	require.NoError(t, err)

	config, err := balancecache.DecodeBalanceConfig(raw)
	require.NoError(t, err)

	return config
}

func remainingTTL(t *testing.T, infra *integrationTestInfra, key string) time.Duration {
	t.Helper()

	ttl, err := infra.redisContainer.Client.PTTL(t.Context(), key).Result()
	require.NoError(t, err)

	return ttl
}

func TestIntegration_ConfigKey_WritesAbsentKeyWithOneHourExpiry(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	config := balanceConfigAt(uuid.New(), 3, false)

	stored, outcome, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, config)
	require.NoError(t, err)
	assert.Equal(t, ConfigWritten, outcome)
	assert.True(t, stored.Equal(config))
	assert.True(t, storedBalanceConfig(t, infra, scope.balanceKey()).Equal(config))

	ttl := remainingTTL(t, infra, scope.balanceKey())
	assert.Greater(t, ttl, 59*time.Minute)
	assert.LessOrEqual(t, ttl, time.Hour)
}

func TestIntegration_ConfigKey_OlderVersionNeverOverwritesNewer(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID := uuid.New()

	newer := balanceConfigAt(balanceID, 5, false)
	_, _, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, newer)
	require.NoError(t, err)

	stored, outcome, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 4, true))
	require.NoError(t, err)
	assert.Equal(t, ConfigSuperseded, outcome)
	assert.True(t, stored.Equal(newer), "the caller gets the newer stored value back")
	assert.True(t, storedBalanceConfig(t, infra, scope.balanceKey()).Equal(newer))

	newest := balanceConfigAt(balanceID, 6, true)
	_, outcome, err = infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, newest)
	require.NoError(t, err)
	assert.Equal(t, ConfigWritten, outcome, "a newer version replaces the stored one")
	assert.True(t, storedBalanceConfig(t, infra, scope.balanceKey()).Equal(newest))

	// Versions compare as numbers, not as text: 10 is newer than 9.
	tenth := balanceConfigAt(balanceID, 10, false)
	_, outcome, err = infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, tenth)
	require.NoError(t, err)
	assert.Equal(t, ConfigWritten, outcome)

	_, outcome, err = infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 9, true))
	require.NoError(t, err)
	assert.Equal(t, ConfigSuperseded, outcome)
	assert.True(t, storedBalanceConfig(t, infra, scope.balanceKey()).Equal(tenth))
}

// Re-writing what the key already holds is the retry of a PATCH and the repeated
// PATCH some clients send before every transaction: it changes nothing, and above
// all does not give the value another hour.
func TestIntegration_ConfigKey_SameVersionAndContentLeavesKeyAndExpiryUntouched(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	config := balanceConfigAt(uuid.New(), 3, true)

	_, _, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, config)
	require.NoError(t, err)

	// Stand in for a key written half an hour ago.
	require.NoError(t, infra.redisContainer.Client.Expire(t.Context(), scope.balanceKey(), 30*time.Minute).Err())

	stored, outcome, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, config)
	require.NoError(t, err)
	assert.Equal(t, ConfigUnchanged, outcome)
	assert.True(t, stored.Equal(config))
	assert.LessOrEqual(t, remainingTTL(t, infra, scope.balanceKey()), 30*time.Minute, "the expiry must not be renewed")
}

func TestIntegration_ConfigKey_SameVersionWithOtherContentIsRefused(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID := uuid.New()
	original := balanceConfigAt(balanceID, 5, false)

	_, _, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, original)
	require.NoError(t, err)

	_, _, err = infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 5, true))
	require.ErrorIs(t, err, ErrConfigVersionConflict)
	assert.True(t, storedBalanceConfig(t, infra, scope.balanceKey()).Equal(original), "the stored value stays")
}

// A balance deleted and recreated under the same alias#key starts again at version
// 0. The key the deleted balance left, at a higher version, must not win: the new
// balance's configuration replaces it, and reads for the new balance ignore it.
func TestIntegration_ConfigKey_AnotherBalanceIsReplacedWhateverItsVersion(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	deleted := balanceConfigAt(uuid.New(), 5, true)
	recreated := balanceConfigAt(uuid.New(), 0, false)

	_, _, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, deleted)
	require.NoError(t, err)

	values, err := infra.repo.ReadConfigKeys(t.Context(), scope.orgID, scope.ledgerID, []string{scope.ref}, nil)
	require.NoError(t, err)
	require.Contains(t, values.Balances, scope.ref)
	assert.False(t, values.Balances[scope.ref].Describes(recreated.BalanceID), "the deleted balance's key is absent for the recreated one")

	stored, outcome, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, recreated)
	require.NoError(t, err)
	assert.Equal(t, ConfigWritten, outcome)
	assert.True(t, stored.Equal(recreated))
	assert.True(t, storedBalanceConfig(t, infra, scope.balanceKey()).Equal(recreated))
}

func TestIntegration_ConfigKey_ReadsNeverExtendTheExpiry(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	accountID := uuid.New()

	_, _, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(uuid.New(), 1, false))
	require.NoError(t, err)

	_, _, err = infra.repo.WriteAccountConfig(t.Context(), scope.orgID, scope.ledgerID, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: true})
	require.NoError(t, err)

	accountKey := utils.AccountConfigKey(scope.orgID, scope.ledgerID, accountID)
	previousBalance := remainingTTL(t, infra, scope.balanceKey())
	previousAccount := remainingTTL(t, infra, accountKey)

	for range 5 {
		values, readErr := infra.repo.ReadConfigKeys(t.Context(), scope.orgID, scope.ledgerID, []string{scope.ref}, []uuid.UUID{accountID})
		require.NoError(t, readErr)
		require.Len(t, values.Balances, 1)
		require.Len(t, values.Accounts, 1)
		assert.True(t, values.Accounts[accountID].Blocked)

		balanceTTL := remainingTTL(t, infra, scope.balanceKey())
		accountTTL := remainingTTL(t, infra, accountKey)
		assert.LessOrEqual(t, balanceTTL, previousBalance)
		assert.LessOrEqual(t, accountTTL, previousAccount)

		previousBalance, previousAccount = balanceTTL, accountTTL
	}

	assert.LessOrEqual(t, previousBalance, time.Hour)
}

func TestIntegration_ConfigKey_TenantsNeverShareKeys(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	first := tmcore.ContextWithTenantID(t.Context(), "tenant-one")
	second := tmcore.ContextWithTenantID(t.Context(), "tenant-two")

	firstConfig := balanceConfigAt(uuid.New(), 2, true)
	secondConfig := balanceConfigAt(uuid.New(), 9, false)

	_, _, err := infra.repo.WriteBalanceConfig(first, scope.orgID, scope.ledgerID, scope.ref, firstConfig)
	require.NoError(t, err)

	_, outcome, err := infra.repo.WriteBalanceConfig(second, scope.orgID, scope.ledgerID, scope.ref, secondConfig)
	require.NoError(t, err)
	assert.Equal(t, ConfigWritten, outcome, "another tenant's key is another key")

	assert.True(t, storedBalanceConfig(t, infra, "tenant:tenant-one:"+scope.balanceKey()).Equal(firstConfig))
	assert.True(t, storedBalanceConfig(t, infra, "tenant:tenant-two:"+scope.balanceKey()).Equal(secondConfig))

	exists, err := infra.redisContainer.Client.Exists(t.Context(), scope.balanceKey()).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "a tenant-scoped write never lands on the unprefixed key")

	values, err := infra.repo.ReadConfigKeys(first, scope.orgID, scope.ledgerID, []string{scope.ref}, nil)
	require.NoError(t, err)
	assert.True(t, values.Balances[scope.ref].Equal(firstConfig))
}

func TestIntegration_ConfigKey_UnreadableValueReadsAsMissingAndIsReplaced(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.balanceKey(), `{"balanceId":"x","configVersion":"99"}`, time.Hour).Err())

	values, err := infra.repo.ReadConfigKeys(t.Context(), scope.orgID, scope.ledgerID, []string{scope.ref}, nil)
	require.NoError(t, err)
	assert.Empty(t, values.Balances)

	config := balanceConfigAt(uuid.New(), 1, false)
	_, outcome, err := infra.repo.WriteBalanceConfig(t.Context(), scope.orgID, scope.ledgerID, scope.ref, config)
	require.NoError(t, err)
	assert.Equal(t, ConfigWritten, outcome)
	assert.True(t, storedBalanceConfig(t, infra, scope.balanceKey()).Equal(config))
}

func TestIntegration_ConfigKey_AccountKeyFollowsTheSameRule(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	accountID := uuid.New()
	blocked := accounting.AccountConfig{AccountID: accountID, Version: 4, Blocked: true}

	_, outcome, err := infra.repo.WriteAccountConfig(t.Context(), scope.orgID, scope.ledgerID, blocked)
	require.NoError(t, err)
	assert.Equal(t, ConfigWritten, outcome)

	stored, outcome, err := infra.repo.WriteAccountConfig(t.Context(), scope.orgID, scope.ledgerID, accounting.AccountConfig{AccountID: accountID, Version: 3})
	require.NoError(t, err)
	assert.Equal(t, ConfigSuperseded, outcome)
	assert.Equal(t, blocked, stored)

	_, outcome, err = infra.repo.WriteAccountConfig(t.Context(), scope.orgID, scope.ledgerID, blocked)
	require.NoError(t, err)
	assert.Equal(t, ConfigUnchanged, outcome)

	_, _, err = infra.repo.WriteAccountConfig(t.Context(), scope.orgID, scope.ledgerID, accounting.AccountConfig{AccountID: accountID, Version: 4})
	require.ErrorIs(t, err, ErrConfigVersionConflict)

	values, err := infra.repo.ReadConfigKeys(t.Context(), scope.orgID, scope.ledgerID, nil, []uuid.UUID{accountID})
	require.NoError(t, err)
	assert.Equal(t, blocked, values.Accounts[accountID])
}

// configCopyBlob is a dual blob whose money is in exact, non-canonical legacy bytes,
// so any re-encoding of money by the copy would show in a byte comparison.
func configCopyBlob(balanceID, accountID uuid.UUID, available string, version int, copyVersion string) string {
	blob := `{"SchemaVersion":2,"ID":"` + balanceID.String() + `","id":"` + balanceID.String() + `",` +
		`"AccountID":"` + accountID.String() + `","accountId":"` + accountID.String() + `",` +
		`"AccountType":"deposit","accountType":"deposit","AssetCode":"USD","assetCode":"USD",` +
		`"Alias":"@config","alias":"@config","Key":"default","key":"default","Direction":"credit","direction":"credit",` +
		`"BalanceScope":"transactional","balanceScope":"transactional",` +
		`"Available":` + available + `,"available":"` + available + `","OnHold":"0.10","onHold":"0.1",` +
		`"OverdraftUsed":"0","overdraftUsed":"0","OverdraftLimit":"0","overdraftLimit":"0",` +
		`"Version":` + jsonInt(version) + `,"version":"` + jsonInt(version) + `",` +
		`"AllowSending":1,"allowSending":true,"AllowReceiving":1,"allowReceiving":true,"Blocked":0,"blocked":false,` +
		`"AllowOverdraft":0,"allowOverdraft":false,"OverdraftLimitEnabled":0,"overdraftLimitEnabled":false`

	if copyVersion != "" {
		blob += `,"balanceConfigVersion":"` + copyVersion + `"`
	}

	return blob + `}`
}

func jsonInt(value int) string {
	encoded, _ := json.Marshal(value)

	return string(encoded)
}

func blobFields(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &fields))

	return fields
}

func storedBlob(t *testing.T, infra *integrationTestInfra, key string) string {
	t.Helper()

	raw, err := infra.redisContainer.Client.Get(t.Context(), key).Result()
	require.NoError(t, err)

	return raw
}

var moneyAndIdentityFields = []string{
	"ID", "id", "AccountID", "accountId", "Alias", "alias", "Key", "key", "AssetCode", "assetCode",
	"Available", "available", "OnHold", "onHold", "OverdraftUsed", "overdraftUsed", "Version", "version",
	"Blocked", "blocked", "Direction", "direction",
}

func TestIntegration_ConfigCopy_WritesFamilyAndVersionTogether(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID, accountID := uuid.New(), uuid.New()
	before := configCopyBlob(balanceID, accountID, "120.500", 9, "4")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), before, time.Hour).Err())

	config := accounting.BalanceConfig{
		BalanceID: balanceID, Version: 6, AllowSending: false, AllowReceiving: true,
		AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: decimal.RequireFromString("75.25"),
		BalanceScope: mmodel.BalanceScopeTransactional,
	}

	outcome, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, config)
	require.NoError(t, err)
	assert.Equal(t, BlobCopyWritten, outcome)

	after := blobFields(t, storedBlob(t, infra, scope.blobKey()))
	original := blobFields(t, before)

	for _, name := range moneyAndIdentityFields {
		assert.Equal(t, string(original[name]), string(after[name]), "%s must keep its exact bytes", name)
	}

	for name, want := range map[string]string{
		"AllowSending": `0`, "allowSending": `false`, "AllowOverdraft": `1`, "allowOverdraft": `true`,
		"OverdraftLimitEnabled": `1`, "overdraftLimitEnabled": `true`,
		"OverdraftLimit": `"75.25"`, "overdraftLimit": `"75.25"`, "balanceConfigVersion": `"6"`,
	} {
		assert.Equal(t, want, string(after[name]), name)
	}
}

// commandNamesHook records the name of every command the repository sends.
type commandNamesHook struct {
	mu    sync.Mutex
	names []string
}

func (h *commandNamesHook) DialHook(next redisclient.DialHook) redisclient.DialHook { return next }

func (h *commandNamesHook) ProcessHook(next redisclient.ProcessHook) redisclient.ProcessHook {
	return func(ctx context.Context, cmd redisclient.Cmder) error {
		h.mu.Lock()
		h.names = append(h.names, cmd.Name())
		h.mu.Unlock()

		return next(ctx, cmd)
	}
}

func (h *commandNamesHook) ProcessPipelineHook(next redisclient.ProcessPipelineHook) redisclient.ProcessPipelineHook {
	return next
}

func (h *commandNamesHook) sent() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return append([]string(nil), h.names...)
}

// A PATCH repeating the configuration finds the copy of its version already
// holding it: one read, no write, the blob and its expiry untouched.
func TestIntegration_ConfigCopy_SameVersionAndContentOnlyReads(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID := uuid.New()
	blob := configCopyBlob(balanceID, uuid.New(), "50", 3, "4")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), blob, 30*time.Minute).Err())

	hook := &commandNamesHook{}
	repo := newHookedConfigRepository(t, infra, hook)

	outcome, err := repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 4, false))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyAlreadyCurrent, outcome)
	assert.Equal(t, []string{"get"}, hook.sent(), "the copy must only read the blob")
	assert.Equal(t, blob, storedBlob(t, infra, scope.blobKey()))
	assert.LessOrEqual(t, remainingTTL(t, infra, scope.blobKey()), 30*time.Minute, "the blob expiry must not be renewed")
}

// A blob that diverged under the same version (every row starts at version 0, so a
// blob that diverged before versions existed carries the row's version) is healed.
func TestIntegration_ConfigCopy_SameVersionWithOtherContentIsHealed(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID := uuid.New()
	diverged := configCopyBlob(balanceID, uuid.New(), "50", 3, "")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), diverged, time.Hour).Err())

	outcome, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 0, true))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyWritten, outcome)

	after := blobFields(t, storedBlob(t, infra, scope.blobKey()))
	assert.Equal(t, `1`, string(after["AllowOverdraft"]))
	assert.Equal(t, `true`, string(after["allowOverdraft"]))
	assert.Equal(t, `"0"`, string(after["balanceConfigVersion"]))
	assert.Equal(t, `50`, string(after["Available"]), "money keeps its exact bytes")
}

// Copies of two PATCHes can land out of order. The later one to arrive carries
// the older version and must leave the newer, complete copy in place.
func TestIntegration_ConfigCopy_OlderCopyNeverOverwritesNewer(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID := uuid.New()
	newer := configCopyBlob(balanceID, uuid.New(), "50", 3, "6")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), newer, time.Hour).Err())

	outcome, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 5, true))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyAlreadyCurrent, outcome)
	assert.Equal(t, newer, storedBlob(t, infra, scope.blobKey()), "the blob must not change")
}

func TestIntegration_ConfigCopy_LegacyMoneyBytesSurvive(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID, accountID := uuid.New(), uuid.New()
	legacy := `{"ID":"` + balanceID.String() + `","AccountID":"` + accountID.String() + `","AccountType":"deposit","AssetCode":"USD",` +
		`"Alias":"0#@config#default","Key":"default","Available":120.500,"OnHold":1E1,"Version":9,` +
		`"AllowSending":1,"AllowReceiving":1,"Direction":"credit","OverdraftUsed":"0","AllowOverdraft":0,` +
		`"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), legacy, time.Hour).Err())

	outcome, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 1, true))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyWritten, outcome)

	stored := storedBlob(t, infra, scope.blobKey())
	assert.Contains(t, stored, `"Available":120.500`)
	assert.Contains(t, stored, `"OnHold":1E1`)
	assert.Contains(t, stored, `"Version":9`)
	assert.Contains(t, stored, `"Alias":"0#@config#default"`)
	assert.Contains(t, stored, `"AllowOverdraft":1`)
	assert.NotContains(t, stored, `"allowOverdraft"`, "a legacy blob keeps its shape")
}

// A transaction that moves money between the copy's read and its write must never
// be undone by the copy: the compare-and-set on the exact bytes refuses, the copy
// reads again and applies the configuration on top of the transaction's money.
func TestIntegration_ConfigCopy_TransactionBetweenReadAndWriteKeepsItsMoney(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID, accountID := uuid.New(), uuid.New()
	before := configCopyBlob(balanceID, accountID, "120", 9, "")
	moved := configCopyBlob(balanceID, accountID, "119.990", 10, "")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), before, time.Hour).Err())

	hook := &afterCommandHook{name: "get"}
	hook.after = func(ctx context.Context, call int32) error {
		if call != 1 {
			return nil
		}

		return infra.redisContainer.Client.SetArgs(ctx, scope.blobKey(), moved, redisclient.SetArgs{KeepTTL: true}).Err()
	}

	repo := newHookedConfigRepository(t, infra, hook)

	outcome, err := repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 2, true))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyWritten, outcome)
	assert.Equal(t, int32(2), hook.calls.Load(), "the refused compare-and-set must force exactly one fresh read")

	after := blobFields(t, storedBlob(t, infra, scope.blobKey()))
	transaction := blobFields(t, moved)

	for _, name := range moneyAndIdentityFields {
		assert.Equal(t, string(transaction[name]), string(after[name]), "%s must be the transaction's bytes", name)
	}

	assert.Equal(t, `1`, string(after["AllowOverdraft"]))
	assert.Equal(t, `true`, string(after["allowOverdraft"]))
	assert.Equal(t, `"2"`, string(after["balanceConfigVersion"]))

	snapshot, err := balancecache.DecodeForRead([]byte(storedBlob(t, infra, scope.blobKey())))
	require.NoError(t, err)
	assert.True(t, snapshot.Available.Equal(decimal.RequireFromString("119.99")))
	assert.Equal(t, int64(10), snapshot.Version)
}

// A blob that changes on every attempt ends the copy as a conflict, without the copy
// ever writing over the latest money.
func TestIntegration_ConfigCopy_ConflictsExhaustWithoutWriting(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID, accountID := uuid.New(), uuid.New()
	moves := []string{
		configCopyBlob(balanceID, accountID, "99", 10, ""),
		configCopyBlob(balanceID, accountID, "98", 11, ""),
		configCopyBlob(balanceID, accountID, "97", 12, ""),
	}

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), configCopyBlob(balanceID, accountID, "100", 9, ""), time.Hour).Err())

	hook := &afterCommandHook{name: "get"}
	hook.after = func(ctx context.Context, call int32) error {
		return infra.redisContainer.Client.SetArgs(ctx, scope.blobKey(), moves[call-1], redisclient.SetArgs{KeepTTL: true}).Err()
	}

	repo := newHookedConfigRepository(t, infra, hook)

	outcome, err := repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 2, true))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyConflict, outcome)
	assert.False(t, outcome.Confirmed())
	assert.Equal(t, moves[2], storedBlob(t, infra, scope.blobKey()), "the latest money stays exactly as written")
}

func TestIntegration_ConfigCopy_AbsentBlobIsNotCreated(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()

	outcome, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(uuid.New(), 2, true))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyBlobAbsent, outcome)
	assert.True(t, outcome.Confirmed())

	exists, err := infra.redisContainer.Client.Exists(t.Context(), scope.blobKey()).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
}

// A blob left under the alias#key by a balance deleted and recreated under the same
// alias#key belongs to another balance: the engine refuses it, so there is nothing
// to confirm, and the copy leaves it alone without failing.
func TestIntegration_ConfigCopy_BlobOfAnotherBalanceHasNothingToConfirm(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	accountID := uuid.New()
	deleted := configCopyBlob(uuid.New(), accountID, "10", 1, "9")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), deleted, time.Hour).Err())

	outcome, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(uuid.New(), 1, true))
	require.NoError(t, err)
	assert.Equal(t, BlobCopyOfAnotherBalance, outcome)
	assert.True(t, outcome.Confirmed())
	assert.Equal(t, deleted, storedBlob(t, infra, scope.blobKey()), "the other balance's blob must not change")

	otherRef := "@config#savings"
	otherKey := utils.BalanceInternalKey(scope.orgID, scope.ledgerID, otherRef)
	stale := configCopyBlob(uuid.New(), accountID, "20", 2, "")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), otherKey, stale, time.Hour).Err())

	outcomes, err := infra.repo.CopyAccountConfigToBlobs(t.Context(), scope.orgID, scope.ledgerID, []string{scope.ref, otherRef},
		accounting.AccountConfig{AccountID: uuid.New(), Version: 3, Blocked: true})
	require.NoError(t, err, "a blob of another account must not stop the copy")
	assert.Equal(t, map[string]BlobCopyOutcome{scope.ref: BlobCopyOfAnotherBalance, otherRef: BlobCopyOfAnotherBalance}, outcomes)

	sameAccount, err := infra.repo.CopyAccountConfigToBlobs(t.Context(), scope.orgID, scope.ledgerID, []string{scope.ref, otherRef},
		accounting.AccountConfig{AccountID: accountID, Version: 3, Blocked: true})
	require.NoError(t, err)
	assert.Equal(t, map[string]BlobCopyOutcome{scope.ref: BlobCopyWritten, otherRef: BlobCopyWritten}, sameAccount)
}

func TestIntegration_ConfigCopy_DeletedBalanceIsRefused(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	scope := newConfigKeyScope()
	balanceID := uuid.New()
	blob := configCopyBlob(balanceID, uuid.New(), "10", 1, "")

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), blob, time.Hour).Err())

	marker := "balance_delete_marker:{transactions}:" + scope.orgID.String() + ":" + scope.ledgerID.String() + ":" + scope.ref
	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), marker, "1", time.Minute).Err())

	_, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(balanceID, 2, true))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0019")
	assert.Equal(t, blob, storedBlob(t, infra, scope.blobKey()))
}

// A blob the copy cannot read takes no copy: the copy fails, which fails the PATCH
// as a technical error, and the unreadable bytes stay exactly as they were.
func TestIntegration_ConfigCopy_UnreadableBlobFailsAndIsLeftIntact(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)

	for name, unreadable := range map[string]string{
		"not JSON":          "not-json",
		"truncated JSON":    `{"ID":"` + uuid.NewString() + `","Available":"10"`,
		"not a JSON object": `["balance"]`,
	} {
		t.Run(name, func(t *testing.T) {
			scope := newConfigKeyScope()

			require.NoError(t, infra.redisContainer.Client.Set(t.Context(), scope.blobKey(), unreadable, time.Hour).Err())

			_, err := infra.repo.CopyBalanceConfigToBlob(t.Context(), scope.orgID, scope.ledgerID, scope.ref, balanceConfigAt(uuid.New(), 2, true))
			require.Error(t, err)
			assert.Equal(t, unreadable, storedBlob(t, infra, scope.blobKey()))

			_, err = infra.repo.CopyAccountConfigToBlobs(t.Context(), scope.orgID, scope.ledgerID, []string{scope.ref},
				accounting.AccountConfig{AccountID: uuid.New(), Version: 2, Blocked: true})
			require.Error(t, err)
			assert.Equal(t, unreadable, storedBlob(t, infra, scope.blobKey()))
		})
	}
}

// The account copy reads every blob of the account in one MGET and writes only the
// ones holding an older copy; a transaction on one of them between the MGET and its
// write is kept, exactly as for a single balance.
func TestIntegration_AccountConfigCopy_WritesOnlyStaleBlobsAndKeepsTheirMoney(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	staleRef, currentRef, absentRef := "@account#default", "@account#savings", "@account#overdraft"
	staleKey := utils.BalanceInternalKey(orgID, ledgerID, staleRef)
	currentKey := utils.BalanceInternalKey(orgID, ledgerID, currentRef)

	staleID := uuid.New()
	moved := configCopyBlob(staleID, accountID, "40.50", 3, "")
	current := `{"SchemaVersion":2,"id":"` + uuid.New().String() + `","accountId":"` + accountID.String() + `","accountType":"deposit","assetCode":"USD","alias":"@account","key":"savings","available":"1","onHold":"0","version":"1","allowSending":true,"allowReceiving":true,"blocked":true,"accountConfigVersion":"9"}`

	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), staleKey, configCopyBlob(staleID, accountID, "41", 2, ""), time.Hour).Err())
	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), currentKey, current, time.Hour).Err())

	hook := &afterCommandHook{name: "mget"}
	hook.after = func(ctx context.Context, call int32) error {
		if call != 1 {
			return nil
		}

		return infra.redisContainer.Client.SetArgs(ctx, staleKey, moved, redisclient.SetArgs{KeepTTL: true}).Err()
	}

	repo := newHookedConfigRepository(t, infra, hook)
	config := accounting.AccountConfig{AccountID: accountID, Version: 9, Blocked: true}

	outcomes, err := repo.CopyAccountConfigToBlobs(t.Context(), orgID, ledgerID, []string{staleRef, currentRef, absentRef}, config)
	require.NoError(t, err)
	assert.Equal(t, map[string]BlobCopyOutcome{
		staleRef:   BlobCopyWritten,
		currentRef: BlobCopyAlreadyCurrent,
		absentRef:  BlobCopyBlobAbsent,
	}, outcomes)

	after := blobFields(t, storedBlob(t, infra, staleKey))
	transaction := blobFields(t, moved)

	for _, name := range []string{"Available", "available", "Version", "version", "AllowOverdraft", "allowOverdraft", "balanceConfigVersion"} {
		assert.Equal(t, string(transaction[name]), string(after[name]), "%s must be the transaction's bytes", name)
	}

	assert.Equal(t, `1`, string(after["Blocked"]))
	assert.Equal(t, `true`, string(after["blocked"]))
	assert.Equal(t, `"9"`, string(after["accountConfigVersion"]))
	assert.Equal(t, current, storedBlob(t, infra, currentKey), "a current blob is not written")
}
