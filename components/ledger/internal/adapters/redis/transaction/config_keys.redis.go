// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"time"

	libBackoff "github.com/LerianStudio/lib-commons/v7/commons/backoff"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// The configuration keys' names, value format, write rule, expiry and the blob
// copy are specified in package balancecache (config.go). This file is their I/O.

//go:embed scripts/write_config_key.lua
var writeConfigKeyLua string

var writeConfigKeyScript = redis.NewScript(writeConfigKeyLua)

const (
	balanceConfigIdentityField = "balanceId"
	accountConfigIdentityField = "accountId"
)

// ConfigWriteOutcome tells what a conditional configuration write did.
type ConfigWriteOutcome int

const (
	// ConfigWritten means the key now holds the written value with a fresh expiry.
	ConfigWritten ConfigWriteOutcome = iota + 1
	// ConfigUnchanged means the key already held the same version with the same
	// content; nothing was written and the expiry was not touched.
	ConfigUnchanged
	// ConfigSuperseded means the key holds a newer version of the same row, which
	// the write returns instead; nothing was written.
	ConfigSuperseded
)

// ErrConfigVersionConflict reports a key holding different content under the
// version being written. A version identifies one committed row state, so this is
// an inconsistency, not a retry, and the stored value is left in place.
var ErrConfigVersionConflict = errors.New("configuration key holds different content under the same version")

// BlobCopyOutcome tells what copying a configuration into a balance blob did.
type BlobCopyOutcome int

const (
	// BlobCopyWritten means the copy and its version were written.
	BlobCopyWritten BlobCopyOutcome = iota + 1
	// BlobCopyAlreadyCurrent means the blob already held a newer copy, or a copy of
	// that version with the same content; nothing was written.
	BlobCopyAlreadyCurrent
	// BlobCopyBlobAbsent means there is no blob to copy into: the next load seeds it
	// from PostgreSQL.
	BlobCopyBlobAbsent
	// BlobCopyConflict means the blob kept changing between the read and the
	// compare-and-set for every attempt, and the copy may still be missing.
	BlobCopyConflict
	// BlobCopyOfAnotherBalance means the blob under that alias#key belongs to
	// another balance or account, such as one deleted and recreated under the same
	// alias#key. The engine refuses such a blob, so there is no copy to confirm and
	// nothing was written.
	BlobCopyOfAnotherBalance
)

// Confirmed reports whether the blob is known not to hold an older copy of the
// configuration: written, already current, absent, or a blob of another balance. A
// conflict is not confirmed.
func (o BlobCopyOutcome) Confirmed() bool {
	return o == BlobCopyWritten || o == BlobCopyAlreadyCurrent || o == BlobCopyBlobAbsent || o == BlobCopyOfAnotherBalance
}

func (o BlobCopyOutcome) String() string {
	switch o {
	case BlobCopyWritten:
		return "written"
	case BlobCopyAlreadyCurrent:
		return "already_current"
	case BlobCopyBlobAbsent:
		return "blob_absent"
	case BlobCopyConflict:
		return "conflict"
	case BlobCopyOfAnotherBalance:
		return "of_another_balance"
	default:
		return "unknown"
	}
}

// ConfigKeyValues holds the configuration keys found by one batch read. A missing
// or unreadable key has no entry. A balance entry may describe another balance than
// the caller's (BalanceConfig.Describes): such an entry is absent for that caller.
type ConfigKeyValues struct {
	Balances map[string]accounting.BalanceConfig
	Accounts map[uuid.UUID]accounting.AccountConfig
}

