//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// newOverdraftConfigFixture builds a cached primary holding less than the debit
// and its cached overdraft companion: the debit commits only through an overdraft
// draw, so the configuration the engine decides with is visible in the outcome.
func newOverdraftConfigFixture(t *testing.T, client redis.UniversalClient) *integrationFixture {
	t.Helper()

	f := newIntegrationFixture(t, client)
	f.input.Execution.Balances[0].Available = decimal.NewFromInt(10)
	f.addCompanion("0")
	f.seed(t, 0, f.input.Execution.Balances[0])
	f.seed(t, 1, f.input.Execution.Balances[1])

	return f
}

// seedWithCopyVersions caches the balance at index as this release writes it: the
// configuration copy carries the version of each family, version 0 included, as a
// PATCH copy writes it. seed leaves a version-0 family without a version field, as
// a blob an earlier release wrote.
func (f *integrationFixture) seedWithCopyVersions(t *testing.T, index int, snapshot accounting.BalanceSnapshot) {
	t.Helper()
	encoded, err := balancecache.Encode(snapshot, balancecache.FormatDual)
	require.NoError(t, err)

	key := f.resolved.Balances[f.input.Execution.Balances[index].BalanceRef].Balance
	require.NoError(t, f.client.Set(context.Background(), key, withCopyVersions(t, encoded, snapshot.ConfigVersion, snapshot.AccountConfigVersion), time.Hour).Err())
}

// withCopyVersions sets both configuration copy version fields of an encoded blob.
func withCopyVersions(t *testing.T, encoded []byte, balanceVersion, accountVersion int64) []byte {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	fields[balancecache.BlobBalanceConfigVersionField] = json.RawMessage(strconv.Quote(strconv.FormatInt(balanceVersion, 10)))
	fields[balancecache.BlobAccountConfigVersionField] = json.RawMessage(strconv.Quote(strconv.FormatInt(accountVersion, 10)))

	return mustMarshal(t, fields)
}

func (f *integrationFixture) blobFields(t *testing.T, index int) map[string]json.RawMessage {
	t.Helper()

	raw, err := f.client.Get(context.Background(), f.resolved.Balances[f.input.Execution.Balances[index].BalanceRef].Balance).Bytes()
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))

	return fields
}

func (f *integrationFixture) storedKey(t *testing.T, key string) integrationStoredKey {
	t.Helper()

	dump, err := f.client.Dump(context.Background(), key).Result()
	exists := err == nil
	if !exists {
		require.ErrorIs(t, err, redis.Nil)
	}

	expiry, err := f.client.Do(context.Background(), "PEXPIRETIME", key).Int64()
	require.NoError(t, err)

	return integrationStoredKey{Exists: exists, Dump: dump, Expiry: expiry}
}

func (f *integrationFixture) balanceConfigKey(index int) string {
	return f.resolved.Balances[f.input.Execution.Balances[index].BalanceRef].Config
}

func (f *integrationFixture) accountConfigKey(t *testing.T) string {
	t.Helper()
	f.syncAccountProtection(t)

	return f.resolved.Accounts[f.input.Execution.Balances[0].AccountID].Config
}

// requireBlobConfiguration asserts the configuration copy a committed execution
// left in the blob, in both cache representations, with the versions it decided.
func requireBlobConfiguration(t *testing.T, fields map[string]json.RawMessage, allowOverdraft bool, balanceVersion, accountVersion string) {
	t.Helper()

	legacy := "0"
	if allowOverdraft {
		legacy = "1"
	}

	require.Equal(t, strconv.FormatBool(allowOverdraft), string(fields["allowOverdraft"]))
	require.Equal(t, legacy, string(fields["AllowOverdraft"]))

	requireBlobVersion(t, fields, balancecache.BlobBalanceConfigVersionField, balanceVersion)
	requireBlobVersion(t, fields, balancecache.BlobAccountConfigVersionField, accountVersion)
}

func requireBlobVersion(t *testing.T, fields map[string]json.RawMessage, field, version string) {
	t.Helper()

	if version == "" {
		require.NotContains(t, fields, field, "a version-0 decision leaves the blob without version fields")

		return
	}

	require.Equal(t, strconv.Quote(version), string(fields[field]))
}

