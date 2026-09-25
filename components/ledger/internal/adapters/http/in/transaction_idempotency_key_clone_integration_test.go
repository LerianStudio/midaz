// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redisadapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// A Huma header string is a view over fasthttp's request buffer, which the server rewrites
// when the next request arrives on the same keep-alive connection. The idempotency value is
// stored from a goroutine that runs after the handler returns, so a key that is still a view
// by then names whatever request now occupies the buffer: request A's transaction lands in
// request B's slot, B replays A instead of posting its own transfer, and an honest retry of A
// meets its own empty placeholder (409 0084).
//
// These tests make that reuse deterministic. Key A is a string backed by a byte slice the
// test owns, exactly the shape fasthttp hands Huma, and the Redis repository rewrites those
// bytes with key B's the moment A's claim returns. The claim is synchronous and happens
// before the value write is launched, so no sleep orders the overwrite.
//
// NOT PARALLEL: setupTestInfra and the Huma error hooks share process-global state (see
// transaction_handler_v2_integration_test.go).

const (
	bufferReuseKeyA = "key-A-0000000031"
	bufferReuseKeyB = "key-B-0000000082"
)

// idempotentCreate issues one create with the given X-Idempotency value and returns the
// resulting transaction ID and the X-Idempotency-Replayed header value.
type idempotentCreate func(ctx context.Context, idempotencyKey string) (transactionID, replayed string, err error)

// overwriteKeyAfterClaim is the real Redis repository with one addition: when the
// idempotency slot named claimKey is claimed, it runs rewrite once, after the claim.
type overwriteKeyAfterClaim struct {
	redisadapter.RedisRepository
	claimKey string
	rewrite  func()
}

func (r *overwriteKeyAfterClaim) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	claimed, err := r.RedisRepository.SetNX(ctx, key, value, ttl)

	if key == r.claimKey && r.rewrite != nil {
		r.rewrite()
		r.rewrite = nil
	}

	return claimed, err
}

func TestIntegration_TransactionCreate_IdempotencyKeySurvivesRequestBufferReuse(t *testing.T) {
	t.Run("v1 json", func(t *testing.T) {
		infra := setupBufferReuseInfra(t)

		body := []byte(equivalentV1Body)

		create := func(ctx context.Context, idempotencyKey string) (string, string, error) {
			out, err := infra.handler.CreateTransactionJSON(ctx, &CreateTransactionJSONRequest{
				OrganizationID: infra.orgID.String(),
				LedgerID:       infra.ledgerID.String(),
				IdempotencyKey: idempotencyKey,
				RawBody:        body,
			})
			if err != nil {
				return "", "", err
			}

			return out.Body.ID, out.IdempotencyReplayed, nil
		}

		assertEachRequestKeepsItsOwnSlot(t, infra, create)
	})

	t.Run("v2 direct", func(t *testing.T) {
		infra := setupBufferReuseInfra(t)

		body := []byte(v2ScopedBody(equivalentV2Body, infra.orgID, infra.ledgerID))

		create := func(ctx context.Context, idempotencyKey string) (string, string, error) {
			out, err := infra.handler.CreateTransactionDirectV2(ctx, &CreateTransactionInputV2{
				IdempotencyKey: idempotencyKey,
				RawBody:        body,
			})
			if err != nil {
				return "", "", err
			}

			return out.Body.ID, out.IdempotencyReplayed, nil
		}

		assertEachRequestKeepsItsOwnSlot(t, infra, create)
	})
}

func setupBufferReuseInfra(t *testing.T) *testInfra {
	t.Helper()

	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	return infra
}

// assertEachRequestKeepsItsOwnSlot runs request A with a key whose bytes become key B's once
// A's claim returns, then checks where A's transaction was stored, that a create with key B
// posts its own transfer, and that an honest retry of A replays A.
func assertEachRequestKeepsItsOwnSlot(t *testing.T, infra *testInfra, create idempotentCreate) {
	t.Helper()

	ctx := context.Background()

	require.Len(t, bufferReuseKeyB, len(bufferReuseKeyA), "the overwrite reuses key A's bytes, so both keys need the same length")

	keyABuffer := []byte(bufferReuseKeyA)
	keyAView := unsafe.String(&keyABuffer[0], len(keyABuffer))

	infra.handler.Command.TransactionRedisRepo = &overwriteKeyAfterClaim{
		RedisRepository: infra.redisRepo,
		claimKey:        utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, bufferReuseKeyA),
		rewrite:         func() { copy(keyABuffer, bufferReuseKeyB) },
	}

	txA, replayedA, err := create(ctx, keyAView)
	require.NoError(t, err, "request A must be created")
	assert.Equal(t, "false", replayedA, "request A is a first create")
	require.Equal(t, bufferReuseKeyB, keyAView, "the test must rewrite key A's bytes after the claim")

	storedUnder, storedValue := waitForIdempotencyValue(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, bufferReuseKeyA, bufferReuseKeyB)
	assert.Equal(t, bufferReuseKeyA, storedUnder, "request A's transaction must be stored under request A's own key")
	assert.Contains(t, storedValue, txA, "the stored value must be request A's transaction")

	slotB, err := infra.redisRepo.Get(ctx, utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, bufferReuseKeyB))
	require.NoError(t, err, "reading key B's slot")
	assert.Empty(t, slotB, "key B's slot must hold nothing until request B arrives")

	txB, replayedB, err := create(ctx, bufferReuseKeyB)
	require.NoError(t, err, "request B must be created")
	assert.Equal(t, "false", replayedB, "request B carries a new key and must not be a replay")
	assert.NotEqual(t, txA, txB, "request B must post its own transaction")
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "requests A and B must each persist a transaction")

	retryTx, retryReplayed, err := create(ctx, bufferReuseKeyA)
	require.NoError(t, err, "an honest retry of request A must replay, not report the key as in use")
	assert.Equal(t, "true", retryReplayed, "the retry of request A must be a replay")
	assert.Equal(t, txA, retryTx, "the retry of request A must return request A's transaction")
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "a replay must not persist another transaction")
}

// waitForIdempotencyValue polls Redis until one of the given keys' slots holds a stored
// transaction, and returns that key and its value. Polling is bounded by a fixed retry count
// and only waits for the asynchronous value write; it orders nothing in the scenario.
func waitForIdempotencyValue(t *testing.T, ctx context.Context, redisRepo redisadapter.RedisRepository, orgID, ledgerID uuid.UUID, keys ...string) (key, value string) {
	t.Helper()

	for range 400 {
		for _, candidate := range keys {
			stored, err := redisRepo.Get(ctx, utils.IdempotencyInternalKey(orgID, ledgerID, candidate))
			if err == nil && strings.HasPrefix(strings.TrimSpace(stored), "{") {
				return candidate, stored
			}
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("no idempotency value was stored under any of %q within the retry budget", keys)

	return "", ""
}
