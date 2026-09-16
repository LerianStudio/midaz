// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

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

// CreateAtomicTransactionBatchV2 strictly decodes and structurally validates the
// whole wrapper before invoking the batch command once. The collector preserves the
// request array order and aggregates body-only errors; repository, fee, Tracer,
// idempotency, and accounting work starts only after that phase succeeds.
func (handler *TransactionHandler) CreateAtomicTransactionBatchV2(
	ctx context.Context,
	in *CreateAtomicTransactionBatchV2Input,
) (*CreateAtomicTransactionBatchV2Output, error) {
	if err := ctx.Err(); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}
	if handler.TransactionBatchMaxSize < 1 || handler.TransactionBatchMaxSize > atomicTransactionBatchV2AbsoluteMaxSize {
		return nil, pkgHTTP.HumaProblem(fmt.Errorf(
			"atomic transaction batch maximum size must be between 1 and %d",
			atomicTransactionBatchV2AbsoluteMaxSize,
		))
	}

	decoded, err := decodeAndValidateAtomicTransactionBatchV2(in.RawBody, handler.TransactionBatchMaxSize)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	organizationID, ledgerID, err := parseOrgLedger(decoded.scope.OrganizationID, decoded.scope.LedgerID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}
	if handler.Command == nil {
		return nil, pkgHTTP.HumaProblem(errors.New("atomic transaction batch command is not configured"))
	}

	items := make([]command.CreateAtomicTransactionBatchV2ItemInput, len(decoded.items))
	for index := range decoded.items {
		items[index] = command.CreateAtomicTransactionBatchV2ItemInput{
			OrganizationID:          organizationID,
			LedgerID:                ledgerID,
			Transaction:             decoded.items[index].normalized.transaction,
			AccountBlockExceptionID: decoded.items[index].accountBlockExceptionID,
		}
	}

	result, err := handler.Command.CreateAtomicTransactionBatchV2(ctx, command.CreateAtomicTransactionBatchV2Input{
		Transactions:     items,
		CanonicalRequest: decoded.canonicalRequest,
		IdempotencyKey:   in.IdempotencyKey,
		IdempotencyTTL:   pkgHTTP.ParseIdempotencyTTL(in.IdempotencyTTL),
	})
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}
	if result == nil {
		return nil, pkgHTTP.HumaProblem(errors.New("atomic transaction batch command returned no result"))
	}

	transactions := make([]*TransactionV2, len(result.Transactions))
	for index := range result.Transactions {
		transactions[index] = newTransactionV2(result.Transactions[index])
	}

	return &CreateAtomicTransactionBatchV2Output{
		Status:              http.StatusCreated,
		IdempotencyReplayed: replayedHeader(result.Replayed),
		Body: &CreateAtomicTransactionBatchV2Response{
			BatchID:      result.BatchID.String(),
			Transactions: transactions,
		},
	}, nil
}
