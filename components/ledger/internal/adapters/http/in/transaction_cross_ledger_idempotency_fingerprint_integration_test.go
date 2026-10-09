// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// crossLedgerIdempotencyPair is two cross-ledger enabled ledgers of the
// fixture's organization, ordered so ledgerA is the coordination scope of
// every group whose origin is in ledgerA.
type crossLedgerIdempotencyPair struct {
	ledgerA, ledgerB uuid.UUID
	raw              string
}

func (fixture *atomicBatchHTTPIntegrationFixture) crossLedgerIdempotencyPair(t *testing.T, name string) crossLedgerIdempotencyPair {
	t.Helper()

	ledgerA, ledgerB := fixture.newLedger(t), fixture.newLedger(t)
	if ledgerA.String() > ledgerB.String() {
		ledgerA, ledgerB = ledgerB, ledgerA
	}

	fixture.setCrossLedgerEnabled(t, ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgerB, true)

	source, destination := "@"+name+"-source", "@"+name+"-destination"
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerA, source, "@external/USD", 100)
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerB, "@external/USD", destination, 100)

	request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "cross-ledger idempotency "+name, source, destination, 10)
	request.Credits[0].LedgerID = ledgerB.String()

	raw, err := json.Marshal(request)
	require.NoError(t, err)

	return crossLedgerIdempotencyPair{ledgerA: ledgerA, ledgerB: ledgerB, raw: string(raw)}
}

// postCrossLedger posts body verbatim, so a test controls the exact bytes the
// server fingerprints.
func (fixture *atomicBatchHTTPIntegrationFixture) postCrossLedger(t *testing.T, action, body, key string) (int, []byte, *http.Response) {
	t.Helper()

	response := postTransaction(t, fixture.app, v2CreateURL(action), body, key)

	return response.StatusCode, drainBody(t, response), response
}

func (fixture *atomicBatchHTTPIntegrationFixture) groupIdempotencyRedisKey(ledgerID uuid.UUID, effectiveKey string) string {
	return utils.AtomicTransactionBatchIdempotencyInternalKey(fixture.infra.orgID, ledgerID, effectiveKey)
}

func (fixture *atomicBatchHTTPIntegrationFixture) groupIdempotencyRecord(t *testing.T, redisKey string) redistransaction.AtomicTransactionBatchIdempotencyRecord {
	t.Helper()

	stored, err := fixture.infra.redisContainer.Client.Get(context.Background(), redisKey).Bytes()
	require.NoError(t, err, "the group must leave an idempotency record at %s", redisKey)

	var record redistransaction.AtomicTransactionBatchIdempotencyRecord
	require.NoError(t, json.Unmarshal(stored, &record))

	return record
}

func countPendingTransactionsInLedger(t *testing.T, fixture *atomicBatchHTTPIntegrationFixture, ledgerID uuid.UUID) int {
	t.Helper()

	var count int
	require.NoError(t, fixture.infra.pgContainer.DB.QueryRow(
		`SELECT COUNT(*) FROM transaction WHERE ledger_id = $1 AND status = $2`, ledgerID, constant.PENDING,
	).Scan(&count))

	return count
}

func sha256Hex(source string) string {
	digest := sha256.Sum256([]byte(source))

	return hex.EncodeToString(digest[:])
}

// reserializedRequestBody returns the same JSON request with every object's
// properties in reverse order and indented, keeping array order and number
// literals intact.
func reserializedRequestBody(t *testing.T, raw string) string {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var value any
	require.NoError(t, decoder.Decode(&value))

	var buffer bytes.Buffer
	writeReversedJSON(t, &buffer, value, "")

	reserialized := buffer.String()
	require.NotEqual(t, raw, reserialized)

	return reserialized
}