func TestIntegrationEngineConfigKeyDecidesOverTheBlobCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	t.Run("a refusal by the key's configuration writes nothing", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		primary := f.input.Execution.Balances[0]
		primary.AllowOverdraft = false
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(primary, 1))
		before := f.capture(t)

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`, "the key turned the overdraft off; the blob and the request still allow it")
		require.Equal(t, before, f.capture(t), "a refused execution writes no blob, no copy and no configuration key")
	})

	t.Run("an approved execution copies the key's configuration and both versions", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.seedWithCopyVersions(t, 0, f.input.Execution.Balances[0])
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(4)
		primary := f.input.Execution.Balances[0]
		primary.AllowOverdraft = false
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(primary, 1))
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: primary.AccountID, Version: 3})
		balanceKey, accountKey := f.storedKey(t, f.balanceConfigKey(0)), f.storedKey(t, f.accountConfigKey(t))

		raw, err := f.run(t)
		require.NoError(t, err)

		result := decodeIntegrationResult(t, raw)
		require.Equal(t, integrationState{Available: "6", OnHold: "0", OverdraftUsed: "0", Version: "1"}, finalState(result.Final[0]))
		require.False(t, result.Final[0].AllowOverdraft, "the result reports the configuration the execution decided with")

		fields := f.blobFields(t, 0)
		requireBlobConfiguration(t, fields, false, "1", "3")
		require.Equal(t, `"6"`, string(fields["Available"]))
		require.Equal(t, balanceKey, f.storedKey(t, f.balanceConfigKey(0)), "the engine never writes nor renews a configuration key")
		require.Equal(t, accountKey, f.storedKey(t, f.accountConfigKey(t)), "the engine never writes nor renews a configuration key")
	})

	t.Run("a newer key decides over an older request and is left untouched", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.seedWithCopyVersions(t, 0, f.input.Execution.Balances[0])
		f.input.Execution.Balances[0].AllowOverdraft = false
		f.input.Execution.Balances[0].ConfigVersion = 4
		live := f.input.Execution.Balances[0]
		live.AllowOverdraft = true
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(live, 5))
		key := f.storedKey(t, f.balanceConfigKey(0))

		raw, err := f.run(t)
		require.NoError(t, err, "the key's version 5 allows the draw the request's version 4 forbids")

		result := decodeIntegrationResult(t, raw)
		require.Len(t, result.Movements, 2)
		require.Equal(t, "20", result.Final[0].OverdraftUsed)
		requireBlobConfiguration(t, f.blobFields(t, 0), true, "5", "0")
		require.Equal(t, key, f.storedKey(t, f.balanceConfigKey(0)))
	})

	t.Run("versions compare as numbers, not as text", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Balances[0].ConfigVersion = 9
		live := f.input.Execution.Balances[0]
		live.AllowOverdraft = false
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(live, 10))

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`, "version 10 is newer than version 9")
	})

	t.Run("an older key loses to a newer request", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		// Go read version 4 from the primary after the key it found was replaced;
		// the key still holds version 3, which allows the overdraft.
		f.input.Execution.Balances[0].ConfigVersion = 4
		stale := f.input.Execution.Balances[0]
		f.input.Execution.Balances[0].AllowOverdraft = false
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(stale, 3))
		before := f.capture(t)

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`, "version 4 of the request decides over version 3 of the key")
		require.Equal(t, before, f.capture(t))
	})

	t.Run("an older key loses to a newer blob copy, which an approval keeps", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		// A PATCH confirmed version 5 in the blob; the key and the request still
		// hold version 4, which allows the overdraft.
		f.input.Execution.Balances[0].ConfigVersion = 4
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(f.input.Execution.Balances[0], 4))
		copied := f.input.Execution.Balances[0]
		copied.AllowOverdraft, copied.ConfigVersion = false, 5
		f.seed(t, 0, copied)
		before := f.capture(t)

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`, "version 5 of the blob copy decides over version 4 of the key")
		require.Equal(t, before, f.capture(t))

		f.rotateExecution()
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(4)

		_, err = f.run(t)
		require.NoError(t, err)
		requireBlobConfiguration(t, f.blobFields(t, 0), false, "5", "")
	})

	t.Run("an equal version prefers the request over the blob copy", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		// The blob's version-0 copy still allows the overdraft; the request read
		// the row, at the same version, without it.
		f.input.Execution.Balances[0].AllowOverdraft = false

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`)
	})

	t.Run("an equal version prefers the key over the request", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Balances[0].ConfigVersion = 2
		live := f.input.Execution.Balances[0]
		live.AllowOverdraft = false
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(live, 2))

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`)
	})
}

func TestIntegrationEngineConfigKeyIsOnlyRead(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	t.Run("a key gone before the execution leaves the request in force for that execution only", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.seedWithCopyVersions(t, 0, f.input.Execution.Balances[0])
		// Go read version 3 from the key; the key expired before the script ran.
		f.input.Execution.Balances[0].AllowOverdraft = false
		f.input.Execution.Balances[0].ConfigVersion = 3
		f.input.Execution.Balances[0].AccountConfigVersion = 2
		before := f.capture(t)

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`, "the request's version 3 decides over the blob's version-0 copy")
		require.Equal(t, before, f.capture(t))

		f.rotateExecution()
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(4)

		_, err = f.run(t)
		require.NoError(t, err)
		requireBlobConfiguration(t, f.blobFields(t, 0), false, "3", "2")
		for _, key := range []string{f.balanceConfigKey(0), f.balanceConfigKey(1), f.accountConfigKey(t)} {
			require.Zero(t, f.client.Exists(context.Background(), key).Val(), "the engine never recreates a configuration key")
		}
	})

	t.Run("a key keeps the absolute expiry Go gave it across executions", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(1)
		f.liveConfiguration(t, 0, f.input.Execution.Balances[0], 1)
		balanceKey, accountKey := f.storedKey(t, f.balanceConfigKey(0)), f.storedKey(t, f.accountConfigKey(t))
		require.Positive(t, balanceKey.Expiry)

		for range 3 {
			_, err := f.run(t)
			require.NoError(t, err)
			f.rotateExecution()
		}

		require.Equal(t, balanceKey, f.storedKey(t, f.balanceConfigKey(0)))
		require.Equal(t, accountKey, f.storedKey(t, f.accountConfigKey(t)))
	})
}

func TestIntegrationEngineBlobCopyNeverMovesBack(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	// A PATCH confirmed version 5 in the blob after Go composed the request with
	// version 4, and the key expired before the script ran.
	confirmed := func(t *testing.T) *integrationFixture {
		t.Helper()

		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Balances[0].ConfigVersion = 4
		copied := f.input.Execution.Balances[0]
		copied.AllowOverdraft, copied.ConfigVersion = false, 5
		f.seed(t, 0, copied)

		return f
	}

	t.Run("the newer blob copy decides", func(t *testing.T) {
		f := confirmed(t)
		before := f.capture(t)

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"insufficient_funds"`)
		require.Equal(t, before, f.capture(t))
	})

	t.Run("an approved execution keeps the newer copy", func(t *testing.T) {
		f := confirmed(t)
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(4)

		_, err := f.run(t)
		require.NoError(t, err)
		requireBlobConfiguration(t, f.blobFields(t, 0), false, "5", "")
	})
}