// ReadConfigKeys reads the configuration keys of the given balances (alias#key)
// and accounts in one MGET. Reading never changes a key's expiry. A value that
// cannot be decoded is reported as missing, so the caller reloads it from
// PostgreSQL and the next write replaces it.
func (rr *RedisConsumerRepository) ReadConfigKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, balanceRefs []string, accountIDs []uuid.UUID) (ConfigKeyValues, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_config_keys")
	defer span.End()

	span.SetAttributes(
		attribute.Int("app.request.balance_refs_count", len(balanceRefs)),
		attribute.Int("app.request.account_ids_count", len(accountIDs)),
	)

	result := ConfigKeyValues{
		Balances: make(map[string]accounting.BalanceConfig, len(balanceRefs)),
		Accounts: make(map[uuid.UUID]accounting.AccountConfig, len(accountIDs)),
	}

	keys := make([]string, 0, len(balanceRefs)+len(accountIDs))
	for _, ref := range balanceRefs {
		keys = append(keys, utils.BalanceConfigKey(organizationID, ledgerID, ref))
	}

	for _, accountID := range accountIDs {
		keys = append(keys, utils.AccountConfigKey(organizationID, ledgerID, accountID))
	}

	if len(keys) == 0 {
		return result, nil
	}

	values, err := rr.MGet(ctx, keys)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read configuration keys", err)

		return ConfigKeyValues{}, fmt.Errorf("read configuration keys: %w", err)
	}

	unreadable := 0

	for i, ref := range balanceRefs {
		raw, exists := values[keys[i]]
		if !exists {
			continue
		}

		config, decodeErr := balancecache.DecodeBalanceConfig([]byte(raw))
		if decodeErr != nil {
			unreadable++

			logger.Log(ctx, libLog.LevelWarn, "Unreadable balance configuration key, reloading from the database",
				libLog.String("balance_ref", ref), libLog.Err(decodeErr))

			continue
		}

		result.Balances[ref] = config
	}

	for i, accountID := range accountIDs {
		raw, exists := values[keys[len(balanceRefs)+i]]
		if !exists {
			continue
		}

		config, decodeErr := balancecache.DecodeAccountConfig([]byte(raw))
		if decodeErr != nil || config.AccountID != accountID {
			unreadable++

			logger.Log(ctx, libLog.LevelWarn, "Unreadable account configuration key, reloading from the database",
				libLog.String("account_id", accountID.String()), libLog.Err(decodeErr))

			continue
		}

		result.Accounts[accountID] = config
	}

	span.SetAttributes(
		attribute.Int("app.response.config_keys.found", len(result.Balances)+len(result.Accounts)),
		attribute.Int("app.response.config_keys.unreadable", unreadable),
	)

	return result, nil
}

// WriteBalanceConfig writes the configuration key of one balance (alias#key)
// under the conditional rule and returns the value the key holds afterwards: the
// written one, or the newer one already stored. A stored value of another balance
// is replaced whatever its version.
func (rr *RedisConsumerRepository) WriteBalanceConfig(ctx context.Context, organizationID, ledgerID uuid.UUID, balanceRef string, config accounting.BalanceConfig) (accounting.BalanceConfig, ConfigWriteOutcome, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.write_balance_config")
	defer span.End()

	value, err := balancecache.EncodeBalanceConfig(config)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to encode balance configuration", err)

		return accounting.BalanceConfig{}, 0, fmt.Errorf("encode balance configuration: %w", err)
	}

	status, stored, err := rr.writeConfigKey(ctx, utils.BalanceConfigKey(organizationID, ledgerID, balanceRef),
		value, config.Version, balanceConfigIdentityField, config.BalanceID.String())
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to write balance configuration key", err)

		return accounting.BalanceConfig{}, 0, err
	}

	if status == configKeyWritten {
		span.SetAttributes(attribute.String("app.response.config_key.outcome", "written"))

		return config, ConfigWritten, nil
	}

	winner, err := balancecache.DecodeBalanceConfig([]byte(stored))
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode stored balance configuration", err)

		return accounting.BalanceConfig{}, 0, fmt.Errorf("decode stored balance configuration: %w", err)
	}

	if status == configKeyNewerStored {
		span.SetAttributes(attribute.String("app.response.config_key.outcome", "superseded"))

		return winner, ConfigSuperseded, nil
	}

	if !winner.Equal(config) {
		libOpentelemetry.HandleSpanError(span, "Balance configuration key conflicts under the same version", ErrConfigVersionConflict)

		return winner, 0, fmt.Errorf("balance configuration %s version %d: %w", config.BalanceID, config.Version, ErrConfigVersionConflict)
	}

	span.SetAttributes(attribute.String("app.response.config_key.outcome", "unchanged"))

	return winner, ConfigUnchanged, nil
}

// WriteAccountConfig writes the configuration key of one account under the
// conditional rule and returns the value the key holds afterwards.
func (rr *RedisConsumerRepository) WriteAccountConfig(ctx context.Context, organizationID, ledgerID uuid.UUID, config accounting.AccountConfig) (accounting.AccountConfig, ConfigWriteOutcome, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.write_account_config")
	defer span.End()

	value, err := balancecache.EncodeAccountConfig(config)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to encode account configuration", err)

		return accounting.AccountConfig{}, 0, fmt.Errorf("encode account configuration: %w", err)
	}

	status, stored, err := rr.writeConfigKey(ctx, utils.AccountConfigKey(organizationID, ledgerID, config.AccountID),
		value, config.Version, accountConfigIdentityField, config.AccountID.String())
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to write account configuration key", err)

		return accounting.AccountConfig{}, 0, err
	}

	if status == configKeyWritten {
		span.SetAttributes(attribute.String("app.response.config_key.outcome", "written"))

		return config, ConfigWritten, nil
	}

	winner, err := balancecache.DecodeAccountConfig([]byte(stored))
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode stored account configuration", err)

		return accounting.AccountConfig{}, 0, fmt.Errorf("decode stored account configuration: %w", err)
	}

	if status == configKeyNewerStored {
		span.SetAttributes(attribute.String("app.response.config_key.outcome", "superseded"))

		return winner, ConfigSuperseded, nil
	}

	if winner != config {
		libOpentelemetry.HandleSpanError(span, "Account configuration key conflicts under the same version", ErrConfigVersionConflict)

		return winner, 0, fmt.Errorf("account configuration %s version %d: %w", config.AccountID, config.Version, ErrConfigVersionConflict)
	}

	span.SetAttributes(attribute.String("app.response.config_key.outcome", "unchanged"))

	return winner, ConfigUnchanged, nil
}

