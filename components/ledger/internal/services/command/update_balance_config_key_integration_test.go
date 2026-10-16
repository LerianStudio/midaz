//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// configPatchHarness drives the real balance and account PATCH use cases against
// PostgreSQL and the cache. The cache client the use case uses carries a hook, so a
// test can fail, hold or observe the commands a PATCH sends, and stand in for an
// engine write between two of them.
type configPatchHarness struct {
	uc *UseCase

	transactionDB *sql.DB
	onboardingDB  *sql.DB
	cache         *redistestutil.ContainerResult
	redisRepo     *txRedis.RedisConsumerRepository
	balances      *controllableBalanceRepo
	hook          *patchCommandHook
	emitter       *pkgStreaming.MockEmitter
	eventsFrom    int

	organizationID uuid.UUID
	ledgerID       uuid.UUID
}

func newConfigPatchHarness(t *testing.T) *configPatchHarness {
	t.Helper()

	transaction := pgtestutil.SetupMigratedContainer(t, "transaction")
	cache := redistestutil.SetupReusableContainer(t)

	transactionConn := pgtestutil.ConnectPostgresClient(t.Context(), t,
		pgtestutil.BuildConnectionString(transaction.Host, transaction.Port, transaction.Config),
		pgtestutil.BuildConnectionString(transaction.Host, transaction.Port, transaction.Config))

	conn := redistestutil.CreateConnectionWithDB(t, cache.Addr, cache.DB)
	client, err := conn.GetClient(t.Context())
	require.NoError(t, err)

	hook := &patchCommandHook{}
	client.AddHook(hook)

	redisRepo, err := txRedis.NewConsumerRedis(conn)
	require.NoError(t, err)

	balances := &controllableBalanceRepo{Repository: balance.NewBalancePostgreSQLRepository(transactionConn, false)}
	emitter := pkgStreaming.NewMockEmitter()

	return &configPatchHarness{
		uc: &UseCase{
			BalanceRepo:          balances,
			TransactionRedisRepo: redisRepo,
			TransactionReader:    &feeDebtReader{},
			Streaming:            emitter,
		},
		transactionDB:  transaction.DB,
		cache:          cache,
		redisRepo:      redisRepo,
		balances:       balances,
		hook:           hook,
		emitter:        emitter,
		organizationID: uuid.Must(libCommons.GenerateUUIDv7()),
		ledgerID:       uuid.Must(libCommons.GenerateUUIDv7()),
	}
}

// withAccounts adds the onboarding database that owns account.blocked, so the
// account PATCH runs against its real row.
func (h *configPatchHarness) withAccounts(t *testing.T) {
	t.Helper()

	onboarding := pgtestutil.SetupMigratedContainer(t, "onboarding")
	onboardingConn := pgtestutil.ConnectPostgresClient(t.Context(), t,
		pgtestutil.BuildConnectionString(onboarding.Host, onboarding.Port, onboarding.Config),
		pgtestutil.BuildConnectionString(onboarding.Host, onboarding.Port, onboarding.Config))

	h.onboardingDB = onboarding.DB
	h.organizationID = pgtestutil.CreateTestOrganization(t, onboarding.DB)
	h.ledgerID = pgtestutil.CreateTestLedger(t, onboarding.DB, h.organizationID)

	// The PATCH carries no metadata; the onboarding metadata store holds none.
	metadata := mongodb.NewMockRepository(gomock.NewController(t))
	metadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	h.uc.AccountRepo = account.NewAccountPostgreSQLRepository(onboardingConn)
	h.uc.OnboardingMetadataRepo = metadata
}

// controllableBalanceRepo is the real balance repository with switches that make
// one call fail or run something just before the parent's UPDATE.
type controllableBalanceRepo struct {
	balance.Repository

	failUpdates  atomic.Int32
	findErr      atomic.Pointer[error]
	beforeUpdate atomic.Pointer[func()]
}

var errInjectedDatabaseFailure = errors.New("injected database failure")

func (r *controllableBalanceRepo) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) (*mmodel.Balance, bool, error) {
	if r.failUpdates.Add(-1) >= 0 {
		return nil, false, errInjectedDatabaseFailure
	}

	if before := r.beforeUpdate.Swap(nil); before != nil {
		(*before)()
	}

	return r.Repository.Update(ctx, organizationID, ledgerID, balanceID, update)
}

func (r *controllableBalanceRepo) FindByAccountIDAndKey(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, key string) (*mmodel.Balance, error) {
	if err := r.findErr.Load(); err != nil {
		return nil, *err
	}

	return r.Repository.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, key)
}

// patchRequestTag marks the commands one request sends, so a test can hold or
// observe one PATCH among several.
type patchRequestTag struct{}

func taggedRequest(ctx context.Context, tag string) context.Context {
	return context.WithValue(ctx, patchRequestTag{}, tag)
}

// patchCommand is one command a PATCH sent to the cache.
type patchCommand struct {
	tag  string
	name string
	keys []string
}

// patchCommandHook records every command that succeeded and runs the test's
// interventions: before may fail or hold a command, after runs once a command
// succeeded.
type patchCommandHook struct {
	mu       sync.Mutex
	commands []patchCommand
	before   func(context.Context, patchCommand) error
	after    func(context.Context, patchCommand)
}