// TestIntegrationEngineKeepsTheBlobCopyOnAnEqualVersion pins what an approved
// execution writes when the version it decided with is not newer than the copy in
// the blob. A release that reads the configuration from the blob changes the
// copy's content but not its version label, so a key at that same version can
// hold the content the change replaced. The key still decides; the blob keeps its
// copy, so a rollback still obeys the change. Money moves as usual.
func TestIntegrationEngineKeepsTheBlobCopyOnAnEqualVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	t.Run("an earlier release's configuration change survives, and a newer version is still copied", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Transactions[0].RejectBlockedBalances = true
		for index := range f.input.Execution.Balances {
			f.input.Execution.Balances[index].AccountConfigVersion = 1
		}
		primary := &f.input.Execution.Balances[0]
		primary.AllowSending, primary.AllowReceiving, primary.AllowOverdraft = true, true, true
		primary.OverdraftLimitEnabled, primary.OverdraftLimit, primary.BalanceScope = false, decimal.Zero, "transactional"
		primary.ConfigVersion = 1
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(*primary, 1))
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: primary.AccountID, Version: 1, Blocked: false})
		// The earlier release changed every field of both families in the blobs and
		// left the version fields it does not know as they were.
		changed := *primary
		changed.AllowSending, changed.AllowReceiving, changed.AllowOverdraft = false, false, false
		changed.OverdraftLimitEnabled, changed.OverdraftLimit, changed.BalanceScope = true, decimal.NewFromInt(50), "internal"
		changed.Blocked = true
		f.seedWithCopyVersions(t, 0, changed)
		companion := f.input.Execution.Balances[1]
		companion.Blocked = true
		f.seedWithCopyVersions(t, 1, companion)
		before := blobConfigCopy(f.blobFields(t, 0))

		raw, err := f.run(t)
		require.NoError(t, err, "the keys at the same versions allow the draw and do not block the account")

		result := decodeIntegrationResult(t, raw)
		require.Equal(t, "20", result.Final[0].OverdraftUsed)
		require.False(t, result.Final[0].Blocked, "the result reports the configuration the execution decided with")
		fields := f.blobFields(t, 0)
		require.Equal(t, before, blobConfigCopy(fields), "the blob keeps every field and version of both copies")
		require.Equal(t, strconv.Quote(result.Final[0].Available), string(fields["Available"]), "money moves as usual")
		require.Equal(t, strconv.Quote(result.Final[0].OverdraftUsed), string(fields["OverdraftUsed"]))
		require.Equal(t, result.Final[0].Version, string(fields["Version"]))

		// Go reloaded the balance row at a newer version: that family's copy follows
		// it, and the account's copy, still at the key's version, stays.
		f.rotateExecution()
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(1)
		f.input.Execution.Balances[0].ConfigVersion = 2
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(f.input.Execution.Balances[0], 2))

		_, err = f.run(t)
		require.NoError(t, err)
		fields = f.blobFields(t, 0)
		requireBlobConfiguration(t, fields, true, "2", "1")
		copied, err := balancecache.Decode(mustMarshal(t, fields))
		require.NoError(t, err)
		want, got := snapshotBalanceConfig(f.input.Execution.Balances[0], 2), snapshotBalanceConfig(copied, 2)
		require.True(t, want.Equal(got), "the newer version is copied whole: want %+v, got %+v", want, got)
		require.Equal(t, "1", string(fields["Blocked"]), "the account's copy at the key's version stays")
	})

	t.Run("an earlier release's account block survives in every blob of the account", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Transactions[0].RejectBlockedBalances = true
		for index := range f.input.Execution.Balances {
			f.input.Execution.Balances[index].AccountConfigVersion = 1
		}
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: f.input.Execution.Balances[0].AccountID, Version: 1, Blocked: false})
		// The earlier release blocked the account in both blobs and left their account
		// version, the key's, as it was.
		before := make([]map[string]string, len(f.input.Execution.Balances))
		for index := range f.input.Execution.Balances {
			blocked := f.input.Execution.Balances[index]
			blocked.Blocked = true
			f.seedWithCopyVersions(t, index, blocked)
			before[index] = blobConfigCopy(f.blobFields(t, index))
		}

		raw, err := f.run(t)
		require.NoError(t, err, "the key at the same version decides the account is not blocked")

		result := decodeIntegrationResult(t, raw)
		require.Len(t, result.Final, 2)
		require.False(t, result.Final[0].Blocked, "the result reports the configuration the execution decided with")

		available := make(map[string]string, len(result.Final))
		for _, final := range result.Final {
			available[final.BalanceRef] = final.Available
		}

		for index, balance := range f.input.Execution.Balances {
			fields := f.blobFields(t, index)
			require.Equal(t, before[index], blobConfigCopy(fields), "the blob keeps the block the earlier release wrote")
			require.Equal(t, "1", string(fields["Blocked"]))
			require.Equal(t, strconv.Quote(available[balance.BalanceRef]), string(fields["Available"]), "money moves as usual")
		}
	})
}

// blobConfigCopyFields names every field of the configuration copy in a blob: each
// family in both cache representations, and the version of each family.
var blobConfigCopyFields = []string{
	"AllowSending", "allowSending", "AllowReceiving", "allowReceiving",
	"AllowOverdraft", "allowOverdraft", "OverdraftLimitEnabled", "overdraftLimitEnabled",
	"OverdraftLimit", "overdraftLimit", "BalanceScope", "balanceScope",
	balancecache.BlobBalanceConfigVersionField,
	"Blocked", "blocked",
	balancecache.BlobAccountConfigVersionField,
}

// blobConfigCopy returns the raw bytes of every configuration copy field of a blob;
// a field the blob does not have is left out.
func blobConfigCopy(fields map[string]json.RawMessage) map[string]string {
	copied := make(map[string]string, len(blobConfigCopyFields))

	for _, name := range blobConfigCopyFields {
		if raw, exists := fields[name]; exists {
			copied[name] = string(raw)
		}
	}

	return copied
}