func writeReversedJSON(t *testing.T, buffer *bytes.Buffer, value any, indent string) {
	t.Helper()

	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}

		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		buffer.WriteString("{\n")

		for index, key := range keys {
			encodedKey, err := json.Marshal(key)
			require.NoError(t, err)
			buffer.WriteString(indent + "  ")
			buffer.Write(encodedKey)
			buffer.WriteString(" : ")
			writeReversedJSON(t, buffer, typed[key], indent+"  ")

			if index < len(keys)-1 {
				buffer.WriteString(",")
			}

			buffer.WriteString("\n")
		}

		buffer.WriteString(indent + "}")
	case []any:
		buffer.WriteString("[ ")

		for index, element := range typed {
			if index > 0 {
				buffer.WriteString(" , ")
			}

			writeReversedJSON(t, buffer, element, indent)
		}

		buffer.WriteString(" ]")
	case json.Number:
		buffer.WriteString(typed.String())
	default:
		encoded, err := json.Marshal(typed)
		require.NoError(t, err)
		buffer.Write(encoded)
	}
}

func requireCrossLedgerReplay(t *testing.T, status int, body []byte, response *http.Response, original []byte) {
	t.Helper()

	require.Equal(t, http.StatusCreated, status, "body: %s", string(body))
	require.Equal(t, "true", response.Header.Get("X-Idempotency-Replayed"))
	require.JSONEq(t, string(original), string(body))
}

func requireCrossLedgerIdempotencyConflict(t *testing.T, status int, body []byte) {
	t.Helper()

	require.Equal(t, http.StatusConflict, status, "body: %s", string(body))
	requireProblemCode(t, body, constant.ErrIdempotencyKey.Error())
}