func (h *patchCommandHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *patchCommandHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h *patchCommandHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		tag, _ := ctx.Value(patchRequestTag{}).(string)
		command := patchCommand{tag: tag, name: cmd.Name(), keys: commandKeys(cmd)}

		h.mu.Lock()
		before, after := h.before, h.after
		h.mu.Unlock()

		if before != nil {
			if err := before(ctx, command); err != nil {
				cmd.SetErr(err)

				return err
			}
		}

		if err := next(ctx, cmd); err != nil {
			return err
		}

		// Only commands that succeeded are recorded, so a script loaded by EVAL
		// after a NOSCRIPT reply counts once.
		h.mu.Lock()
		h.commands = append(h.commands, command)
		h.mu.Unlock()

		if after != nil {
			after(ctx, command)
		}

		return nil
	}
}

func commandKeys(cmd redis.Cmder) []string {
	args := cmd.Args()

	switch cmd.Name() {
	case "eval", "evalsha":
		if len(args) < 3 {
			return nil
		}

		var count int
		if _, err := fmt.Sscan(fmt.Sprint(args[2]), &count); err != nil || len(args) < 3+count {
			return nil
		}

		keys := make([]string, 0, count)
		for _, key := range args[3 : 3+count] {
			keys = append(keys, fmt.Sprint(key))
		}

		return keys
	case "mget":
		keys := make([]string, 0, len(args)-1)
		for _, key := range args[1:] {
			keys = append(keys, fmt.Sprint(key))
		}

		return keys
	default:
		if len(args) < 2 {
			return nil
		}

		return []string{fmt.Sprint(args[1])}
	}
}

func (h *patchCommandHook) intervene(before func(context.Context, patchCommand) error, after func(context.Context, patchCommand)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.before, h.after = before, after
}

func (h *patchCommandHook) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.commands, h.before, h.after = nil, nil, nil
}

// count reports how many recorded commands match.
func (h *patchCommandHook) count(match func(patchCommand) bool) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	n := 0

	for _, command := range h.commands {
		if match(command) {
			n++
		}
	}

	return n
}

func isScriptOn(command patchCommand, key string) bool {
	return (command.name == "eval" || command.name == "evalsha") && len(command.keys) > 0 && command.keys[0] == key
}

// failOnce fails the first command matching, once.
func failOnce(match func(patchCommand) bool) func(context.Context, patchCommand) error {
	var failed atomic.Bool

	return func(_ context.Context, command patchCommand) error {
		if match(command) && failed.CompareAndSwap(false, true) {
			return errors.New("injected cache failure")
		}

		return nil
	}
}

// configPatchBalance describes one persisted balance row.
type configPatchBalance struct {
	id             uuid.UUID
	accountID      uuid.UUID
	alias          string
	key            string
	allowSending   bool
	allowReceiving bool
	settings       *mmodel.BalanceSettings
	configVersion  int64
	deleted        bool
}

func (b configPatchBalance) ref() string { return b.alias + "#" + b.key }

func newConfigPatchBalance(alias string) configPatchBalance {
	return configPatchBalance{
		id:             uuid.Must(libCommons.GenerateUUIDv7()),
		accountID:      uuid.Must(libCommons.GenerateUUIDv7()),
		alias:          alias,
		key:            constant.DefaultBalanceKey,
		allowSending:   true,
		allowReceiving: true,
		settings:       &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeTransactional},
	}
}

func (h *configPatchHarness) seedBalance(t *testing.T, seed configPatchBalance) {
	t.Helper()

	settings, err := json.Marshal(seed.settings)
	require.NoError(t, err)

	var deletedAt any
	if seed.deleted {
		deletedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	}

	_, err = h.transactionDB.Exec(`
		INSERT INTO balance (
			id, organization_id, ledger_id, account_id, alias, key, asset_code,
			available, on_hold, overdraft_used, version, account_type,
			allow_sending, allow_receiving, direction, settings, config_version, created_at, updated_at, deleted_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'USD', 0, 0, 0, 1, 'deposit', $7, $8, 'credit', $9, $10, $11, $11, $12)
	`, seed.id, h.organizationID, h.ledgerID, seed.accountID, seed.alias, seed.key,
		seed.allowSending, seed.allowReceiving, settings, seed.configVersion, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), deletedAt)
	require.NoError(t, err, "failed to seed the balance")
}

// configPatchMoney is the live money a blob carries.
type configPatchMoney struct {
	available string
	onHold    string
	version   int64
}

// cacheBlob publishes the live blob of a balance: its money, and a configuration
// copy of the row's flags and settings under the given copy versions.
func (h *configPatchHarness) cacheBlob(t *testing.T, seed configPatchBalance, money configPatchMoney, copyVersion, accountCopyVersion int64) string {
	t.Helper()

	encoded := h.encodeBlob(t, seed, money, copyVersion, accountCopyVersion)
	require.NoError(t, h.cache.Client.Set(t.Context(), h.blobKey(seed.ref()), encoded, time.Hour).Err())

	return encoded
}

