// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ResolveTransactionGroupMembers reads the member manifest from the addressed
// transaction's indexed evidence and resolves each member through
// ResolveEngineWriteBehindTransaction, so a member already persisted and one
// still pending projection answer alike.
func (uc *UseCase) ResolveTransactionGroupMembers(
	ctx context.Context,
	organizationID, ledgerID, transactionID, groupID uuid.UUID,
) ([]*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.resolve_transaction_group_members")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.transaction_id", transactionID.String()),
		attribute.String("app.request.group_id", groupID.String()),
	)

	manifest, found, err := uc.engineExecutionMembers(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read the engine execution members", err)

		return nil, err
	}

	span.SetAttributes(attribute.Bool("app.transaction_group.members_from_engine", found))

	if !found {
		return uc.FindTransactionsByGroupID(readrouting.WithPrimaryRead(ctx), groupID)
	}

	members := make([]*transaction.Transaction, 0, len(manifest))

	for _, member := range manifest {
		resolved, err := uc.ResolveEngineWriteBehindTransaction(
			readrouting.WithPrimaryRead(ctx),
			member.OrganizationID,
			member.LedgerID,
			member.TransactionID,
		)

		var notFound pkg.EntityNotFoundError
		if errors.As(err, &notFound) || (err == nil && (resolved == nil || resolved.Transaction == nil)) {
			incomplete := pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Cross-ledger group member not found", incomplete)

			return nil, incomplete
		}

		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to resolve a cross-ledger group member", err)

			return nil, err
		}

		members = append(members, resolved.Transaction)
	}

	return members, nil
}

// engineExecutionMembers reads the member manifest from the addressed
// transaction's indexed evidence. found is false when there is no index or the
// execution recorded no manifest; evidence missing behind an index fails closed.
func (uc *UseCase) engineExecutionMembers(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
) ([]EngineExecutionMember, bool, error) {
	repository := uc.EngineWriteBehindRepo
	if repository == nil && uc.TransactionRedisRepo != nil {
		repository, _ = uc.TransactionRedisRepo.(redis.EngineWriteBehindRepository)
	}

	if repository == nil || uc.EngineWriteBehindCodec == nil ||
		organizationID == uuid.Nil || ledgerID == uuid.Nil || transactionID == uuid.Nil {
		return nil, false, nil
	}

	rawIndex, err := repository.GetEngineTransactionIndex(ctx, organizationID, ledgerID, transactionID)
	if errors.Is(err, redis.ErrEngineWriteBehindNotFound) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("read engine transaction index: %w", err)
	}

	executionID, _, err := uc.EngineWriteBehindCodec.DecodeEngineTransactionIndex(ctx, rawIndex, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, false, fmt.Errorf("decode engine transaction index: %w", err)
	}

	rawEnvelope, rawReceipt, err := repository.GetEngineTransactionEvidence(ctx, organizationID, ledgerID, transactionID, executionID)
	if err != nil {
		return nil, false, fmt.Errorf("resolve indexed engine transaction evidence: %w", err)
	}

	return uc.EngineWriteBehindCodec.DecodeEngineTransactionExecutionMembers(
		ctx, rawIndex, rawEnvelope, rawReceipt, organizationID, ledgerID, transactionID,
	)
}
