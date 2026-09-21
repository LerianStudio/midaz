// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
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
) (*CreateAtomicTransactionBatchV2Result, error) {
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

	batch, err := buildCrossLedgerAtomicBatchInput(in, groupID)
	if err != nil {
		return nil, err
	}

	return uc.CreateAtomicTransactionBatchV2(ctx, batch)
}

func buildCrossLedgerAtomicBatchInput(
	in CreateCrossLedgerTransactionV2Input,
	groupID uuid.UUID,
) (CreateAtomicTransactionBatchV2Input, error) {
	scopes := crossLedgerTransactionScopes{
		from: make([]atomicTransactionBatchLedgerRef, len(in.Scopes.Debits)),
		to:   make([]atomicTransactionBatchLedgerRef, len(in.Scopes.Credits)),
	}
	for index, scope := range in.Scopes.Debits {
		scopes.from[index] = atomicTransactionBatchLedgerRef{organizationID: scope.OrganizationID, ledgerID: scope.LedgerID}
	}

	for index, scope := range in.Scopes.Credits {
		scopes.to[index] = atomicTransactionBatchLedgerRef{organizationID: scope.OrganizationID, ledgerID: scope.LedgerID}
	}

	parts, err := decomposeCrossLedgerTransaction(in.Transaction, scopes)
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