// encodeBlob is safe to call from any goroutine: it reports through assert only.
func (h *configPatchHarness) encodeBlob(t *testing.T, seed configPatchBalance, money configPatchMoney, copyVersion, accountCopyVersion int64) string {
	snapshot := accounting.BalanceSnapshot{
		BalanceRef: seed.ref(), ID: seed.id, AccountID: seed.accountID,
		AccountType: "deposit", AssetCode: "USD", Alias: seed.alias, Key: seed.key,
		Direction: "credit", BalanceScope: mmodel.BalanceScopeTransactional,
		Available: decimal.RequireFromString(money.available), OnHold: decimal.RequireFromString(money.onHold),
		OverdraftUsed: decimal.Zero, OverdraftLimit: decimal.Zero, Version: money.version,
		AllowSending: seed.allowSending, AllowReceiving: seed.allowReceiving,
		ConfigVersion: copyVersion, AccountConfigVersion: accountCopyVersion,
	}

	if settings := seed.settings; settings != nil {
		snapshot.AllowOverdraft = settings.AllowOverdraft
		snapshot.OverdraftLimitEnabled = settings.OverdraftLimitEnabled

		if settings.OverdraftLimit != nil {
			snapshot.OverdraftLimit = decimal.RequireFromString(*settings.OverdraftLimit)
		}
	}

	encoded, err := balancecache.Encode(snapshot, balancecache.FormatDual)
	assert.NoError(t, err)

	return string(encoded)
}

func (h *configPatchHarness) blobKey(ref string) string {
	return utils.BalanceInternalKey(h.organizationID, h.ledgerID, ref)
}

func (h *configPatchHarness) configKey(ref string) string {
	return utils.BalanceConfigKey(h.organizationID, h.ledgerID, ref)
}

func (h *configPatchHarness) blob(t *testing.T, ref string) string {
	t.Helper()

	raw, err := h.cache.Client.Get(t.Context(), h.blobKey(ref)).Result()
	require.NoError(t, err)

	return raw
}

func (h *configPatchHarness) storedConfig(t *testing.T, ref string) accounting.BalanceConfig {
	t.Helper()

	raw, err := h.cache.Client.Get(t.Context(), h.configKey(ref)).Bytes()
	require.NoError(t, err)

	config, err := balancecache.DecodeBalanceConfig(raw)
	require.NoError(t, err)

	return config
}

// writeRowToKey writes the configuration key from the row as PostgreSQL holds it,
// which is what a seed leaves behind.
func (h *configPatchHarness) writeRowToKey(t *testing.T, seed configPatchBalance) {
	t.Helper()

	row, err := h.balances.Find(t.Context(), h.organizationID, h.ledgerID, seed.id)
	require.NoError(t, err)

	config, err := balancecache.BalanceConfigFromBalance(row)
	require.NoError(t, err)

	_, _, err = h.redisRepo.WriteBalanceConfig(t.Context(), h.organizationID, h.ledgerID, seed.ref(), config)
	require.NoError(t, err)
}

func (h *configPatchHarness) row(t *testing.T, id uuid.UUID) *mmodel.Balance {
	t.Helper()

	row, err := h.balances.Find(t.Context(), h.organizationID, h.ledgerID, id)
	require.NoError(t, err)

	return row
}

func (h *configPatchHarness) patch(ctx context.Context, seed configPatchBalance, update mmodel.UpdateBalance) error {
	_, err := h.uc.Update(ctx, h.organizationID, h.ledgerID, seed.id, update)

	return err
}

// forgetEvents makes the events published so far invisible to events and
// published.
func (h *configPatchHarness) forgetEvents() {
	h.eventsFrom = len(h.emitter.Events())
}

func (h *configPatchHarness) published() int {
	return len(h.emitter.Events()) - h.eventsFrom
}

// events returns the subjects of the balance.config_changed events of a change type.
func (h *configPatchHarness) events(t *testing.T, changeType string) []string {
	t.Helper()

	subjects := []string{}

	for _, event := range h.emitter.Events()[h.eventsFrom:] {
		var payload map[string]any
		require.NoError(t, json.Unmarshal(event.Payload, &payload))

		if payload["changeType"] == changeType {
			subjects = append(subjects, event.Subject)
		}
	}

	return subjects
}

func blobFieldsOf(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &fields))

	return fields
}

var blobMoneyFields = []string{"Available", "available", "OnHold", "onHold", "Version", "version", "OverdraftUsed", "overdraftUsed", "ID", "id"}

func requireSameMoney(t *testing.T, before, after string) {
	t.Helper()

	original, current := blobFieldsOf(t, before), blobFieldsOf(t, after)

	for _, name := range blobMoneyFields {
		assert.Equal(t, string(original[name]), string(current[name]), "%s must keep its exact bytes", name)
	}
}

func boolPointer(value bool) *bool { return &value }

func overdraftSettings(allow bool, limit string) *mmodel.BalanceSettings {
	settings := &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeTransactional, AllowOverdraft: allow}
	if limit != "" {
		settings.OverdraftLimitEnabled = true
		settings.OverdraftLimit = &limit
	}

	return settings
}

