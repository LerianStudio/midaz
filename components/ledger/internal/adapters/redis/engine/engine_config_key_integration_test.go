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
		requireBlobConfiguration(t, f.blobFields(t, 0), true, "5", "")
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

	t.Run("an earlier release's overdraft change survives, and a newer version is still copied", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Balances[0].ConfigVersion = 1
		primary := f.input.Execution.Balances[0]
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(primary, 1))
		// The earlier release turned the overdraft off in the blob and left the
		// version label it does not know as it was.
		changed := primary
		changed.AllowOverdraft = false
		f.seed(t, 0, changed)

		raw, err := f.run(t)
		require.NoError(t, err, "the key at the same version still allows the draw")

		result := decodeIntegrationResult(t, raw)
		require.Equal(t, "20", result.Final[0].OverdraftUsed)
		fields := f.blobFields(t, 0)
		requireBlobConfiguration(t, fields, false, "1", "")
		require.Equal(t, strconv.Quote(result.Final[0].Available), string(fields["Available"]), "money moves as usual")
		require.Equal(t, strconv.Quote(result.Final[0].OverdraftUsed), string(fields["OverdraftUsed"]))
		require.Equal(t, result.Final[0].Version, string(fields["Version"]))

		// Go reloaded the row at a newer version: the copy follows it.
		f.rotateExecution()
		f.input.Execution.Transactions[0].Postings[0].Amount = decimal.NewFromInt(1)
		f.input.Execution.Balances[0].ConfigVersion = 2
		f.writeBalanceConfig(t, 0, snapshotBalanceConfig(f.input.Execution.Balances[0], 2))

		_, err = f.run(t)
		require.NoError(t, err)
		requireBlobConfiguration(t, f.blobFields(t, 0), true, "2", "")
	})

	t.Run("an earlier release's account block survives in every blob of the account", func(t *testing.T) {
		f := newOverdraftConfigFixture(t, container.Client)
		f.input.Execution.Transactions[0].RejectBlockedBalances = true
		f.writeAccountConfig(t, accounting.AccountConfig{AccountID: f.input.Execution.Balances[0].AccountID, Version: 0, Blocked: false})
		// The earlier release blocked the account in both blobs; neither carries an
		// account version, so the copies and the key are all at version 0.
		for index := range f.input.Execution.Balances {
			blocked := f.input.Execution.Balances[index]
			blocked.Blocked = true
			f.seed(t, index, blocked)
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
			require.Equal(t, "1", string(fields["Blocked"]), "the blob keeps the block the earlier release wrote")
			require.Equal(t, "true", string(fields["blocked"]))
			require.NotContains(t, fields, balancecache.BlobAccountConfigVersionField)
			require.Equal(t, strconv.Quote(available[balance.BalanceRef]), string(fields["Available"]), "money moves as usual")
		}
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

func TestIntegrationEngineCanonicalConfigurationNeedsNoLimitRepair(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)

	// A newer key replaces the blob's copy with its canonical limit; a request at
	// the blob's own version decides, but the blob keeps its copy as it was.
	for _, test := range []struct {
		source    string
		blobLimit string
	}{
		{source: "configuration key", blobLimit: `"1000"`},
		{source: "request", blobLimit: `"1000.00"`},
	} {
		t.Run(test.source, func(t *testing.T) {
			f := newIntegrationFixture(t, container.Client)
			f.input.Execution.Balances[0].OverdraftLimitEnabled = true
			encoded, err := balancecache.Encode(f.input.Execution.Balances[0], balancecache.FormatDual)
			require.NoError(t, err)
			encoded = replaceLimitTestBytes(t, encoded, []byte(`"OverdraftLimit":"1000"`), []byte(`"OverdraftLimit":"1000.00"`))
			require.NoError(t, f.client.Set(context.Background(), f.resolved.Balances["@source#default"].Balance, encoded, time.Hour).Err())
			if test.source == "configuration key" {
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
// of the blob and its compare-and-set. The execution copies the key's
// configuration and version into the blob, so the PATCH finds its version already
// there and confirms without writing the blob, and the money the execution wrote
// stays.
func TestIntegrationEngineCopyConfirmsAConfigurationPatch(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	ctx := context.Background()
	container := redistestutil.SetupReusableContainer(t)
	input, limits := richAdapterExecution(t)
	balance := &input.Execution.Balances[0]
	balance.Available = decimal.NewFromInt(100)
	balance.AllowOverdraft = true
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
	require.Equal(t, redistransaction.BlobCopyAlreadyCurrent, copied, "the engine already copied the PATCH's configuration")
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
