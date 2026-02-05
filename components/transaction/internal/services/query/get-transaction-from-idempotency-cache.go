// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetTransactionFromIdempotencyCache attempts to retrieve a transaction from the idempotency cache.
// It performs a two-step lookup:
// 1. Gets the idempotency key from the reverse mapping (transactionID -> idempotency key)
// 2. Gets the cached transaction JSON using the idempotency key
//
// Returns the cached transaction and true if found, or nil and false on cache miss or any error.
// This method uses graceful degradation - any error results in a cache miss rather than propagating the error.
func (uc *UseCase) GetTransactionFromIdempotencyCache(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, bool) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_from_idempotency_cache")
	defer span.End()

	// Look up reverse mapping (transactionID -> idempotency key)
	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())

	idempotencyKey, err := uc.RedisRepo.Get(ctx, reverseKey)
	if err != nil {
		logger.Infof("Cache reverse mapping lookup failed for transactionID %s: %v", transactionID.String(), err)

		span.SetAttributes(attribute.Bool("app.request.cache.hit", false))
		span.SetAttributes(attribute.String("app.request.cache.miss_reason", "reverse_key_error"))

		return nil, false
	}

	if idempotencyKey == "" {
		logger.Infof("Cache reverse mapping not found for transactionID %s", transactionID.String())

		span.SetAttributes(attribute.Bool("app.request.cache.hit", false))
		span.SetAttributes(attribute.String("app.request.cache.miss_reason", "reverse_key_not_found"))

		return nil, false
	}

	// Then look up idempotency response (idempotency key -> transaction JSON)
	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

	cachedJSON, err := uc.RedisRepo.Get(ctx, internalKey)
	if err != nil {
		logger.Infof("Cache lookup failed for idempotency key, transactionID %s: %v", transactionID.String(), err)

		span.SetAttributes(attribute.Bool("app.request.cache.hit", false))
		span.SetAttributes(attribute.String("app.request.cache.miss_reason", "idempotency_key_error"))

		return nil, false
	}

	if cachedJSON == "" {
		logger.Infof("Cache idempotency response not found or empty for transactionID %s", transactionID.String())

		span.SetAttributes(attribute.Bool("app.request.cache.hit", false))
		span.SetAttributes(attribute.String("app.request.cache.miss_reason", "idempotency_response_empty"))

		return nil, false
	}

	// Unmarshal cached transaction
	var cachedTran transaction.Transaction
	if err := json.Unmarshal([]byte(cachedJSON), &cachedTran); err != nil {
		logger.Infof("Failed to unmarshal cached transaction for transactionID %s: %v", transactionID.String(), err)

		span.SetAttributes(attribute.Bool("app.request.cache.hit", false))
		span.SetAttributes(attribute.String("app.request.cache.miss_reason", "unmarshal_error"))

		return nil, false
	}

	logger.Infof("Cache hit for transactionID %s", transactionID.String())

	span.SetAttributes(attribute.Bool("app.request.cache.hit", true))

	return &cachedTran, true
}