// A PATCH on a cached balance whose blob holds an older copy: the key gets the
// committed row with its version, and the blob gets the whole configuration and the
// version without a single money byte changing.
func TestIntegration_ConfigKeyPatch_WritesTheCommittedRowAndConfirmsTheCopy(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-a1")
	seed.settings = overdraftSettings(true, "500")
	seed.configVersion = 4
	h.seedBalance(t, seed)
	h.writeRowToKey(t, seed)

	before := h.cacheBlob(t, seed, configPatchMoney{available: "1500.25", onHold: "10", version: 42}, 4, 0)

	require.NoError(t, h.patch(t.Context(), seed, mmodel.UpdateBalance{Settings: overdraftSettings(false, "")}))

	row := h.row(t, seed.id)
	assert.Equal(t, int64(5), row.ConfigVersion)

	stored := h.storedConfig(t, seed.ref())
	assert.Equal(t, seed.id, stored.BalanceID)
	assert.Equal(t, int64(5), stored.Version)
	assert.False(t, stored.AllowOverdraft)
	assert.False(t, stored.OverdraftLimitEnabled)
	assert.True(t, stored.AllowSending)

	ttl, err := h.cache.Client.PTTL(t.Context(), h.configKey(seed.ref())).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 59*time.Minute, "a written key lives one hour")

	after := h.blob(t, seed.ref())
	requireSameMoney(t, before, after)

	fields := blobFieldsOf(t, after)
	assert.Equal(t, `0`, string(fields["AllowOverdraft"]))
	assert.Equal(t, `false`, string(fields["allowOverdraft"]))
	assert.Equal(t, `0`, string(fields["OverdraftLimitEnabled"]))
	assert.Equal(t, `"5"`, string(fields[balancecache.BlobBalanceConfigVersionField]))
}

// Every cache failure after the commit fails the PATCH as a technical error, with
// PostgreSQL already holding the new configuration; the client's retry commits
// nothing new (same version) and completes the key and the copy.
func TestIntegration_ConfigKeyPatch_FailsClosedAndTheRetryCompletes(t *testing.T) {
	cases := []struct {
		name   string
		update mmodel.UpdateBalance
		failOn func(h *configPatchHarness, seed configPatchBalance) func(patchCommand) bool
		check  func(t *testing.T, row *mmodel.Balance)
	}{
		{
			name:   "settings with the key write failing",
			update: mmodel.UpdateBalance{Settings: overdraftSettings(false, "")},
			failOn: func(h *configPatchHarness, seed configPatchBalance) func(patchCommand) bool {
				return func(c patchCommand) bool { return isScriptOn(c, h.configKey(seed.ref())) }
			},
			check: func(t *testing.T, row *mmodel.Balance) { assert.False(t, row.Settings.AllowOverdraft) },
		},
		{
			name:   "allow flags with the key write failing",
			update: mmodel.UpdateBalance{AllowSending: boolPointer(false)},
			failOn: func(h *configPatchHarness, seed configPatchBalance) func(patchCommand) bool {
				return func(c patchCommand) bool { return isScriptOn(c, h.configKey(seed.ref())) }
			},
			check: func(t *testing.T, row *mmodel.Balance) { assert.False(t, row.AllowSending) },
		},
		{
			name:   "settings with the blob copy failing",
			update: mmodel.UpdateBalance{Settings: overdraftSettings(false, "")},
			failOn: func(h *configPatchHarness, seed configPatchBalance) func(patchCommand) bool {
				return func(c patchCommand) bool { return isScriptOn(c, h.blobKey(seed.ref())) }
			},
			check: func(t *testing.T, row *mmodel.Balance) { assert.False(t, row.Settings.AllowOverdraft) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newConfigPatchHarness(t)

			seed := newConfigPatchBalance("@config-a2")
			seed.settings = overdraftSettings(true, "")
			h.seedBalance(t, seed)
			h.writeRowToKey(t, seed)
			before := h.cacheBlob(t, seed, configPatchMoney{available: "100", onHold: "0", version: 3}, 0, 0)

			h.hook.intervene(failOnce(tc.failOn(h, seed)), nil)

			err := h.patch(t.Context(), seed, tc.update)
			require.Error(t, err, "the PATCH must fail closed")
			assert.False(t, pkg.IsBusinessError(err), "a cache failure is a technical error")

			committed := h.row(t, seed.id)
			assert.Equal(t, int64(1), committed.ConfigVersion, "PostgreSQL already holds the new configuration")
			tc.check(t, committed)

			h.hook.reset()

			require.NoError(t, h.patch(t.Context(), seed, tc.update), "the retry completes the write")

			assert.Equal(t, int64(1), h.row(t, seed.id).ConfigVersion, "the retry commits no new version")

			stored := h.storedConfig(t, seed.ref())
			assert.Equal(t, int64(1), stored.Version)

			committedConfig, err := balancecache.BalanceConfigFromBalance(committed)
			require.NoError(t, err)
			assert.True(t, stored.Equal(committedConfig), "the key holds the committed row: %+v", stored)

			after := h.blob(t, seed.ref())
			requireSameMoney(t, before, after)
			assert.Equal(t, `"1"`, string(blobFieldsOf(t, after)[balancecache.BlobBalanceConfigVersionField]))
		})
	}
}

