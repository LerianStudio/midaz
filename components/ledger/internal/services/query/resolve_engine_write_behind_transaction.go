// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const engineMaterializedTransactionTTL = 5 * time.Minute

type EngineTransactionResolutionSource string

const (
	EngineTransactionResolutionMaterialized EngineTransactionResolutionSource = "materialized"
	EngineTransactionResolutionEvidence     EngineTransactionResolutionSource = "evidence"
	EngineTransactionResolutionPrimary      EngineTransactionResolutionSource = "primary"
)

type EngineTransactionResolution struct {
	Transaction *transaction.Transaction
	Source      EngineTransactionResolutionSource
	ExecutionID uuid.UUID
	Pending     bool
}

// EngineWriteBehindEvidenceCodec keeps query independent from command while
// reusing command's canonical versioned codecs and transaction composer.
type EngineWriteBehindEvidenceCodec interface {
	DecodeEngineTransactionIndex(context.Context, []byte, uuid.UUID, uuid.UUID, uuid.UUID) (uuid.UUID, bool, error)
	BuildEngineTransactionLookup(context.Context, []byte, []byte, []byte, uuid.UUID, uuid.UUID, uuid.UUID) (*transaction.Transaction, error)
}

func (uc *UseCase) CanResolveEngineWriteBehind() bool {
	if uc == nil || uc.EngineWriteBehindCodec == nil {
		return false
	}

	if uc.EngineWriteBehindRepo != nil {
		return true
	}

	_, ok := uc.TransactionRedisRepo.(redis.EngineWriteBehindRepository)

	return ok
}

// ResolveEngineWriteBehindTransaction returns the latest indexed accounting
// state without consulting an eventually consistent replica. Absence of a
// pending index falls back to the PostgreSQL primary; corruption or missing
// evidence behind an existing index fails closed instead of returning old SQL.
//
// An index read that fails on transport is not an absence: a reachable index
// may already describe an execution SQL has not received, so the read
// propagates instead of falling back. Only ErrEngineWriteBehindNotFound proves
// there is no newer accounting state to miss. A scope carrying a nil ID skips
// the index: the engine never indexes one, so only the primary can answer it.
//
//nolint:gocognit,gocyclo // the materialized/evidence/primary chain deliberately classifies each corruption and miss independently
func (uc *UseCase) ResolveEngineWriteBehindTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*EngineTransactionResolution, error) {
	repository := uc.EngineWriteBehindRepo
	if repository == nil && uc.TransactionRedisRepo != nil {
		repository, _ = uc.TransactionRedisRepo.(redis.EngineWriteBehindRepository)
	}

	if repository == nil || organizationID == uuid.Nil || ledgerID == uuid.Nil || transactionID == uuid.Nil {
		return uc.resolveEngineTransactionFromPrimary(ctx, organizationID, ledgerID, transactionID)
	}

	if uc.EngineWriteBehindCodec == nil {
		return nil, fmt.Errorf("engine write-behind evidence codec is not configured")
	}

	for attempt := 0; attempt < 3; attempt++ {
		rawIndex, err := repository.GetEngineTransactionIndex(ctx, organizationID, ledgerID, transactionID)
		if errors.Is(err, redis.ErrEngineWriteBehindNotFound) {
			return uc.resolveEngineTransactionFromPrimary(ctx, organizationID, ledgerID, transactionID)
		}

		if err != nil {
			return nil, fmt.Errorf("read engine transaction index: %w", err)
		}

		executionID, pending, err := uc.EngineWriteBehindCodec.DecodeEngineTransactionIndex(ctx, rawIndex, organizationID, ledgerID, transactionID)
		if err != nil {
			return nil, fmt.Errorf("decode engine transaction index: %w", err)
		}

		materialized, materializedErr := repository.GetEngineMaterializedTransaction(ctx, organizationID, ledgerID, transactionID)
		if materializedErr == nil && materialized.ExecutionID == executionID {
			tran, decodeErr := decodeMaterializedEngineTransaction(materialized.Payload, organizationID, ledgerID, transactionID)
			if decodeErr == nil {
				current, readErr := repository.GetEngineTransactionIndex(ctx, organizationID, ledgerID, transactionID)
				if readErr == nil && bytes.Equal(current, rawIndex) {
					return &EngineTransactionResolution{Transaction: tran, Source: EngineTransactionResolutionMaterialized, ExecutionID: executionID, Pending: pending}, nil
				}

				if readErr != nil && !errors.Is(readErr, redis.ErrEngineWriteBehindNotFound) {
					return nil, readErr
				}

				continue
			}
		}

		rawEnvelope, rawReceipt, err := repository.GetEngineTransactionEvidence(ctx, organizationID, ledgerID, transactionID, executionID)
		if err != nil {
			return nil, fmt.Errorf("resolve indexed engine transaction evidence: %w", err)
		}

		tran, err := uc.EngineWriteBehindCodec.BuildEngineTransactionLookup(ctx, rawIndex, rawEnvelope, rawReceipt, organizationID, ledgerID, transactionID)
		if err != nil {
			return nil, err
		}

		payload, err := msgpack.Marshal(tran)
		if err != nil {
			return nil, fmt.Errorf("encode materialized engine transaction: %w", err)
		}

		stored, materializeErr := repository.MaterializeEngineTransaction(ctx, organizationID, ledgerID, transactionID, executionID, payload, engineMaterializedTransactionTTL)
		if materializeErr == nil && !stored {
			continue
		}

		if materializeErr != nil {
			current, readErr := repository.GetEngineTransactionIndex(ctx, organizationID, ledgerID, transactionID)
			if readErr != nil || !bytes.Equal(current, rawIndex) {
				if readErr != nil && !errors.Is(readErr, redis.ErrEngineWriteBehindNotFound) {
					return nil, readErr
				}

				continue
			}
		}

		return &EngineTransactionResolution{Transaction: tran, Source: EngineTransactionResolutionEvidence, ExecutionID: executionID, Pending: pending}, nil
	}

	return nil, fmt.Errorf("engine transaction index changed during bounded lookup")
}