func TestIntegration_CrossLedgerV2_IdempotencyFingerprint(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	db := fixture.infra.pgContainer.DB

	t.Run("a keyed direct retry that re-serializes the request replays the group", func(t *testing.T) {
		pair := fixture.crossLedgerIdempotencyPair(t, "keyed-direct-reserialized")
		key := "keyed-direct-reserialized"

		status, original, _ := fixture.postCrossLedger(t, "direct", pair.raw, key)
		decodeCrossLedgerGroup(t, status, original, http.StatusCreated)

		status, body, response := fixture.postCrossLedger(t, "direct", reserializedRequestBody(t, pair.raw), key)
		requireCrossLedgerReplay(t, status, body, response, original)
		require.Equal(t, 1, countTransactionsInLedger(t, db, pair.ledgerA))
		require.Equal(t, 1, countTransactionsInLedger(t, db, pair.ledgerB))

		fingerprint, err := v2IdempotencyFingerprint([]byte(pair.raw), false, "")
		require.NoError(t, err)
		record := fixture.groupIdempotencyRecord(t, fixture.groupIdempotencyRedisKey(pair.ledgerA, key))
		require.Equal(t, redistransaction.AtomicTransactionBatchStateComplete, record.State)
		require.Equal(t, fingerprint, record.RequestFingerprint, "a keyed group stores its canonical fingerprint")
	})

	t.Run("a keyed hold retry that re-serializes the request replays the held group", func(t *testing.T) {
		pair := fixture.crossLedgerIdempotencyPair(t, "keyed-hold-reserialized")
		key := "keyed-hold-reserialized"

		status, original, _ := fixture.postCrossLedger(t, "hold", pair.raw, key)
		held := decodeCrossLedgerGroup(t, status, original, http.StatusCreated)
		require.Len(t, held.Transactions, 1)

		status, body, response := fixture.postCrossLedger(t, "hold", reserializedRequestBody(t, pair.raw), key)
		requireCrossLedgerReplay(t, status, body, response, original)
		require.Equal(t, 1, countPendingTransactionsInLedger(t, fixture, pair.ledgerA))
		require.Equal(t, 1, countTransactionsInLedger(t, db, pair.ledgerA))
		require.Zero(t, countTransactionsInLedger(t, db, pair.ledgerB))

		fingerprint, err := v2IdempotencyFingerprint([]byte(pair.raw), true, "")
		require.NoError(t, err)
		record := fixture.groupIdempotencyRecord(t, fixture.groupIdempotencyRedisKey(pair.ledgerA, key))
		require.Equal(t, fingerprint, record.RequestFingerprint, "a keyed hold stores its canonical fingerprint")
	})

	t.Run("a keyed retry with a changed amount or on the other action conflicts", func(t *testing.T) {
		pair := fixture.crossLedgerIdempotencyPair(t, "keyed-changed")
		key := "keyed-changed"

		status, original, _ := fixture.postCrossLedger(t, "direct", pair.raw, key)
		decodeCrossLedgerGroup(t, status, original, http.StatusCreated)

		changed := strings.ReplaceAll(pair.raw, `"10"`, `"20"`)
		require.NotEqual(t, pair.raw, changed)
		status, body, _ := fixture.postCrossLedger(t, "direct", changed, key)
		requireCrossLedgerIdempotencyConflict(t, status, body)

		status, body, _ = fixture.postCrossLedger(t, "hold", pair.raw, key)
		requireCrossLedgerIdempotencyConflict(t, status, body)

		require.Equal(t, 1, countTransactionsInLedger(t, db, pair.ledgerA))
		require.Equal(t, 1, countTransactionsInLedger(t, db, pair.ledgerB))
	})

	t.Run("a keyed record holding the raw-bytes fingerprint replays only identical bytes", func(t *testing.T) {
		pair := fixture.crossLedgerIdempotencyPair(t, "keyed-raw-record")
		key := "keyed-raw-record"

		status, original, _ := fixture.postCrossLedger(t, "direct", pair.raw, key)
		decodeCrossLedgerGroup(t, status, original, http.StatusCreated)

		redisKey := fixture.groupIdempotencyRedisKey(pair.ledgerA, key)
		record := fixture.groupIdempotencyRecord(t, redisKey)
		record.RequestFingerprint = sha256Hex(v2IdempotencyHashSource([]byte(pair.raw), false, ""))
		rewritten, err := json.Marshal(record)
		require.NoError(t, err)
		require.NoError(t, fixture.infra.redisContainer.Client.Set(context.Background(), redisKey, rewritten, redis.KeepTTL).Err())

		status, body, response := fixture.postCrossLedger(t, "direct", pair.raw, key)
		requireCrossLedgerReplay(t, status, body, response, original)

		status, body, _ = fixture.postCrossLedger(t, "direct", reserializedRequestBody(t, pair.raw), key)
		requireCrossLedgerIdempotencyConflict(t, status, body)

		require.Equal(t, 1, countTransactionsInLedger(t, db, pair.ledgerA))
		require.Equal(t, 1, countTransactionsInLedger(t, db, pair.ledgerB))
	})

	t.Run("without a key only identical bytes replay", func(t *testing.T) {
		pair := fixture.crossLedgerIdempotencyPair(t, "unkeyed")

		status, original, _ := fixture.postCrossLedger(t, "direct", pair.raw, "")
		first := decodeCrossLedgerGroup(t, status, original, http.StatusCreated)

		rawFingerprint := sha256Hex(v2IdempotencyHashSource([]byte(pair.raw), false, ""))
		record := fixture.groupIdempotencyRecord(t, fixture.groupIdempotencyRedisKey(pair.ledgerA, rawFingerprint))
		require.Equal(t, rawFingerprint, record.RequestFingerprint, "an unkeyed group is named and identified by its raw bytes")

		status, body, response := fixture.postCrossLedger(t, "direct", pair.raw, "")
		requireCrossLedgerReplay(t, status, body, response, original)

		status, body, response = fixture.postCrossLedger(t, "direct", reserializedRequestBody(t, pair.raw), "")
		second := decodeCrossLedgerGroup(t, status, body, http.StatusCreated)
		require.Equal(t, "false", response.Header.Get("X-Idempotency-Replayed"))
		require.NotEqual(t, *first.GroupID, *second.GroupID)

		require.Equal(t, 2, countTransactionsInLedger(t, db, pair.ledgerA))
		require.Equal(t, 2, countTransactionsInLedger(t, db, pair.ledgerB))
	})
}