// A transaction approved between the PATCH's read of the blob and its
// compare-and-set moves money and keeps the blob's configuration copy, as the engine
// does with every blob that exists: the compare-and-set loses, and the PATCH reads
// the blob again and writes its copy over the transaction's bytes, money untouched.
func TestIntegration_ConfigKeyPatch_TransactionBetweenReadAndWriteMakesThePatchCopyAgain(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-a14")
	seed.configVersion = 4
	h.seedBalance(t, seed)
	h.writeRowToKey(t, seed)
	h.cacheBlob(t, seed, configPatchMoney{available: "100", onHold: "0", version: 7}, 4, 0)

	var engineBytes string

	var moved atomic.Bool

	h.hook.intervene(nil, func(ctx context.Context, c patchCommand) {
		if c.tag == "patch" && c.name == "get" && c.keys[0] == h.blobKey(seed.ref()) && moved.CompareAndSwap(false, true) {
			// The engine's write: new money, the blob's own copy and labels kept.
			engineBytes = h.cacheBlob(t, seed, configPatchMoney{available: "99.5", onHold: "0", version: 8}, 4, 0)
		}
	})

	require.NoError(t, h.patch(taggedRequest(t.Context(), "patch"), seed, mmodel.UpdateBalance{AllowSending: boolPointer(false)}))

	after := h.blob(t, seed.ref())
	requireSameMoney(t, engineBytes, after)
	fields := blobFieldsOf(t, after)
	assert.Equal(t, `0`, string(fields["AllowSending"]), "the PATCH wrote its copy on its second read")
	assert.Equal(t, `false`, string(fields["allowSending"]))
	assert.Equal(t, `"5"`, string(fields[balancecache.BlobBalanceConfigVersionField]))
	assert.Equal(t, 3, h.hook.count(func(c patchCommand) bool {
		return c.tag == "patch" && c.name == "get" && c.keys[0] == h.blobKey(seed.ref())
	}), "the refused compare-and-set forces exactly one fresh read, then the response reads the money")
}

// Two PATCHes on different fields of one balance, the earlier one held between its
// commit and its cache writes: both answer success and the blob ends with the whole
// configuration of the newest version, never a partial copy stamped with it.
func TestIntegration_ConfigKeyPatch_ConcurrentPatchesLeaveTheNewestWholeCopy(t *testing.T) {
	settings := mmodel.UpdateBalance{Settings: overdraftSettings(true, "250")}
	flags := mmodel.UpdateBalance{AllowSending: boolPointer(false)}

	for name, order := range map[string][2]mmodel.UpdateBalance{
		"settings held, allow flags first": {settings, flags},
		"allow flags held, settings first": {flags, settings},
	} {
		t.Run(name, func(t *testing.T) {
			h := newConfigPatchHarness(t)

			seed := newConfigPatchBalance("@config-a16")
			h.seedBalance(t, seed)
			h.writeRowToKey(t, seed)
			before := h.cacheBlob(t, seed, configPatchMoney{available: "100", onHold: "0", version: 3}, 0, 0)

			reached, release := make(chan struct{}), make(chan struct{})

			var (
				held        atomic.Bool
				releaseOnce sync.Once
				patches     sync.WaitGroup
			)

			releaseHeld := func() { releaseOnce.Do(func() { close(release) }) }
			// A failed assertion must not leave the held PATCH blocked on the shared
			// stores: release it and wait for it before the harness goes away.
			t.Cleanup(func() {
				releaseHeld()
				patches.Wait()
			})

			h.hook.intervene(func(_ context.Context, c patchCommand) error {
				if c.tag == "held" && held.CompareAndSwap(false, true) {
					close(reached)
					<-release
				}

				return nil
			}, nil)

			heldResult := make(chan error, 1)

			patches.Add(1)

			go func() {
				defer patches.Done()

				heldResult <- h.patch(taggedRequest(t.Context(), "held"), seed, order[0])
			}()

			select {
			case <-reached:
			case <-time.After(30 * time.Second):
				t.Fatal("the held PATCH never reached the cache")
			}

			require.NoError(t, h.patch(t.Context(), seed, order[1]))
			releaseHeld()
			require.NoError(t, <-heldResult)

			row := h.row(t, seed.id)
			assert.Equal(t, int64(2), row.ConfigVersion)

			stored := h.storedConfig(t, seed.ref())
			assert.Equal(t, int64(2), stored.Version, "the key holds the newest version")
			assert.True(t, stored.AllowOverdraft)
			assert.False(t, stored.AllowSending)

			after := h.blob(t, seed.ref())
			requireSameMoney(t, before, after)

			fields := blobFieldsOf(t, after)
			for field, want := range map[string]string{
				"AllowSending": `0`, "allowSending": `false`, "AllowReceiving": `1`, "allowReceiving": `true`,
				"AllowOverdraft": `1`, "allowOverdraft": `true`, "OverdraftLimitEnabled": `1`, "overdraftLimitEnabled": `true`,
				"OverdraftLimit": `"250"`, "overdraftLimit": `"250"`, "BalanceScope": `"transactional"`,
				balancecache.BlobBalanceConfigVersionField: `"2"`,
			} {
				assert.Equal(t, want, string(fields[field]), field)
			}

			assert.Zero(t, h.hook.count(func(c patchCommand) bool { return c.tag == "held" && isScriptOn(c, h.blobKey(seed.ref())) }),
				"the held PATCH finds a newer copy and never writes the blob")
		})
	}
}

