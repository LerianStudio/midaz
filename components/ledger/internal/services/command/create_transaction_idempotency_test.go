// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// slotFingerprintField is the wire name of the fingerprint inside a stored slot.
// Pods of every version share the slot, so the name is a contract, not a detail.
const slotFingerprintField = "idempotencyFingerprint"

func TestCreateOrCheckTransactionIdempotency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	hash := "test-hash-value"
	fingerprint := "request-fingerprint"
	ttl := 24 * time.Hour

	t.Run("success with key", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, fingerprint, ttl)

		assert.NoError(t, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("success with empty key", func(t *testing.T) {
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, hash)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, "", hash, fingerprint, ttl)

		assert.NoError(t, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("stored value without a fingerprint replays whatever request reuses the key", func(t *testing.T) {
		key := "legacy-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		legacyTxn := transaction.Transaction{ID: uuid.New().String(), Description: "written before fingerprints"}
		legacyJSON, err := json.Marshal(legacyTxn)
		require.NoError(t, err)

		mockRedisRepo.EXPECT().SetNX(gomock.Any(), internalKey, "", ttl).Return(false, nil).Times(1)
		mockRedisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(string(legacyJSON), nil).Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, "any-other-request", ttl)

		require.NoError(t, err)
		require.NotNil(t, result.Replay)
		assert.Equal(t, legacyTxn.ID, result.Replay.ID)
		assert.Equal(t, legacyTxn.Description, result.Replay.Description)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("stored value with the same fingerprint replays", func(t *testing.T) {
		key := "same-request-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		storedTxn := transaction.Transaction{ID: uuid.New().String(), Description: "first"}

		mockRedisRepo.EXPECT().SetNX(gomock.Any(), internalKey, "", ttl).Return(false, nil).Times(1)
		mockRedisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(slotValueWithFingerprint(t, storedTxn, fingerprint), nil).Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, fingerprint, ttl)

		require.NoError(t, err)
		require.NotNil(t, result.Replay)
		assert.Equal(t, storedTxn.ID, result.Replay.ID)
		assert.Equal(t, storedTxn.Description, result.Replay.Description)
	})

	t.Run("stored value with a different fingerprint is a conflict and leaves the slot alone", func(t *testing.T) {
		key := "reused-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		storedTxn := transaction.Transaction{ID: uuid.New().String(), Description: "first"}

		// No Del or Set expectation: the mock fails the test if the conflict
		// path releases or overwrites the slot the first request owns.
		mockRedisRepo.EXPECT().SetNX(gomock.Any(), internalKey, "", ttl).Return(false, nil).Times(1)
		mockRedisRepo.EXPECT().Get(gomock.Any(), internalKey).Return(slotValueWithFingerprint(t, storedTxn, "first-request"), nil).Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, "second-request", ttl)

		var conflict pkg.EntityConflictError
		require.ErrorAs(t, err, &conflict)
		assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
		assert.Nil(t, result.Replay)
	})

	t.Run("key already exists with invalid JSON returns error", func(t *testing.T) {
		key := "bad-json-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("not-valid-json", nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, fingerprint, ttl)

		assert.Error(t, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("key exists with empty value returns idempotency error", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, fingerprint, ttl)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("key disappeared between SetNX and Get returns idempotency error", func(t *testing.T) {
		key := "disappearing-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", goredis.Nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, fingerprint, ttl)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("SetNX returns error", func(t *testing.T) {
		key := "error-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		expectedErr := assert.AnError

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, expectedErr).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, fingerprint, ttl)

		assert.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("Get returns non-nil error other than redis.Nil", func(t *testing.T) {
		key := "get-error-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		expectedErr := assert.AnError

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", expectedErr).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, fingerprint, ttl)

		assert.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})
}

func TestSetTransactionIdempotencyValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	hash := "test-hash-value"
	fingerprint := "request-fingerprint"
	ttl := 24 * time.Hour

	newTxn := func(description string) transaction.Transaction {
		return transaction.Transaction{
			ID:                       uuid.New().String(),
			OrganizationID:           organizationID.String(),
			LedgerID:                 ledgerID.String(),
			Description:              description,
			ChartOfAccountsGroupName: "test-group",
			Status:                   transaction.Status{Code: "COMMITTED"},
		}
	}

	t.Run("stores the transaction and the request fingerprint under the key", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		txn := newTxn("Test transaction")

		var stored string

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, gomock.Any(), ttl).
			DoAndReturn(func(_ context.Context, _, value string, _ time.Duration) error {
				stored = value
				return nil
			}).
			Times(1)

		uc.SetTransactionIdempotencyValue(ctx, organizationID, ledgerID, key, hash, fingerprint, txn, ttl)

		assert.JSONEq(t, slotValueWithFingerprint(t, txn, fingerprint), stored)
	})

	t.Run("stores under the hash when no key was sent", func(t *testing.T) {
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, hash)
		txn := newTxn("Test transaction with empty key")

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, gomock.Any(), ttl).
			Return(nil).
			Times(1)

		uc.SetTransactionIdempotencyValue(ctx, organizationID, ledgerID, "", hash, fingerprint, txn, ttl)
	})

	t.Run("a pod that decodes only the transaction still replays the stored value", func(t *testing.T) {
		key := "mixed-version-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		txn := newTxn("Read by an older decoder")

		var stored string

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, gomock.Any(), ttl).
			DoAndReturn(func(_ context.Context, _, value string, _ time.Duration) error {
				stored = value
				return nil
			}).
			Times(1)

		uc.SetTransactionIdempotencyValue(ctx, organizationID, ledgerID, key, hash, fingerprint, txn, ttl)

		// Older pods decode the slot straight into transaction.Transaction, so the
		// fingerprint must be an extra top-level field they ignore, not an envelope.
		var decoded transaction.Transaction
		require.NoError(t, json.Unmarshal([]byte(stored), &decoded))
		assert.Equal(t, txn, decoded)
	})

	t.Run("redis set error", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		txn := newTxn("Test transaction with redis error")

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, gomock.Any(), ttl).
			Return(assert.AnError).
			Times(1)

		// Should not panic or return error
		uc.SetTransactionIdempotencyValue(ctx, organizationID, ledgerID, key, hash, fingerprint, txn, ttl)
	})
}