const (
	configKeySameVersion int64 = 0
	configKeyWritten     int64 = 1
	configKeyNewerStored int64 = 2
)

func (rr *RedisConsumerRepository) writeConfigKey(ctx context.Context, key string, value []byte, version int64, identityField, identity string) (int64, string, error) {
	prefixedKey, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		return 0, "", fmt.Errorf("namespace configuration key: %w", err)
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("get redis client: %w", err)
	}

	ttlSeconds := strconv.FormatInt(int64(cachepolicy.ConfigKeyTTL/time.Second), 10)

	reply, err := writeConfigKeyScript.Run(ctx, rds, []string{prefixedKey},
		string(value), strconv.FormatInt(version, 10), identityField, identity, ttlSeconds).Slice()
	if err != nil {
		return 0, "", fmt.Errorf("write configuration key: %w", err)
	}

	if len(reply) != 2 {
		return 0, "", fmt.Errorf("write configuration key: unexpected reply length %d", len(reply))
	}

	status, statusOK := reply[0].(int64)
	stored, storedOK := reply[1].(string)

	if !statusOK || !storedOK || status < configKeySameVersion || status > configKeyNewerStored {
		return 0, "", fmt.Errorf("write configuration key: unexpected reply %v", reply)
	}

	return status, stored, nil
}

// CopyBalanceConfigToBlob makes the balance blob (alias#key) hold a complete copy
// of config, for releases that read the configuration from the blob. It writes
// only when the blob's copy is older, or of the same version with other content
// (see balancecache.CopyBalanceConfig), through a compare-and-set on the exact bytes
// it read: if anything changed the blob in between — a transaction moving money,
// most of all — the write is refused and the blob is read again, so a newer money
// state is never overwritten. Money bytes are carried over unchanged; see
// balancecache.CopyBalanceConfig.
func (rr *RedisConsumerRepository) CopyBalanceConfigToBlob(ctx context.Context, organizationID, ledgerID uuid.UUID, balanceRef string, config accounting.BalanceConfig) (BlobCopyOutcome, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.copy_balance_config_to_blob")
	defer span.End()

	span.SetAttributes(attribute.Int64("app.request.config.version", config.Version))

	outcome, attempts, err := rr.copyConfigToBlob(ctx, utils.BalanceInternalKey(organizationID, ledgerID, balanceRef), nil,
		func(raw []byte) ([]byte, bool, error) { return balancecache.CopyBalanceConfig(raw, config) })

	span.SetAttributes(
		attribute.String("app.response.blob_copy.outcome", outcome.String()),
		attribute.Int("app.response.blob_copy.attempts", attempts),
	)

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to copy balance configuration to the blob", err)

		return 0, err
	}

	return outcome, nil
}

// CopyAccountConfigToBlobs makes every blob of the account's balances (alias#key)
// hold the account's blocked flag at config's version. The blobs are read in one
// MGET and only the ones holding an older or a divergent copy are written, each
// through the same exact-bytes compare-and-set as CopyBalanceConfigToBlob. The
// first error stops the copy; the outcomes of the blobs already handled are
// returned with it.
func (rr *RedisConsumerRepository) CopyAccountConfigToBlobs(ctx context.Context, organizationID, ledgerID uuid.UUID, balanceRefs []string, config accounting.AccountConfig) (map[string]BlobCopyOutcome, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.copy_account_config_to_blobs")
	defer span.End()

	span.SetAttributes(
		attribute.Int64("app.request.config.version", config.Version),
		attribute.Int("app.request.balance_refs_count", len(balanceRefs)),
	)

	outcomes := make(map[string]BlobCopyOutcome, len(balanceRefs))
	if len(balanceRefs) == 0 {
		return outcomes, nil
	}

	keys := make([]string, 0, len(balanceRefs))
	for _, ref := range balanceRefs {
		keys = append(keys, utils.BalanceInternalKey(organizationID, ledgerID, ref))
	}

	observed, err := rr.MGet(ctx, keys)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read the account's balance blobs", err)

		return outcomes, fmt.Errorf("read balance blobs for account configuration copy: %w", err)
	}

	conflicts := 0

	for i, ref := range balanceRefs {
		raw, exists := observed[keys[i]]
		if !exists {
			outcomes[ref] = BlobCopyBlobAbsent

			continue
		}

		first := []byte(raw)

		outcome, _, copyErr := rr.copyConfigToBlob(ctx, keys[i], first,
			func(raw []byte) ([]byte, bool, error) { return balancecache.CopyAccountConfig(raw, config) })
		if copyErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to copy account configuration to a blob", copyErr)

			return outcomes, fmt.Errorf("copy account configuration to %s: %w", ref, copyErr)
		}

		if outcome == BlobCopyConflict {
			conflicts++
		}

		outcomes[ref] = outcome
	}

	span.SetAttributes(attribute.Int("app.response.blob_copy.conflicts", conflicts))

	return outcomes, nil
}