// A client repeating the same PATCH before every transaction on a hot balance: the
// version never rises, every PATCH answers success, announces settings_updated and
// only reads the blob, so it never contends with the transactions.
func TestIntegration_ConfigKeyPatch_RepeatedValueOnAHotBalanceOnlyReads(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-a8")
	seed.settings = overdraftSettings(true, "100")
	h.seedBalance(t, seed)

	update := mmodel.UpdateBalance{Settings: overdraftSettings(true, "100")}

	// The first PATCH settles the row and its version; the repetitions follow.
	require.NoError(t, h.patch(t.Context(), seed, update))

	version := h.row(t, seed.id).ConfigVersion
	h.cacheBlob(t, seed, configPatchMoney{available: "1000", onHold: "0", version: 1}, version, 0)

	stop := make(chan struct{})
	engine := sync.WaitGroup{}
	engine.Add(1)

	var stopOnce sync.Once

	stopEngine := func() {
		stopOnce.Do(func() { close(stop) })
		engine.Wait()
	}
	// A failed assertion must not leave the stand-in rewriting the shared blob.
	t.Cleanup(stopEngine)

	// Stands in for the engine: every approved transaction rewrites the blob with
	// new money and the configuration copy it already holds.
	go func() {
		defer engine.Done()

		for i := int64(2); ; i++ {
			select {
			case <-stop:
				return
			default:
			}

			encoded := h.encodeBlob(t, seed, configPatchMoney{available: decimal.NewFromInt(1000 - i).String(), onHold: "0", version: i}, version, 0)
			if !assert.NoError(t, h.cache.Client.Set(context.Background(), h.blobKey(seed.ref()), encoded, time.Hour).Err()) {
				return
			}
		}
	}()

	h.hook.reset()
	h.forgetEvents()

	const repetitions = 20

	for range repetitions {
		require.NoError(t, h.patch(taggedRequest(t.Context(), "repeat"), seed, update))
	}

	stopEngine()

	assert.Equal(t, version, h.row(t, seed.id).ConfigVersion, "the same value never raises the version")
	assert.Zero(t, h.hook.count(func(c patchCommand) bool { return c.tag == "repeat" && isScriptOn(c, h.blobKey(seed.ref())) }),
		"a repeated PATCH never writes the blob")
	assert.Equal(t, 2*repetitions, h.hook.count(func(c patchCommand) bool {
		return c.tag == "repeat" && c.name == "get" && c.keys[0] == h.blobKey(seed.ref())
	}), "each PATCH reads the blob once to confirm the copy and once for the money it answers")
	assert.Len(t, h.events(t, "settings_updated"), repetitions)
}

// A PATCH enabling the overdraft whose key write fails publishes overdraft_enabled
// right after its commit; the retry does not make the transition again and does not
// publish it twice.
func TestIntegration_ConfigKeyPatch_OverdraftEnabledOnceAcrossACacheRetry(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-a10a")
	h.seedBalance(t, seed)

	h.hook.intervene(failOnce(func(c patchCommand) bool { return isScriptOn(c, h.configKey(seed.ref())) }), nil)

	update := mmodel.UpdateBalance{Settings: overdraftSettings(true, "")}

	require.Error(t, h.patch(t.Context(), seed, update))
	h.hook.reset()
	require.NoError(t, h.patch(t.Context(), seed, update))

	companions := h.events(t, "overdraft_enabled")
	require.Len(t, companions, 1, "exactly one publication in total")
	assert.Equal(t, h.companionID(t, seed), companions[0])
	assert.Len(t, h.events(t, "settings_updated"), 2)
}

// A PATCH whose parent UPDATE fails after the companion was created publishes
// nothing; the retry makes the transition and publishes it with the companion that
// already exists.
func TestIntegration_ConfigKeyPatch_RetryAfterAParentFailurePublishesWithTheExistingCompanion(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-a10b")
	h.seedBalance(t, seed)

	h.balances.failUpdates.Store(1)

	update := mmodel.UpdateBalance{Settings: overdraftSettings(true, "")}

	require.ErrorIs(t, h.patch(t.Context(), seed, update), errInjectedDatabaseFailure)

	companionID := h.companionID(t, seed)
	assert.False(t, h.row(t, seed.id).Settings.AllowOverdraft, "the parent was not updated")
	assert.Zero(t, h.published())

	require.NoError(t, h.patch(t.Context(), seed, update))

	assert.Equal(t, []string{companionID}, h.events(t, "overdraft_enabled"))
}

// The companion is readied before the parent's UPDATE: when its lookup fails, the
// parent's configuration is never committed.
func TestIntegration_ConfigKeyPatch_CompanionLookupFailureLeavesTheParentUntouched(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-r3a")
	h.seedBalance(t, seed)

	lookupErr := errors.New("connection reset")
	h.balances.findErr.Store(&lookupErr)

	require.ErrorIs(t, h.patch(t.Context(), seed, mmodel.UpdateBalance{Settings: overdraftSettings(true, "")}), lookupErr)

	row := h.row(t, seed.id)
	assert.False(t, row.Settings.AllowOverdraft)
	assert.Zero(t, row.ConfigVersion)
	assert.Zero(t, h.published())
}

// The current read shows the overdraft on, another PATCH switches it off before
// this PATCH's UPDATE, and this UPDATE switches it on again: the transition is this
// UPDATE's, and it publishes with the companion readied before it.
func TestIntegration_ConfigKeyPatch_TransitionAfterAStaleReadPublishesWithTheReadiedCompanion(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-r3b")
	h.seedBalance(t, seed)

	enable := mmodel.UpdateBalance{Settings: overdraftSettings(true, "")}
	require.NoError(t, h.patch(t.Context(), seed, enable))

	companionID := h.companionID(t, seed)
	h.forgetEvents()

	disableInBetween := func() {
		_, _, err := h.balances.Repository.Update(t.Context(), h.organizationID, h.ledgerID, seed.id,
			mmodel.UpdateBalance{Settings: overdraftSettings(false, "")})
		require.NoError(t, err)
	}
	h.balances.beforeUpdate.Store(&disableInBetween)

	require.NoError(t, h.patch(t.Context(), seed, enable))

	assert.Equal(t, []string{companionID}, h.events(t, "overdraft_enabled"))
	assert.True(t, h.row(t, seed.id).Settings.AllowOverdraft)
}