// TestTransactionJSONLeavesRoomForTheSlotFingerprint locks the two properties the
// slot format relies on: no transaction field is encoded or decoded under the
// fingerprint's name, and the transaction has no custom marshaler that would swallow
// a field added next to it. Names are read from the struct tags, including omitempty
// fields a zero value would not encode, and compared the way encoding/json matches
// them on decode: case-insensitively.
func TestTransactionJSONLeavesRoomForTheSlotFingerprint(t *testing.T) {
	transactionType := reflect.TypeOf(transaction.Transaction{})

	for i := 0; i < transactionType.NumField(); i++ {
		field := transactionType.Field(i)
		require.False(t, field.Anonymous, "embedded field %s would widen the JSON object beyond its declared tags", field.Name)

		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}

		if name == "" {
			name = field.Name
		}

		assert.False(t, strings.EqualFold(name, slotFingerprintField),
			"transaction field %s is encoded as %q, which collides with the slot fingerprint", field.Name, name)
	}

	_, hasMarshaler := any(transaction.Transaction{}).(json.Marshaler)
	_, hasPointerMarshaler := any(&transaction.Transaction{}).(json.Marshaler)
	assert.False(t, hasMarshaler || hasPointerMarshaler, "transaction.Transaction must not define MarshalJSON")
}

// slotValueWithFingerprint builds the stored slot the way the contract describes
// it, independently of the production writer: the transaction's JSON object with
// one extra top-level field.
func slotValueWithFingerprint(t *testing.T, txn transaction.Transaction, fingerprint string) string {
	t.Helper()

	raw, err := json.Marshal(txn)
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))

	fields[slotFingerprintField] = fingerprint

	value, err := json.Marshal(fields)
	require.NoError(t, err)

	return string(value)
}