// TestIntegrationEngineKeepsABlobCopyWithoutAVersion pins what an approved
// execution writes into a blob whose copy of a family has no version field: a blob
// an earlier release seeded from scratch, or one written before versions existed.
// Its copy may be newer than a live key, as when an earlier pod seeds the blob after
// a PATCH it served, so the execution decides as usual but keeps the copy, fields
// and absent version alike. Each family is judged on its own.
func TestIntegrationEngineKeepsABlobCopyWithoutAVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	t.Run("an earlier release's account block survives an older key in every blob of the account", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Transactions[0].RejectBlockedBalances = true
		accountID := f.input.Execution.Balances[0].AccountID
		for index := range f.input.Execution.Balances {
			f.input.Execution.Balances[index].AccountConfigVersion = 1
		}
		// A current pod wrote version 1 (not blocked) to the key; an earlier pod then
		// blocked the account and seeded both blobs from the row, without versions.
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: false})
		before := make([]map[string]string, len(f.input.Execution.Balances))
		for index := range f.input.Execution.Balances {
			blocked := f.input.Execution.Balances[index]
			blocked.Blocked, blocked.AccountConfigVersion = true, 0
			f.seed(t, index, blocked)
			before[index] = blobConfigCopy(f.blobFields(t, index))
			require.Equal(t, "1", before[index]["Blocked"])
			require.NotContains(t, before[index], balancecache.BlobAccountConfigVersionField)
		}

		raw, err := f.run(t)
		require.NoError(t, err, "the key's version 1 decides the account is not blocked")

		result := decodeIntegrationResult(t, raw)
		require.Len(t, result.Final, 2)
		require.False(t, result.Final[0].Blocked, "the result reports the configuration the execution decided with")

		available := make(map[string]string, len(result.Final))
		for _, final := range result.Final {
			available[final.BalanceRef] = final.Available
		}

		for index, balance := range f.input.Execution.Balances {
			fields := f.blobFields(t, index)
			require.Equal(t, before[index], blobConfigCopy(fields), "the blob keeps the copy the earlier release wrote, without a version")
			require.Equal(t, strconv.Quote(available[balance.BalanceRef]), string(fields["Available"]), "money moves as usual")
		}
	})

	t.Run("an earlier release's balance configuration survives an older key", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		primary := &f.input.Execution.Balances[0]
		primary.AllowSending, primary.AllowReceiving, primary.AllowOverdraft = true, true, true
		primary.OverdraftLimitEnabled, primary.OverdraftLimit, primary.BalanceScope = false, decimal.Zero, "transactional"
		primary.ConfigVersion = 1
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(*primary, 1))
		// The earlier release changed every field of the family and seeded the blob
		// without a version.
		changed := *primary
		changed.AllowSending, changed.AllowReceiving, changed.AllowOverdraft = false, false, false
		changed.OverdraftLimitEnabled, changed.OverdraftLimit, changed.BalanceScope = true, decimal.NewFromInt(50), "internal"
		changed.ConfigVersion = 0
		f.seed(t, 0, changed)
		before := blobConfigCopy(f.blobFields(t, 0))
		require.NotContains(t, before, balancecache.BlobBalanceConfigVersionField)

		raw, err := f.run(t)
		require.NoError(t, err, "the key's version 1 allows the draw")

		result := decodeIntegrationResult(t, raw)
		require.Equal(t, "20", result.Final[0].OverdraftUsed)
		fields := f.blobFields(t, 0)
		require.Equal(t, before, blobConfigCopy(fields), "the blob keeps the copy the earlier release wrote, without a version")
		require.Equal(t, strconv.Quote(result.Final[0].Available), string(fields["Available"]), "money moves as usual")
		require.Equal(t, strconv.Quote(result.Final[0].OverdraftUsed), string(fields["OverdraftUsed"]))
	})

	t.Run("a copy this release wrote at version 0 is replaced by a newer decision", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Balances[0].ConfigVersion = 1
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(f.input.Execution.Balances[0], 1))
		stale := f.input.Execution.Balances[0]
		stale.AllowOverdraft, stale.ConfigVersion = false, 0
		f.seedWithCopyVersions(t, 0, stale)
		require.Equal(t, `"0"`, string(f.blobFields(t, 0)[balancecache.BlobBalanceConfigVersionField]))

		_, err := f.run(t)
		require.NoError(t, err)
		requireBlobConfiguration(t, f.blobFields(t, 0), true, "1", "0")
	})
}

func TestIntegrationEngineConfigKeyOfAnotherBalanceIsAbsent(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newOverdraftConfigFixture(t, container.Client)
	// The key a deleted balance left under the same alias#key, newer than anything
	// the recreated balance has.
	deleted := f.input.Execution.Balances[0]
	deleted.ID, deleted.AllowOverdraft = uuid.MustParse("7d1f4c2a-9b3e-4f5a-8c6d-0e1f2a3b4c5d"), false
	f.writeBalanceConfig(t, 0, snapshotBalanceConfig(deleted, 9))
	key := f.storedKey(t, f.balanceConfigKey(0))

	raw, err := f.run(t)
	require.NoError(t, err, "the key of another balance is ignored and the request allows the draw")
	require.Equal(t, "20", decodeIntegrationResult(t, raw).Final[0].OverdraftUsed)
	requireBlobConfiguration(t, f.blobFields(t, 0), true, "", "")
	require.Equal(t, key, f.storedKey(t, f.balanceConfigKey(0)), "only Go replaces the key of another balance")
}