// Two PATCHes enabling the overdraft at the same time, both racing to create the
// companion: exactly one publishes overdraft_enabled, and one companion exists.
func TestIntegration_ConfigKeyPatch_ConcurrentEnablesPublishOnce(t *testing.T) {
	h := newConfigPatchHarness(t)

	for round := range 5 {
		seed := newConfigPatchBalance(fmt.Sprintf("@config-a10c-%d", round))
		h.seedBalance(t, seed)
		h.forgetEvents()

		update := mmodel.UpdateBalance{Settings: overdraftSettings(true, "")}
		start := make(chan struct{})
		results := make(chan error, 2)

		for range 2 {
			go func() {
				<-start
				results <- h.patch(t.Context(), seed, update)
			}()
		}

		close(start)

		for range 2 {
			require.NoError(t, <-results, "round %d", round)
		}

		assert.Equal(t, []string{h.companionID(t, seed)}, h.events(t, "overdraft_enabled"), "round %d", round)
	}
}

// A balance deleted and recreated under the same alias#key leaves the old balance's
// key, at a higher version, and its blob behind. A PATCH on the new balance replaces
// the key with its own row and answers success without touching the old blob, which
// the engine refuses anyway.
func TestIntegration_ConfigKeyPatch_RecreatedBalanceReplacesTheKeyOfTheDeletedOne(t *testing.T) {
	h := newConfigPatchHarness(t)

	deleted := newConfigPatchBalance("@config-a17")
	deleted.settings = overdraftSettings(true, "900")
	deleted.configVersion = 9
	h.seedBalance(t, deleted)
	h.writeRowToKey(t, deleted)
	oldBlob := h.cacheBlob(t, deleted, configPatchMoney{available: "5", onHold: "0", version: 30}, 9, 0)

	_, err := h.transactionDB.Exec(`UPDATE balance SET deleted_at = now() WHERE id = $1`, deleted.id)
	require.NoError(t, err)

	recreated := newConfigPatchBalance("@config-a17")
	recreated.accountID = deleted.accountID
	h.seedBalance(t, recreated)

	require.NoError(t, h.patch(t.Context(), recreated, mmodel.UpdateBalance{AllowReceiving: boolPointer(false)}))

	stored := h.storedConfig(t, recreated.ref())
	assert.Equal(t, recreated.id, stored.BalanceID, "the key now describes the recreated balance")
	assert.Equal(t, int64(1), stored.Version)
	assert.False(t, stored.AllowOverdraft, "nothing of the deleted balance's configuration survives")
	assert.False(t, stored.AllowReceiving)

	values, err := h.redisRepo.ReadConfigKeys(t.Context(), h.organizationID, h.ledgerID, []string{recreated.ref()}, nil)
	require.NoError(t, err)
	assert.True(t, values.Balances[recreated.ref()].Describes(recreated.id), "the next transaction reads the recreated balance's configuration")
	assert.Equal(t, oldBlob, h.blob(t, recreated.ref()), "the deleted balance's blob is left alone")
}

// A legacy blob (uppercase fields only, numeric money in non-canonical form, an
// indexed alias) keeps every money byte through an allow-flags PATCH: the copy never
// rounds nor re-encodes money.
func TestIntegration_ConfigKeyPatch_LegacyBlobKeepsItsMoneyBytes(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-a12")
	h.seedBalance(t, seed)

	legacy := `{"ID":"` + seed.id.String() + `","AccountID":"` + seed.accountID.String() + `","AccountType":"deposit","AssetCode":"USD",` +
		`"Alias":"0#@config-a12#default","Key":"default","Available":120.500,"OnHold":1E1,"Version":9,` +
		`"AllowSending":1,"AllowReceiving":1,"Direction":"credit","OverdraftUsed":"0","AllowOverdraft":0,` +
		`"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`
	require.NoError(t, h.cache.Client.Set(t.Context(), h.blobKey(seed.ref()), legacy, time.Hour).Err())

	require.NoError(t, h.patch(t.Context(), seed, mmodel.UpdateBalance{AllowSending: boolPointer(false)}))

	stored := h.blob(t, seed.ref())
	for _, exact := range []string{`"Available":120.500`, `"OnHold":1E1`, `"Version":9`, `"Alias":"0#@config-a12#default"`, `"AllowSending":0`} {
		assert.Contains(t, stored, exact)
	}

	assert.NotContains(t, stored, `"allowSending"`, "a legacy blob keeps its shape")
}

// A balance under a deletion marker refuses the copy (0019): the blob of a balance
// being deleted is never written, and the PATCH, which cannot confirm the copy,
// fails.
func TestIntegration_ConfigKeyPatch_DeletionMarkerRefusesTheCopy(t *testing.T) {
	h := newConfigPatchHarness(t)

	seed := newConfigPatchBalance("@config-0019")
	h.seedBalance(t, seed)
	blob := h.cacheBlob(t, seed, configPatchMoney{available: "10", onHold: "0", version: 2}, 0, 0)

	marker := "balance_delete_marker:{transactions}:" + h.organizationID.String() + ":" + h.ledgerID.String() + ":" + seed.ref()
	require.NoError(t, h.cache.Client.Set(t.Context(), marker, "1", time.Minute).Err())

	err := h.patch(t.Context(), seed, mmodel.UpdateBalance{AllowSending: boolPointer(false)})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "0019"), "the copy is refused by the deletion marker: %v", err)
	assert.False(t, pkg.IsBusinessError(err), "the refused copy is a technical error, not the business code that shares 0019")

	problem, ok := pkgHTTP.ProblemDetail(err)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, problem.Status, "the client is told to retry")
	assert.Equal(t, constant.ErrInternalServer.Error(), problem.Code)
	assert.Equal(t, blob, h.blob(t, seed.ref()))
}