func TestDeriveTransactionRequestFingerprint(t *testing.T) {
	input := mtransaction.Transaction{
		Description: "same body",
		Send: mtransaction.Send{
			Asset: "USD",
			Value: decimal.NewFromInt(50),
		},
	}

	withOverride := func(override string) mtransaction.Transaction {
		overridden := input
		overridden.OperationTypeOverride = override

		return overridden
	}

	derive := func(t *testing.T, in mtransaction.Transaction, status string) string {
		t.Helper()

		fingerprint, err := deriveTransactionRequestFingerprint(in, status)
		require.NoError(t, err)
		require.NotEmpty(t, fingerprint)

		return fingerprint
	}

	t.Run("the same input and status derive the same fingerprint", func(t *testing.T) {
		assert.Equal(t, derive(t, input, constant.CREATED), derive(t, input, constant.CREATED))
	})

	t.Run("modes that accept the same body derive distinct fingerprints", func(t *testing.T) {
		// /v1 json, annotation, block and unblock serialize the same input: the
		// status and the operation-type override are all that tell them apart.
		fingerprints := map[string]string{
			"json":       derive(t, input, constant.CREATED),
			"annotation": derive(t, input, constant.NOTED),
			"block":      derive(t, withOverride(constant.BLOCK), constant.CREATED),
			"unblock":    derive(t, withOverride(constant.UNBLOCK), constant.CREATED),
		}

		seen := make(map[string]string, len(fingerprints))
		for mode, fingerprint := range fingerprints {
			if other, ok := seen[fingerprint]; ok {
				t.Fatalf("%s and %s derive the same fingerprint", mode, other)
			}

			seen[fingerprint] = mode
		}
	})

	t.Run("the fingerprint of an unchanged request is stable across releases", func(t *testing.T) {
		// Stored slots outlive a deploy (up to the slot TTL): a release that changed
		// this value would answer an honest retry across the rollout with 0084.
		golden := mtransaction.Transaction{Description: "golden", Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(100)}}

		assert.Equal(t, "fe9e69744f41282f9cc73f910cb6f4fa785fc6099b5bc6b425f7f8486bdc1be3", derive(t, golden, constant.CREATED))
	})

	t.Run("a different body derives a different fingerprint", func(t *testing.T) {
		changed := input
		changed.Description = "other body"

		assert.NotEqual(t, derive(t, input, constant.CREATED), derive(t, changed, constant.CREATED))
	})
}

func TestClaimTransactionIdempotency_RequestFingerprint(t *testing.T) {
	ctx := context.Background()
	span := trace.SpanFromContext(ctx)
	logger := libLog.NewNop()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	ttl := time.Minute

	input := mtransaction.Transaction{Description: "same body", Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(50)}}

	newRun := func(status string) *createTransactionRun {
		return &createTransactionRun{
			organizationID: organizationID,
			ledgerID:       ledgerID,
			input:          input,
			status:         status,
			idempotencyKey: "shared-key",
			idempotencyTTL: ttl,
		}
	}

	t.Run("an annotation reusing a json create's slot is a conflict", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

		jsonFingerprint, err := deriveTransactionRequestFingerprint(input, constant.CREATED)
		require.NoError(t, err)

		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "shared-key")
		mockRedisRepo.EXPECT().SetNX(gomock.Any(), internalKey, "", ttl).Return(false, nil)
		mockRedisRepo.EXPECT().Get(gomock.Any(), internalKey).
			Return(slotValueWithFingerprint(t, transaction.Transaction{ID: uuid.New().String()}, jsonFingerprint), nil)

		run := newRun(constant.NOTED)
		replay, err := uc.claimTransactionIdempotency(ctx, span, logger, run, "", "")

		var conflict pkg.EntityConflictError
		require.ErrorAs(t, err, &conflict)
		assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
		assert.Nil(t, replay)
		assert.Nil(t, run.idempotencyInternalKey, "a conflicting request must not own the slot's cleanup")
	})

	t.Run("a supplied fingerprint is compared as given", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

		storedID := uuid.New().String()
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "shared-key")
		mockRedisRepo.EXPECT().SetNX(gomock.Any(), internalKey, "", ttl).Return(false, nil)
		mockRedisRepo.EXPECT().Get(gomock.Any(), internalKey).
			Return(slotValueWithFingerprint(t, transaction.Transaction{ID: storedID}, "canonical-v2-request"), nil)

		run := newRun(constant.CREATED)
		replay, err := uc.claimTransactionIdempotency(ctx, span, logger, run, "", "canonical-v2-request")

		require.NoError(t, err)
		require.NotNil(t, replay)
		assert.Equal(t, storedID, replay.ID)
		assert.Equal(t, "canonical-v2-request", run.idempotencyFingerprint)
	})
}
