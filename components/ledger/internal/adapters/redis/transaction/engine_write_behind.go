// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisgo "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

const engineMaterializedTransactionFormatVersion = 1

var ErrEngineWriteBehindNotFound = errors.New("engine write-behind evidence not found")

// EngineWriteBehindRepository is the narrow indexed-evidence port used by
// point lookup. It stays separate from RedisRepository so existing cache users
// and test doubles do not acquire unrelated financial-evidence methods.
type EngineWriteBehindRepository interface {
	GetEngineTransactionIndex(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) ([]byte, error)
	GetEngineTransactionEvidence(ctx context.Context, organizationID, ledgerID, transactionID, executionID uuid.UUID) (envelope, receipt []byte, err error)
	GetEngineMaterializedTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*EngineMaterializedTransaction, error)
	MaterializeEngineTransaction(ctx context.Context, organizationID, ledgerID, transactionID, executionID uuid.UUID, payload []byte, ttl time.Duration) (bool, error)
}

// EngineMaterializedTransaction is only an accelerator. ExecutionID fences an
// older reconstruction from replacing or satisfying a newer lifecycle state.
type EngineMaterializedTransaction struct {
	FormatVersion int       `json:"formatVersion"`
	ExecutionID   uuid.UUID `json:"executionId"`
	Payload       []byte    `json:"payload"`
}

//go:embed scripts/materialize_engine_transaction.lua
var materializeEngineTransactionLua string

var materializeEngineTransactionScript = redisgo.NewScript(cachepolicy.LuaSource(materializeEngineTransactionLua))

func (rr *RedisConsumerRepository) GetEngineTransactionIndex(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) ([]byte, error) {
	keys, err := engineWriteBehindKeys(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := client.HGet(ctx, keys.index, transactionID.String()).Bytes()
	if errors.Is(err, redisgo.Nil) {
		return nil, ErrEngineWriteBehindNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("read engine transaction index: %w", err)
	}

	return raw, nil
}

func (rr *RedisConsumerRepository) GetEngineTransactionEvidence(ctx context.Context, organizationID, ledgerID, transactionID, executionID uuid.UUID) ([]byte, []byte, error) {
	keys, err := engineWriteBehindKeys(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, nil, err
	}

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	pipe := client.Pipeline()
	evidenceCmd := pipe.HGet(ctx, keys.evidence, transactionID.String()+":"+executionID.String())
	recoveryCmd := pipe.HGet(ctx, keys.recovery, transactionID.String()+":"+executionID.String())
	receiptCmd := pipe.HGet(ctx, keys.receipt, executionID.String())

	_, execErr := pipe.Exec(ctx)
	if execErr != nil && !errors.Is(execErr, redisgo.Nil) {
		return nil, nil, fmt.Errorf("read engine transaction evidence: %w", execErr)
	}

	envelope, envelopeErr := evidenceCmd.Bytes()
	if errors.Is(envelopeErr, redisgo.Nil) {
		envelope, envelopeErr = recoveryCmd.Bytes()
	}

	receipt, receiptErr := receiptCmd.Bytes()
	if errors.Is(envelopeErr, redisgo.Nil) || errors.Is(receiptErr, redisgo.Nil) {
		return nil, nil, ErrEngineWriteBehindNotFound
	}

	if envelopeErr != nil || receiptErr != nil {
		return nil, nil, fmt.Errorf("read engine transaction evidence: envelope=%v receipt=%v", envelopeErr, receiptErr)
	}

	return envelope, receipt, nil
}

func (rr *RedisConsumerRepository) GetEngineMaterializedTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*EngineMaterializedTransaction, error) {
	keys, err := engineWriteBehindKeys(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := client.Get(ctx, keys.materialized).Bytes()
	if errors.Is(err, redisgo.Nil) {
		return nil, ErrEngineWriteBehindNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("read materialized engine transaction: %w", err)
	}

	var record EngineMaterializedTransaction

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&record); err != nil || record.FormatVersion != engineMaterializedTransactionFormatVersion || record.ExecutionID == uuid.Nil || len(record.Payload) == 0 {
		return nil, fmt.Errorf("decode materialized engine transaction: invalid versioned record")
	}

	return &record, nil
}

func (rr *RedisConsumerRepository) MaterializeEngineTransaction(ctx context.Context, organizationID, ledgerID, transactionID, executionID uuid.UUID, payload []byte, ttl time.Duration) (bool, error) {
	if organizationID == uuid.Nil || ledgerID == uuid.Nil || transactionID == uuid.Nil || executionID == uuid.Nil || len(payload) == 0 || ttl <= 0 {
		return false, fmt.Errorf("invalid engine transaction materialization")
	}

	keys, err := engineWriteBehindKeys(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return false, err
	}

	record, err := json.Marshal(EngineMaterializedTransaction{FormatVersion: engineMaterializedTransactionFormatVersion, ExecutionID: executionID, Payload: payload})
	if err != nil {
		return false, fmt.Errorf("encode materialized engine transaction: %w", err)
	}

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	result, err := materializeEngineTransactionScript.Run(ctx, client, []string{keys.index, keys.materialized}, transactionID.String(), executionID.String(), record, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("materialize engine transaction: %w", err)
	}

	return result == 1, nil
}

type engineWriteBehindRedisKeys struct {
	index        string
	recovery     string
	evidence     string
	receipt      string
	materialized string
}

func engineWriteBehindKeys(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (engineWriteBehindRedisKeys, error) {
	if organizationID == uuid.Nil || ledgerID == uuid.Nil || transactionID == uuid.Nil {
		return engineWriteBehindRedisKeys{}, fmt.Errorf("invalid engine write-behind scope")
	}

	scope := organizationID.String() + ":" + ledgerID.String()
	keys := []string{
		"engine:" + cachepolicy.HashTag + ":transaction-index:" + scope,
		cachepolicy.EngineRecoverQueue,
		"engine:" + cachepolicy.HashTag + ":evidence:" + scope,
		"engine:" + cachepolicy.HashTag + ":receipts:" + scope,
		"engine:" + cachepolicy.HashTag + ":materialized:" + scope + ":" + transactionID.String(),
	}

	prefixed, err := tenantKeysFromContext(ctx, keys)
	if err != nil {
		return engineWriteBehindRedisKeys{}, err
	}

	return engineWriteBehindRedisKeys{
		index:        prefixed[0],
		recovery:     prefixed[1],
		evidence:     prefixed[2],
		receipt:      prefixed[3],
		materialized: prefixed[4],
	}, nil
}

var _ EngineWriteBehindRepository = (*RedisConsumerRepository)(nil)