// companionID reads the id of the parent's overdraft companion, which must be unique.
func (h *configPatchHarness) companionID(t *testing.T, parent configPatchBalance) string {
	t.Helper()

	rows, err := h.transactionDB.Query(`SELECT id FROM balance WHERE account_id = $1 AND key = $2 AND deleted_at IS NULL`,
		parent.accountID, constant.OverdraftBalanceKey)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	ids := []string{}

	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))

		ids = append(ids, id)
	}

	require.NoError(t, rows.Err())
	require.Len(t, ids, 1, "exactly one companion exists")

	return ids[0]
}

// An account PATCH blocking an account with three balances writes the account key
// once and copies the flag into every cached blob of the account, money untouched;
// a balance not in cache stays out of it.
func TestIntegration_ConfigKeyPatch_BlockedAccountWritesItsKeyAndEveryBlob(t *testing.T) {
	h := newConfigPatchHarness(t)
	h.withAccounts(t)

	accountID := h.seedAccount(t, "@config-a6")

	cached := []configPatchBalance{}

	for i, key := range []string{"default", "savings", "reserve"} {
		seed := newConfigPatchBalance("@config-a6")
		seed.accountID = accountID
		seed.key = key
		h.seedBalance(t, seed)

		if i < 2 {
			cached = append(cached, seed)
		}
	}

	before := map[string]string{}
	for _, seed := range cached {
		before[seed.ref()] = h.cacheBlob(t, seed, configPatchMoney{available: "77.7", onHold: "1", version: 5}, 0, 0)
	}

	_, err := h.uc.UpdateAccount(t.Context(), h.organizationID, h.ledgerID, nil, accountID,
		&mmodel.UpdateAccountInput{Blocked: boolPointer(true)}, mmodel.HolderOffV1)
	require.NoError(t, err)

	accountKey := utils.AccountConfigKey(h.organizationID, h.ledgerID, accountID)
	assert.Equal(t, 1, h.hook.count(func(c patchCommand) bool { return isScriptOn(c, accountKey) }), "one account key write")

	raw, err := h.cache.Client.Get(t.Context(), accountKey).Bytes()
	require.NoError(t, err)

	stored, err := balancecache.DecodeAccountConfig(raw)
	require.NoError(t, err)
	assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: true}, stored)

	for _, seed := range cached {
		after := h.blob(t, seed.ref())
		requireSameMoney(t, before[seed.ref()], after)

		fields := blobFieldsOf(t, after)
		assert.Equal(t, `1`, string(fields["Blocked"]), seed.ref())
		assert.Equal(t, `true`, string(fields["blocked"]), seed.ref())
		assert.Equal(t, `"1"`, string(fields[balancecache.BlobAccountConfigVersionField]), seed.ref())
	}

	exists, err := h.cache.Client.Exists(t.Context(), h.blobKey("@config-a6#reserve")).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "a balance not in cache is not seeded by the PATCH")
}

// A blocked PATCH whose account key write fails fails closed with the account
// already blocked in PostgreSQL; the retry completes the key.
func TestIntegration_ConfigKeyPatch_BlockedAccountFailsClosedAndTheRetryCompletes(t *testing.T) {
	h := newConfigPatchHarness(t)
	h.withAccounts(t)

	accountID := h.seedAccount(t, "@config-a2-blocked")
	accountKey := utils.AccountConfigKey(h.organizationID, h.ledgerID, accountID)

	h.hook.intervene(failOnce(func(c patchCommand) bool { return isScriptOn(c, accountKey) }), nil)

	input := &mmodel.UpdateAccountInput{Blocked: boolPointer(true)}

	_, err := h.uc.UpdateAccount(t.Context(), h.organizationID, h.ledgerID, nil, accountID, input, mmodel.HolderOffV1)
	require.Error(t, err)
	assert.False(t, pkg.IsBusinessError(err))

	var blocked bool
	require.NoError(t, h.onboardingDB.QueryRow(`SELECT blocked FROM account WHERE id = $1`, accountID).Scan(&blocked))
	assert.True(t, blocked, "PostgreSQL already holds the new flag")

	h.hook.reset()

	_, err = h.uc.UpdateAccount(t.Context(), h.organizationID, h.ledgerID, nil, accountID, input, mmodel.HolderOffV1)
	require.NoError(t, err)

	values, err := h.redisRepo.ReadConfigKeys(t.Context(), h.organizationID, h.ledgerID, nil, []uuid.UUID{accountID})
	require.NoError(t, err)
	assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 1, Blocked: true}, values.Accounts[accountID])
}

// seedAccount writes one unblocked account row.
func (h *configPatchHarness) seedAccount(t *testing.T, alias string) uuid.UUID {
	t.Helper()

	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	_, err := h.onboardingDB.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, 'Config Patch', 'USD', $2, $3, 'ACTIVE', $4, 'deposit', false, $5, $5)
	`, accountID, h.organizationID, h.ledgerID, alias, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err, "failed to seed the account")

	return accountID
}
