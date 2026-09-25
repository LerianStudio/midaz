// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CrossLedgerLegScope identifies the ledger that owns one normalized v2 leg.
type CrossLedgerLegScope struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
}

// CrossLedgerTransactionScopes preserves debit and credit scope order so it
// remains aligned with the normalized transaction legs.
type CrossLedgerTransactionScopes struct {
	Debits  []CrossLedgerLegScope
	Credits []CrossLedgerLegScope
}

// CreateCrossLedgerTransactionV2Input carries one direct-v2 request that spans
// more than one ledger.
type CreateCrossLedgerTransactionV2Input struct {
	Transaction             mtransaction.Transaction
	Scopes                  CrossLedgerTransactionScopes
	AccountBlockExceptionID *uuid.UUID
	CanonicalRequest        []byte
	RequestFingerprint      string
	IdempotencyKey          string
	IdempotencyTTL          time.Duration
}

// CreateCrossLedgerTransactionV2 decomposes a direct request and delegates the
// entire group to the existing one-call atomic batch coordinator.
func (uc *UseCase) CreateCrossLedgerTransactionV2(
	ctx context.Context,
	in CreateCrossLedgerTransactionV2Input,
) (result *CreateAtomicTransactionBatchV2Result, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_cross_ledger_transaction_v2")
	defer span.End()

	start := time.Now()

	defer func() {
		recordCrossLedgerGroupError(ctx, span, logger, "Failed to create cross-ledger transaction", err)
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", crossLedgerOperationCreateTransaction, start, err)
	}()

	span.SetAttributes(attribute.String("app.request.action", constant.ActionDirect))

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if uc.UUIDv7Generator == nil {
		return nil, errors.New("cross-ledger transaction UUIDv7 generator is not configured")
	}

	groupID, err := uc.UUIDv7Generator()
	if err != nil {
		return nil, fmt.Errorf("generate cross-ledger transaction group id: %w", err)
	}

	if groupID == uuid.Nil {
		return nil, errors.New("cross-ledger transaction UUIDv7 generator returned a nil group id")
	}

	span.SetAttributes(attribute.String("app.response.group_id", groupID.String()))

	batch, err := buildCrossLedgerAtomicBatchInput(in, groupID)
	if err != nil {
		return nil, err
	}

	ledgers := setCrossLedgerGroupShape(span, crossLedgerBatchLedgerRefs(batch.Transactions))

	result, err = uc.executeAtomicTransactionBatchV2(ctx, batch)
	if err != nil {
		return nil, err
	}

	if result != nil && !result.Replayed {
		uc.recordCrossLedgerGroupLedgers(ctx, constant.ActionDirect, ledgers)
		uc.publishTransactionGroupEvent(ctx, transactionGroupEventPosted, groupID, nil, result.Transactions, crossLedgerGroupRole)
	}

	return result, nil
}

func buildCrossLedgerAtomicBatchInput(
	in CreateCrossLedgerTransactionV2Input,
	groupID uuid.UUID,
) (CreateAtomicTransactionBatchV2Input, error) {
	parts, err := decomposeCrossLedgerTransaction(in.Transaction, internalCrossLedgerScopes(in.Scopes))
	if err != nil {
		return CreateAtomicTransactionBatchV2Input{}, err
	}

	items := make([]CreateAtomicTransactionBatchV2ItemInput, len(parts))
	for index, part := range parts {
		items[index] = CreateAtomicTransactionBatchV2ItemInput{
			OrganizationID: part.ledgerRef.organizationID,
			LedgerID:       part.ledgerRef.ledgerID,
			Transaction:    part.transaction,
			Action:         constant.ActionDirect,
			Order:          index + 1,
			OriginalIndex:  index,
		}
	}

	if len(items) > 0 {
		items[0].AccountBlockExceptionID = in.AccountBlockExceptionID
	}

	return CreateAtomicTransactionBatchV2Input{
		Transactions:       items,
		GroupID:            &groupID,
		CrossLedgerGroup:   true,
		CanonicalRequest:   append([]byte(nil), in.CanonicalRequest...),
		RequestFingerprint: in.RequestFingerprint,
		IdempotencyKey:     in.IdempotencyKey,
		IdempotencyTTL:     in.IdempotencyTTL,
	}, nil
}