func TestIntegrationEngineAccountConfigKeyBlocksEveryBalanceOfTheAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	for _, ref := range []string{"@source#default", companionRef} {
		t.Run("a requirement on "+ref, func(t *testing.T) {
			f := newOverdraftConfigFixture(t, container.Client)
			f.input.Execution.Transactions[0].RejectBlockedBalances = true
			f.input.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{
				BalanceRef: ref, AssetCode: "USD", Permission: accounting.BalancePermissionReceive,
			}}
			// Both seeds and both blob copies say unblocked; only the key knows.
			f.writeAccountConfig(t, accounting.AccountConfig{AccountID: f.input.Execution.Balances[0].AccountID, Version: 1, Blocked: true})
			before := f.capture(t)

			raw, err := f.run(t)
			require.ErrorContains(t, err, `"code":"account_blocked"`, raw)
			require.ErrorContains(t, err, `"balanceRef":"`+ref+`"`)
			require.Equal(t, before, f.capture(t))
		})
	}

	t.Run("the generated companion movement of a draw", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Transactions[0].RejectBlockedBalances = true
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: f.input.Execution.Balances[0].AccountID, Version: 1, Blocked: true})

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"account_blocked"`)
	})

	t.Run("a valid grant still releases the primary and its companion", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.seedWithCopyVersions(t, 0, f.input.Execution.Balances[0])
		f.seedWithCopyVersions(t, 1, f.input.Execution.Balances[1])
		f.input.Execution.Transactions[0].RejectBlockedBalances = true
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: f.input.Execution.Balances[0].AccountID, Version: 1, Blocked: true})
		f.addGrant(t, "@source", "30")

		raw, err := f.run(t)
		require.NoError(t, err)

		result := decodeIntegrationResult(t, raw)
		require.Len(t, result.Movements, 2)
		require.Equal(t, "overdraft_companion", result.Movements[1].Role)
		require.True(t, result.Final[0].Blocked)
		fields := f.blobFields(t, 1)
		require.Equal(t, "1", string(fields["Blocked"]), "the companion blob carries the account's decided flag")
		require.Equal(t, `"1"`, string(fields[balancecache.BlobAccountConfigVersionField]))
	})
}

// TestIntegrationEngineAccountDecidesWithItsNewestConfiguration pins the account
// decision to the newest version among the account key, the request and the copies
// in every blob of the account: a live key does not win by being present, and no
// blob's copy wins by coming first in the pool.
func TestIntegrationEngineAccountDecidesWithItsNewestConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	blockedFixture := func(t *testing.T) *integrationFixture {
		t.Helper()

		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Transactions[0].RejectBlockedBalances = true

		return f
	}

	refusedAsBlocked := func(t *testing.T, f *integrationFixture) {
		t.Helper()

		before := f.capture(t)

		_, err := f.run(t)
		require.ErrorContains(t, err, `"code":"account_blocked"`)
		require.Equal(t, before, f.capture(t))
	}

	t.Run("an older account key loses to a newer request", func(t *testing.T) {
		f := blockedFixture(t)
		accountID := f.input.Execution.Balances[0].AccountID
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: false})
		for index := range f.input.Execution.Balances {
			f.input.Execution.Balances[index].Blocked = true
			f.input.Execution.Balances[index].AccountConfigVersion = 2
		}

		refusedAsBlocked(t, f)
	})

	t.Run("an older account key loses to a newer blob copy", func(t *testing.T) {
		f := blockedFixture(t)
		accountID := f.input.Execution.Balances[0].AccountID
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: false})
		for index := range f.input.Execution.Balances {
			f.input.Execution.Balances[index].AccountConfigVersion = 1
		}

		copied := f.input.Execution.Balances[0]
		copied.Blocked, copied.AccountConfigVersion = true, 2
		f.seed(t, 0, copied)

		refusedAsBlocked(t, f)
	})

	t.Run("the newest copy among the account's blobs decides, wherever it is in the pool", func(t *testing.T) {
		f := blockedFixture(t)
		// No account key. The first blob of the account holds version 1, not
		// blocked; the second, the companion, holds version 2, blocked.
		first := f.input.Execution.Balances[0]
		first.AccountConfigVersion = 1
		f.seed(t, 0, first)

		second := f.input.Execution.Balances[1]
		second.Blocked, second.AccountConfigVersion = true, 2
		f.seed(t, 1, second)

		refusedAsBlocked(t, f)
	})
}

func TestIntegrationEngineCanonicalConfigurationNeedsNoLimitRepair(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	// A newer key replaces the blob's versioned copy with its canonical limit; a
	// request at the blob's own version decides, and so does a newer key over a copy
	// without a version, but the blob keeps either copy as it was.
	for _, test := range []struct {
		source       string
		copyVersions bool
		blobLimit    string
	}{
		{source: "configuration key", copyVersions: true, blobLimit: `"1000"`},
		{source: "configuration key over a copy without a version", blobLimit: `"1000.00"`},
		{source: "request", blobLimit: `"1000.00"`},
	} {
		t.Run(test.source, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			f.input.Execution.Balances[0].OverdraftLimitEnabled = true
			encoded, err := balancecache.Encode(f.input.Execution.Balances[0], balancecache.FormatDual)
			require.NoError(t, err)
			if test.copyVersions {
				encoded = withCopyVersions(t, encoded, 0, 0)
			}
			encoded = replaceLimitTestBytes(t, encoded, []byte(`"OverdraftLimit":"1000"`), []byte(`"OverdraftLimit":"1000.00"`))
			require.NoError(t, f.client.Set(context.Background(), f.resolved.Balances["@source#default"].Balance, encoded, time.Hour).Err())
			if test.source != "request" {
				f.liveConfiguration(t, 0, f.input.Execution.Balances[0], 1)
			}

			raw, err := f.run(t)
			require.NoError(t, err, "a canonical limit decides; the blob's noncanonical copy needs no repair")
			require.Equal(t, "1000", decodeIntegrationResult(t, raw).Final[0].OverdraftLimit)

			fields := f.blobFields(t, 0)
			require.Equal(t, test.blobLimit, string(fields["OverdraftLimit"]))
			require.Equal(t, `"1000"`, string(fields["overdraftLimit"]))
		})
	}
}

// TestIntegrationEngineResultKeepsTheRollbackReleaseShape pins the JSON field
// sets of the engine response and of the recovery evidence to the ones the
// rollback release (v4.2.2) decodes with unknown fields refused and every field
// required. Configuration versions travel only in the request, the keys and the
// blob, never here.
func TestIntegrationEngineResultKeepsTheRollbackReleaseShape(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newOverdraftConfigFixture(t, container.Client)
	f.input.Execution.Balances[0].ConfigVersion = 7
	f.input.Execution.Balances[0].AccountConfigVersion = 3
	f.input.Execution.Balances[1].ConfigVersion = 2
	f.input.Execution.Balances[1].AccountConfigVersion = 3
	f.liveConfiguration(t, 0, f.input.Execution.Balances[0], 8)

	raw, err := f.run(t)
	require.NoError(t, err)

	rollbackFinal := []string{
		"accountId", "accountType", "alias", "allowOverdraft", "allowReceiving", "allowSending", "assetCode", "balanceRef",
		"balanceScope", "blocked", "direction", "id", "key", "ledgerId", "onHold", "organizationId", "overdraftLimit",
		"overdraftLimitEnabled", "overdraftUsed", "available", "version",
	}
	rollbackMovement := []string{"after", "amount", "balanceRef", "before", "overdraftDelta", "postingRef", "ref", "role", "transactionId", "type"}
	rollbackState := []string{"available", "onHold", "overdraftUsed", "version"}

	var response struct {
		Movements []map[string]json.RawMessage `json:"movements"`
		Final     []map[string]json.RawMessage `json:"final"`
	}
	require.Equal(t, []string{"appliedAtUnixMicro", "final", "movements", "protocolVersion"}, jsonFieldNames(t, []byte(raw)))
	require.NoError(t, json.Unmarshal([]byte(raw), &response))
	require.Len(t, response.Final, 2)
	for _, final := range response.Final {
		require.ElementsMatch(t, rollbackFinal, mapFieldNames(final))
	}
	for _, movement := range response.Movements {
		require.ElementsMatch(t, rollbackMovement, mapFieldNames(movement))
		require.ElementsMatch(t, rollbackState, jsonFieldNames(t, movement["before"]))
		require.ElementsMatch(t, rollbackState, jsonFieldNames(t, movement["after"]))
	}

	_, err = DecodeResult([]byte(raw), f.input.Execution)
	require.NoError(t, err, "the result decodes with the strict decoder both releases share")

	recoveryField := f.input.Execution.Transactions[0].ID.String() + ":" + f.input.Execution.ExecutionID.String()
	evidence, err := f.client.HGet(context.Background(), f.resolved.Recovery, recoveryField).Bytes()
	require.NoError(t, err)

	var recovery struct {
		Record struct {
			Result struct {
				Final []map[string]json.RawMessage `json:"final"`
			} `json:"result"`
		} `json:"record"`
	}
	require.NoError(t, json.Unmarshal(evidence, &recovery))
	require.Len(t, recovery.Record.Result.Final, 2)
	for _, final := range recovery.Record.Result.Final {
		require.ElementsMatch(t, rollbackFinal, mapFieldNames(final))
	}
}

func jsonFieldNames(t *testing.T, raw []byte) []string {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))

	return mapFieldNames(fields)
}

func mapFieldNames(fields map[string]json.RawMessage) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func TestIntegrationEngineRefusesAnotherProtocolVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	payload := string(f.prepared(t).Payload)
	require.Contains(t, payload, `"protocolVersion":4`)

	for _, version := range []string{"3", "5"} {
		before := f.capture(t)

		_, err := f.runRaw(t, strings.Replace(payload, `"protocolVersion":4`, `"protocolVersion":`+version, 1))
		require.ErrorContains(t, err, `"code":"invalid_protocol"`)
		require.Equal(t, before, f.capture(t))
	}
}

func TestIntegrationEngineRejectsMalformedConfigurationDeclarations(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	tests := []struct {
		name   string
		mutate func(*wireRequest)
	}{
		{"balance configuration key index", func(request *wireRequest) { request.Balances[0].ConfigKeyIndex-- }},
		{"account configuration key index", func(request *wireRequest) { request.Accounts[0].ConfigKeyIndex-- }},
		{"noncanonical balance configuration version", func(request *wireRequest) { request.Balances[0].BalanceConfigVersion = "01" }},
		{"negative balance configuration version", func(request *wireRequest) { request.Balances[0].BalanceConfigVersion = "-1" }},
		{"overflowing account configuration version", func(request *wireRequest) { request.Accounts[0].ConfigVersion = "9223372036854775808" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newOverdraftConfigFixture(t, container.Client)
			var request wireRequest
			require.NoError(t, json.Unmarshal(f.prepared(t).Payload, &request))
			test.mutate(&request)
			raw, err := json.Marshal(request)
			require.NoError(t, err)
			before := f.capture(t)

			_, err = f.runRaw(t, string(raw))
			require.ErrorContains(t, err, `"code":"invalid_protocol"`)
			require.Equal(t, before, f.capture(t))
		})
	}

	t.Run("configuration keys of another balance", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		prepared := f.prepared(t)
		var request wireRequest
		require.NoError(t, json.Unmarshal(prepared.Payload, &request))
		keys := append([]string(nil), prepared.Keys...)
		first, second := request.Balances[0].ConfigKeyIndex-1, request.Balances[1].ConfigKeyIndex-1
		keys[first], keys[second] = keys[second], keys[first]

		_, err := f.client.Eval(context.Background(), integrationEngineLua, keys, string(prepared.Payload),
			strconv.Itoa(f.limits.MaxRequestBytes), strconv.Itoa(f.limits.MaxPreparedBytes),
			strconv.Itoa(f.limits.MaxTransactions), strconv.Itoa(f.limits.MaxPostings), strconv.Itoa(f.limits.MaxBalances)).Text()
		require.ErrorContains(t, err, `"code":"invalid_protocol"`)
	})
}

// TestIntegrationEngineConfigKeySwitchUnderConcurrentWriters runs engine executions
// concurrently on one balance while the configuration key turns the overdraft
// off. Every execution that starts after the key changed decides with the new
// configuration, and the money the engine keeps equals the sum of the movements
// it approved: no execution loses another's update.
func TestIntegrationEngineConfigKeySwitchUnderConcurrentWriters(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	const (
		writers         = 8
		perWriter       = 12
		debit           = 5
		initialBalance  = 40
		switchAfterMost = 16
	)

	container := redistestutil.SetupReusableContainer(t)
	f := newOverdraftConfigFixture(t, container.Client)
	f.input.Execution.Balances[0].Available = decimal.NewFromInt(initialBalance)
	f.seed(t, 0, f.input.Execution.Balances[0])
	f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(debit)
	f.writeBalanceConfig(t, 0, snapshotBalanceConfig(f.input.Execution.Balances[0], 1))

	payloads := make([][]byte, 0, writers*perWriter)
	for range writers * perWriter {
		f.rotateExecution()
		payloads = append(payloads, f.prepared(t).Payload)
	}

	keys := f.prepared(t).Keys
	args := []any{
		strconv.Itoa(f.limits.MaxRequestBytes), strconv.Itoa(f.limits.MaxPreparedBytes),
		strconv.Itoa(f.limits.MaxTransactions), strconv.Itoa(f.limits.MaxPostings), strconv.Itoa(f.limits.MaxBalances),
	}

	type outcome struct {
		afterSwitch bool
		drew        bool
		approved    bool
	}

	var (
		switched  atomic.Bool
		approved  atomic.Int32
		outcomes  = make([]outcome, len(payloads))
		waitGroup sync.WaitGroup
		failures  = make(chan error, len(payloads))
		switchNow = make(chan struct{})
		once      sync.Once
	)

	for writer := range writers {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for i := writer * perWriter; i < (writer+1)*perWriter; i++ {
				afterSwitch := switched.Load()

				raw, err := container.Client.Eval(context.Background(), integrationEngineLua, keys, append([]any{string(payloads[i])}, args...)...).Text()
				if err != nil {
					if !strings.Contains(err.Error(), `"code":"insufficient_funds"`) {
						failures <- err
					}

					outcomes[i] = outcome{afterSwitch: afterSwitch}

					continue
				}

				var result integrationResult
				if err := json.Unmarshal([]byte(raw), &result); err != nil {
					failures <- err

					continue
				}

				outcomes[i] = outcome{afterSwitch: afterSwitch, approved: true, drew: len(result.Movements) == 2}

				if approved.Add(1) >= switchAfterMost {
					once.Do(func() { close(switchNow) })
				}
			}
		}()
	}

	// The writers are bounded; a failed assertion still waits for them, so none
	// keeps writing the shared Valkey while later tests run.
	t.Cleanup(waitGroup.Wait)

	select {
	case <-switchNow:
	case <-time.After(time.Minute):
		t.Fatal("the writers never reached the configuration switch")
	}

	off := f.input.Execution.Balances[0]
	off.AllowOverdraft = false
	f.writeBalanceConfig(t, 0, snapshotBalanceConfig(off, 2))
	switched.Store(true)

	waitGroup.Wait()
	close(failures)

	for err := range failures {
		require.NoError(t, err)
	}

	approvedCount, drewBefore, startedAfter := 0, 0, 0
	for _, result := range outcomes {
		if result.approved {
			approvedCount++
		}

		if result.drew && !result.afterSwitch {
			drewBefore++
		}

		if result.afterSwitch {
			startedAfter++
			require.False(t, result.drew, "an execution that started after the switch decided with the overdraft still on")
		}
	}

	require.Positive(t, drewBefore, "the scenario must draw overdraft before the switch")
	require.Positive(t, startedAfter, "the scenario must run executions after the switch")

	fields := f.blobFields(t, 0)
	primary, err := balancecache.Decode(mustMarshal(t, fields))
	require.NoError(t, err)
	companionFields := f.blobFields(t, 1)
	companion, err := balancecache.Decode(mustMarshal(t, companionFields))
	require.NoError(t, err)

	spent := decimal.NewFromInt(int64(debit * approvedCount))
	require.True(t, primary.Available.Sub(primary.OverdraftUsed).Equal(decimal.NewFromInt(initialBalance).Sub(spent)),
		"available %s and overdraft used %s must account for %s approved", primary.Available, primary.OverdraftUsed, spent)
	require.True(t, companion.Available.Equal(primary.OverdraftUsed), "the companion mirrors every draw")
	require.Equal(t, int64(approvedCount), primary.Version, "each approved debit advances the primary exactly once")
}

func mustMarshal(t *testing.T, fields map[string]json.RawMessage) []byte {
	t.Helper()

	raw, err := json.Marshal(fields)
	require.NoError(t, err)

	return raw
}

// TestIntegrationEngineCopyConfirmsAConfigurationPatch runs a configuration PATCH
// against the real engine: an engine execution commits between the PATCH's read
// of the blob and its compare-and-set, so the compare-and-set loses and the PATCH
// reads the blob again. Over a copy with a version, the execution copies the key's
// newer configuration and version into the blob, and the PATCH finds its version
// already there and confirms without writing. Over a copy without a version, which
// the execution keeps, the PATCH writes the copy itself on that second read. Either
// way the money the execution wrote stays.
func TestIntegrationEngineCopyConfirmsAConfigurationPatch(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	for _, test := range []struct {
		name        string
		copyVersion int64
		want        redistransaction.BlobCopyOutcome
	}{
		{name: "the engine copies over a copy with a version", copyVersion: 4, want: redistransaction.BlobCopyAlreadyCurrent},
		{name: "the PATCH copies over a copy without a version", want: redistransaction.BlobCopyWritten},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			input, limits := richAdapterExecution(t)
			balance := &input.Execution.Balances[0]
			balance.Available = decimal.NewFromInt(100)
			balance.AllowOverdraft = true
			balance.ConfigVersion = test.copyVersion
			payload, err := command.DecodeTransactionCompletionPlan(input.CompletionPlans[0].Payload)
			require.NoError(t, err)
			payload.OperationSpecs[0].Balance.Available = balance.Available
			input.CompletionPlans[0].Payload = encodeAdapterRecovery(t, &input, *payload)

			keys, err := resolveAdapterKeys(ctx, input.Execution)
			require.NoError(t, err)
			blobKey := keys.Balances[balance.BalanceRef].Balance
			encoded, err := balancecache.Encode(*balance, balancecache.FormatDual)
			require.NoError(t, err)
			require.NoError(t, container.Client.Set(ctx, blobKey, encoded, time.Hour).Err())

			adapter, err := newAdapterWithLimits(&integrationClientProvider{client: container.Client}, limits)
			require.NoError(t, err)

			patched := snapshotBalanceConfig(*balance, 5)
			patched.AllowOverdraft = false

			connection := redistestutil.CreateConnectionWithDB(t, container.Addr, container.DB)
			patchClient, err := connection.GetClient(ctx)
			require.NoError(t, err)

			var executed *accounting.ExecutionResult

			hook := &afterBlobRead{key: blobKey, after: func() error {
				var executeErr error

				executed, executeErr = adapter.Execute(ctx, input)

				return executeErr
			}}
			patchClient.AddHook(hook)

			repository, err := redistransaction.NewConsumerRedis(connection)
			require.NoError(t, err)

			_, outcome, err := repository.WriteBalanceConfig(ctx, input.Execution.OrganizationID, input.Execution.LedgerID, balance.BalanceRef, patched)
			require.NoError(t, err)
			require.Equal(t, redistransaction.ConfigWritten, outcome)

			copied, err := repository.CopyBalanceConfigToBlob(ctx, input.Execution.OrganizationID, input.Execution.LedgerID, balance.BalanceRef, patched)
			require.NoError(t, err)
			require.NoError(t, hook.err)
			require.NotNil(t, executed, "the engine execution ran between the PATCH's read and its write")
			require.Equal(t, test.want, copied)
			require.Equal(t, int32(2), hook.reads.Load(), "the PATCH read the blob again after its compare-and-set lost")

			raw, err := container.Client.Get(ctx, blobKey).Bytes()
			require.NoError(t, err)
			stored, err := balancecache.Decode(raw)
			require.NoError(t, err)
			require.True(t, stored.Available.Equal(executed.Final[0].Available), "the PATCH kept the money the engine wrote")
			require.Equal(t, executed.Final[0].Version, stored.Version)
			require.False(t, stored.AllowOverdraft)
			balanceVersion, _, err := balancecache.BlobConfigVersions(raw)
			require.NoError(t, err)
			require.Equal(t, int64(5), balanceVersion)
		})
	}
}

// TestIntegrationEngineDecidesWithTheCopyAPatchWroteIntoItsBlob closes the loop
// between the configuration PATCH and the engine. The PATCH writer copies a newer
// configuration into a blob of each shape the copy supports, the blob produced by
// an engine execution among them, and carries its money over unchanged; the engine
// then reads that copy and decides with it, with no configuration key and a
// request still at the older version.
func TestIntegrationEngineDecidesWithTheCopyAPatchWroteIntoItsBlob(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	repository, err := redistransaction.NewConsumerRedis(redistestutil.CreateConnectionWithDB(t, container.Addr, container.DB))
	require.NoError(t, err)

	// tenantFixture places the overdraft fixture under its own tenant, where the
	// PATCH writer of that tenant addresses the blobs.
	tenantFixture := func(t *testing.T) (*integrationFixture, context.Context) {
		t.Helper()

		tenantID := "config-copy-" + uuid.NewString()
		f := newIntegrationFixtureWithPrefix(t, container.Client, "tenant:"+tenantID+":")
		f.input.Execution.Balances[0].Available = decimal.NewFromInt(10)
		f.addCompanion("0")
		f.seed(t, 1, f.input.Execution.Balances[1])

		return f, tmcore.ContextWithTenantID(context.Background(), tenantID)
	}

	storeFields := func(t *testing.T, f *integrationFixture, fields map[string]json.RawMessage) {
		t.Helper()

		encoded, err := json.Marshal(fields)
		require.NoError(t, err)
		require.NoError(t, f.client.Set(context.Background(), f.resolved.Balances["@source#default"].Balance, encoded, time.Hour).Err())
	}

	shapes := []struct {
		name  string
		store func(t *testing.T, f *integrationFixture)
	}{
		{name: "a blob an engine execution wrote", store: func(t *testing.T, f *integrationFixture) {
			f.seed(t, 0, f.input.Execution.Balances[0])
			f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(4)

			_, err := f.run(t)
			require.NoError(t, err)
		}},
		{name: "a new-only blob", store: func(t *testing.T, f *integrationFixture) {
			encoded, err := balancecache.Encode(f.input.Execution.Balances[0], balancecache.FormatNewOnly)
			require.NoError(t, err)
			require.NoError(t, f.client.Set(context.Background(), f.resolved.Balances["@source#default"].Balance, encoded, time.Hour).Err())
		}},
		{name: "a legacy blob with the uppercase fields only", store: func(t *testing.T, f *integrationFixture) {
			primary := f.input.Execution.Balances[0]
			storeFields(t, f, pureLegacyIndexedAliasBlob(t, primary, primary.Alias))
		}},
	}

	moneyFields := []string{"Available", "available", "OnHold", "onHold", "OverdraftUsed", "overdraftUsed", "Version", "version"}
	money := func(fields map[string]json.RawMessage) map[string]string {
		kept := make(map[string]string, len(moneyFields))
		for _, name := range moneyFields {
			if raw, ok := fields[name]; ok {
				kept[name] = string(raw)
			}
		}

		return kept
	}

	for _, shape := range shapes {
		t.Run("the balance configuration copied into "+shape.name, func(t *testing.T) {
			f, ctx := tenantFixture(t)
			shape.store(t, f)

			primary := f.input.Execution.Balances[0]
			patched := snapshotBalanceConfig(primary, 1)
			patched.AllowOverdraft = false
			before := money(f.blobFields(t, 0))
			require.NotEmpty(t, before)

			outcome, err := repository.CopyBalanceConfigToBlob(ctx, f.input.Execution.OrganizationID, f.input.Execution.LedgerID, primary.BalanceRef, patched)
			require.NoError(t, err)
			require.Equal(t, redistransaction.BlobCopyWritten, outcome)

			fields := f.blobFields(t, 0)
			require.Equal(t, before, money(fields), "the copy carries the money over unchanged")
			requireCopiedOverdraft(t, fields, false, 1)

			// The request still carries version 0 with the overdraft allowed, and
			// there is no configuration key: only the copy turns the overdraft off.
			f.rotateExecution()
			f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(30)
			unchanged := f.capture(t)

			_, err = f.run(t)
			require.ErrorContains(t, err, `"code":"insufficient_funds"`, "the engine decides with the copy the PATCH wrote")
			require.Equal(t, unchanged, f.capture(t))

			f.rotateExecution()
			f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(1)

			raw, err := f.run(t)
			require.NoError(t, err, "the engine reads the copied blob")
			require.False(t, decodeIntegrationResult(t, raw).Final[0].AllowOverdraft)

			requireCopiedOverdraft(t, f.blobFields(t, 0), false, 1)
		})
	}

	t.Run("the account's blocked flag copied into blobs an engine execution wrote", func(t *testing.T) {
		f, ctx := tenantFixture(t)
		shapes[0].store(t, f)

		primary := f.input.Execution.Balances[0]
		refs := []string{primary.BalanceRef, f.input.Execution.Balances[1].BalanceRef}
		outcomes, err := repository.CopyAccountConfigToBlobs(ctx, f.input.Execution.OrganizationID, f.input.Execution.LedgerID, refs,
			accounting.AccountConfig{AccountID: primary.AccountID, Version: 1, Blocked: true})
		require.NoError(t, err)

		for _, ref := range refs {
			require.Equal(t, redistransaction.BlobCopyWritten, outcomes[ref], ref)
		}

		f.rotateExecution()
		f.input.Execution.Transactions[0].RejectBlockedBalances = true
		unchanged := f.capture(t)

		_, err = f.run(t)
		require.ErrorContains(t, err, `"code":"account_blocked"`, "the engine decides with the copy the PATCH wrote")
		require.Equal(t, unchanged, f.capture(t))
	})
}

// requireCopiedOverdraft reads the blob's balance configuration copy the way every
// reader does, whatever representations the blob carries, with its version.
func requireCopiedOverdraft(t *testing.T, fields map[string]json.RawMessage, allowOverdraft bool, version int64) {
	t.Helper()

	raw := mustMarshal(t, fields)
	blob, err := balancecache.DecodeForRead(raw)
	require.NoError(t, err)
	require.Equal(t, allowOverdraft, blob.AllowOverdraft)

	copied, _, err := balancecache.BlobConfigVersions(raw)
	require.NoError(t, err)
	require.Equal(t, version, copied)
}

// afterBlobRead runs once, right after the first read of one blob.
type afterBlobRead struct {
	key   string
	reads atomic.Int32
	after func() error
	err   error
}

func (h *afterBlobRead) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *afterBlobRead) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := next(ctx, cmd); err != nil {
			return err
		}

		if cmd.Name() != "get" || len(cmd.Args()) < 2 || cmd.Args()[1] != h.key {
			return nil
		}

		if h.reads.Add(1) == 1 {
			h.err = h.after()
		}

		return nil
	}
}

func (h *afterBlobRead) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