// ResolveTransactionProjection adapts the engine-aware lookup to command's
// lifecycle-write port without exposing query-specific source enums.
func (uc *UseCase) ResolveTransactionProjection(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, uuid.UUID, bool, error) {
	resolved, err := uc.ResolveEngineWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, uuid.Nil, false, err
	}

	if resolved == nil {
		return nil, uuid.Nil, false, nil
	}

	return resolved.Transaction, resolved.ExecutionID, resolved.Pending, nil
}

func (uc *UseCase) resolveEngineTransactionFromPrimary(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*EngineTransactionResolution, error) {
	if uc.TransactionRepo == nil {
		return nil, redis.ErrEngineWriteBehindNotFound
	}

	primaryCtx := readrouting.WithPrimaryRead(ctx)

	tran, err := uc.TransactionRepo.FindWithOperations(primaryCtx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	// FindWithOperations joins on operations, so a missing transaction and a row
	// whose operations are not persisted yet both come back as an empty value. The
	// row-only read tells them apart: not-found for the first, the real row for
	// the second.
	if tran == nil || tran.ID == "" {
		tran, err = uc.TransactionRepo.Find(primaryCtx, organizationID, ledgerID, transactionID)
		if err != nil {
			return nil, err
		}

		tran.Operations = []*operation.Operation{}
	}

	if uc.TransactionMetadataRepo != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, constant.EntityTransaction, transactionID.String())
		if err != nil {
			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return &EngineTransactionResolution{Transaction: tran, Source: EngineTransactionResolutionPrimary}, nil
}

func decodeMaterializedEngineTransaction(payload []byte, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	var tran transaction.Transaction
	if err := msgpack.Unmarshal(payload, &tran); err != nil {
		return nil, fmt.Errorf("decode materialized engine transaction: %w", err)
	}

	if tran.ID != transactionID.String() || tran.OrganizationID != organizationID.String() || tran.LedgerID != ledgerID.String() {
		return nil, fmt.Errorf("materialized engine transaction scope mismatch")
	}

	return &tran, nil
}