// maxBalanceSettingsCASAttempts bounds the configuration copy retries caused by
// confirmed concurrent cache mutations. Redis transport errors are returned immediately because an
// EVAL error may be commit-ambiguous and the client controls transport retries.
//
// The engine never writes the configuration copy of a blob that exists, so on a hot
// balance the PATCH has to win the compare-and-set against the transactions itself.
// A lost compare-and-set waits a short full-jitter pause before reading again, which
// keeps the retries from falling into step with the engine writes; the attempts and
// the pauses are bounded, so a balance that never goes quiet fails the PATCH with a
// technical error the client can retry.
const (
	maxBalanceSettingsCASAttempts  = 8
	balanceSettingsCASBackoffBase  = time.Millisecond
	balanceSettingsCASBackoffLimit = 16 * time.Millisecond
)

// balanceSettingsCASBackoff is the pause after the attempt-th lost compare-and-set:
// a full-jitter share of an exponential step from balanceSettingsCASBackoffBase,
// capped at balanceSettingsCASBackoffLimit.
func balanceSettingsCASBackoff(attempt int) time.Duration {
	return libBackoff.FullJitter(min(libBackoff.Exponential(balanceSettingsCASBackoffBase, attempt-1), balanceSettingsCASBackoffLimit))
}

// copyConfigToBlob runs the read / prepare / compare-and-set loop on one blob.
// first, when set, is the blob already read by the caller and serves as the first
// observation. It reuses the settings compare-and-set script, which also refuses a
// balance under a deletion marker. A blob of another balance is left alone.
func (rr *RedisConsumerRepository) copyConfigToBlob(ctx context.Context, blobKey string, first []byte, prepare func([]byte) ([]byte, bool, error)) (BlobCopyOutcome, int, error) {
	prefixedKey, err := tenantKeyFromContextOrError(ctx, blobKey)
	if err != nil {
		return 0, 0, fmt.Errorf("namespace balance blob key: %w", err)
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("get redis client: %w", err)
	}

	ttlSeconds := strconv.FormatInt(int64(balanceCacheSettingsTTL/time.Second), 10)
	observed := first

	for attempt := 1; attempt <= maxBalanceSettingsCASAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, attempt, err
		}

		if observed == nil {
			observed, err = rds.Get(ctx, prefixedKey).Bytes()
			if errors.Is(err, redis.Nil) {
				return BlobCopyBlobAbsent, attempt, nil
			}

			if err != nil {
				return 0, attempt, fmt.Errorf("read balance blob for configuration copy: %w", err)
			}
		}

		replacement, current, err := prepare(observed)
		if errors.Is(err, balancecache.ErrBlobOfAnotherBalance) {
			return BlobCopyOfAnotherBalance, attempt, nil
		}

		if err != nil {
			return 0, attempt, fmt.Errorf("prepare configuration copy: %w", err)
		}

		if current {
			return BlobCopyAlreadyCurrent, attempt, nil
		}

		result, err := updateBalanceSettingsScript.Run(ctx, rds, []string{prefixedKey}, observed, replacement, ttlSeconds).Int64()
		if err != nil {
			return 0, attempt, fmt.Errorf("compare-and-set configuration copy: %w", err)
		}

		switch result {
		case 1:
			return BlobCopyWritten, attempt, nil
		case 0:
			return BlobCopyBlobAbsent, attempt, nil
		case 2:
			observed = nil
		default:
			return 0, attempt, fmt.Errorf("compare-and-set configuration copy: unexpected result %d", result)
		}

		if attempt < maxBalanceSettingsCASAttempts {
			if err := libBackoff.WaitContext(ctx, balanceSettingsCASBackoff(attempt)); err != nil {
				return 0, attempt, err
			}
		}
	}

	return BlobCopyConflict, maxBalanceSettingsCASAttempts, nil
}
