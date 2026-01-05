package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/utils"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// hashPrefixLength is the length of hash prefix used for logging.
const hashPrefixLength = 8

// CreateOrCheckIdempotencyKey attempts to create an idempotency key in Redis using SetNX.
// If the key already exists, it returns the stored value. Returns nil if the key was created.
func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	assert.That(organizationID != uuid.Nil,
		"organization_id must not be nil UUID for idempotency key",
		"key", key,
		"hash_prefix", hash[:min(len(hash), hashPrefixLength)])
	assert.That(ledgerID != uuid.Nil,
		"ledger_id must not be nil UUID for idempotency key",
		"organization_id", organizationID,
		"key", key)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

	success, err := uc.RedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error to lock idempotency key on redis failed", err)

		logger.Error("Error to lock idempotency key on redis failed:", err.Error())

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
	}

	if !success {
		value, err := uc.RedisRepo.Get(ctx, internalKey)
		if err != nil && !errors.Is(err, redis.Nil) {
			libOpentelemetry.HandleSpanError(&span, "Error to get idempotency key on redis failed", err)

			logger.Error("Error to get idempotency key on redis failed:", err.Error())

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		if !libCommons.IsNilOrEmpty(&value) {
			logger.Infof("Found value on redis with this key: %v", internalKey)

			return &value, nil
		}

		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

		logger.Warnf("Failed, exists value on redis with this key: %v", err)

		return nil, err
	}

	return nil, nil
}

// SetValueOnExistingIdempotencyKey func that set value on idempotency key to return to user.
func (uc *UseCase) SetValueOnExistingIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, t transaction.Transaction, ttl time.Duration) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_value_idempotency_key")
	defer span.End()

	logger.Infof("Trying to set value on idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

	value, err := libCommons.StructToJSONString(t)
	if err != nil {
		logger.Error("Err to serialize transaction struct %v\n", err)
	}

	err = uc.RedisRepo.Set(ctx, internalKey, value, ttl)
	if err != nil {
		logger.Error("Error to set value on lock idempotency key on redis:", err.Error())
	}
}

// SetTransactionIdempotencyMapping stores the reverse mapping from transactionID to idempotency key.
// This allows looking up which idempotency key corresponds to a given transaction.
func (uc *UseCase) SetTransactionIdempotencyMapping(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID, idempotencyKey string, ttl time.Duration) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_transaction_idempotency_mapping")
	defer span.End()

	logger.Infof("Trying to set transaction idempotency mapping in redis for transactionID: %s", transactionID)

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID)

	err := uc.RedisRepo.Set(ctx, reverseKey, idempotencyKey, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error setting transaction idempotency mapping in redis", err)

		logger.Errorf("Error setting transaction idempotency mapping in redis for transactionID %s: %s", transactionID, err.Error())
	}
}
