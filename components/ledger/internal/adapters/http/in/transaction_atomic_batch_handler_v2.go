// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

// atomicTransactionBatchAbsoluteMaxItems is the immutable public-contract ceiling.
// An installation may enforce a lower configured limit at runtime, but the OpenAPI
// contract must never advertise or accept more than this many transactions.
const atomicTransactionBatchAbsoluteMaxItems = 50

// CreateAtomicTransactionBatchV2Input is the Huma request envelope for the atomic
// direct-v2 batch route. RawBody keeps runtime validation imperative so the handler
// can collect ordered structural errors across all items before external work starts.
type CreateAtomicTransactionBatchV2Input struct {
	IdempotencyKey string `header:"X-Idempotency" doc:"Idempotency key to safely retry the atomic batch; an identical retry returns the original ordered response"`
	IdempotencyTTL string `header:"X-TTL" doc:"Idempotency slot TTL in seconds (default 300)"`
	RawBody        []byte `contentType:"application/json"`
}

// CreateAtomicTransactionBatchV2Request is the stable request wrapper for an
// ordered atomic batch. Array order is execution order and must be preserved by
// validation, accounting, completion, replay, and response construction.
type CreateAtomicTransactionBatchV2Request struct {
	Transactions []CreateTransactionV2Request `json:"transactions" validate:"min=1,max=50,dive" minItems:"1" maxItems:"50" nullable:"false" doc:"Direct-v2 transactions in execution order. All items succeed atomically or none is applied."`
}

// CreateAtomicTransactionBatchV2Response is the successful public response. BatchID
// is an ephemeral correlation and replay identifier, not a persisted or queryable
// ledger resource. Transactions remain in the exact request-array order.
type CreateAtomicTransactionBatchV2Response struct {
	BatchID      string           `json:"batchId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	Transactions []*TransactionV2 `json:"transactions" nullable:"false" doc:"Created transactions in the exact request-array order."`
}

// CreateAtomicTransactionBatchV2Output is the Huma success envelope: HTTP 201,
// batch-level replay metadata, and the stable ordered response wrapper.
type CreateAtomicTransactionBatchV2Output struct {
	Status              int
	IdempotencyReplayed string `header:"X-Idempotency-Replayed"`
	Body                *CreateAtomicTransactionBatchV2Response
}

// CreateAtomicTransactionBatchV2 is registered with the public contract in task
// 6.1. Task 6.2 replaces this temporary terminal with the structural collector and
// command integration without changing the route or its published types.
func (handler *TransactionHandler) CreateAtomicTransactionBatchV2(
	context.Context,
	*CreateAtomicTransactionBatchV2Input,
) (*CreateAtomicTransactionBatchV2Output, error) {
	return nil, huma.Error501NotImplemented("atomic transaction batch handler is not wired")
}
