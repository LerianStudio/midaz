// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type preparedCrossLedgerRevertPart struct {
	origin     *transaction.Transaction
	reversal   mtransaction.Transaction
	dependency TransactionEvidenceReference
}

// RevertCrossLedgerGroupV2 validates every member before delegating all
// reversals to one multi-scope atomic batch execution.
func (uc *UseCase) RevertCrossLedgerGroupV2(
	ctx context.Context,
	in RevertTransactionInput,
) (*CreateAtomicTransactionBatchV2Result, uuid.UUID, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.revert_cross_ledger_group_v2")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, uuid.Nil, err
	}

	_, target, err := uc.prepareRevertTransaction(ctx, span, in)
	if err != nil {
		if target != nil && target.GroupID != nil {
			err = uc.withCrossLedgerRevertMemberError(ctx, in, target, err)
		}

		return nil, uuid.Nil, err
	}

	if target == nil || target.GroupID == nil {
		return nil, uuid.Nil, pkg.ValidateBusinessError(
			constant.ErrCrossLedgerGroupIncomplete,
			constant.EntityTransaction,
		)
	}

	revertedGroupID, err := uuid.Parse(*target.GroupID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("parse cross-ledger transaction group id: %w", err)
	}

	result, err := uc.revertCrossLedgerGroupV2(ctx, in, revertedGroupID)
	if err != nil {
		return nil, uuid.Nil, err
	}

	return result, revertedGroupID, nil
}

func (uc *UseCase) revertCrossLedgerGroupV2(
	ctx context.Context,
	in RevertTransactionInput,
	revertedGroupID uuid.UUID,
) (result *CreateAtomicTransactionBatchV2Result, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.revert_cross_ledger_group")
	defer span.End()

	start := time.Now()

	defer func() {
		recordCrossLedgerGroupError(ctx, span, logger, "Failed to revert cross-ledger transaction group", err)
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", crossLedgerOperationRevertGroup, start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.action", constant.ActionRevert),
		attribute.String("app.request.group_id", revertedGroupID.String()),
	)

	resolver, ok := uc.TransactionReader.(TransactionGroupMemberResolver)
	if !ok {
		return nil, errors.New("cross-ledger transaction group member resolver is not configured")
	}

	members, err := resolver.ResolveTransactionGroupMembers(
		readrouting.WithPrimaryRead(ctx),
		in.OrganizationID,
		in.LedgerID,
		in.TransactionID,
		revertedGroupID,
	)
	if err != nil {
		return nil, err
	}

	if err := validateCrossLedgerRevertMembers(in.TransactionID, revertedGroupID, members); err != nil {
		return nil, err
	}

	ledgers := setCrossLedgerGroupShape(span, crossLedgerMemberLedgerRefs(members))

	parts := make([]preparedCrossLedgerRevertPart, len(members))
	for index, member := range members {
		part, partErr := uc.prepareCrossLedgerRevertPart(ctx, span, member)
		if partErr != nil {
			message := fmt.Sprintf("transaction %s in ledger %s is not revertible", member.ID, member.LedgerID)

			return nil, withAtomicTransactionBatchItemError(partErr, index, message)
		}

		parts[index] = part
	}

	if uc.UUIDv7Generator == nil {
		return nil, errors.New("cross-ledger revert UUIDv7 generator is not configured")
	}

	newGroupID, err := uc.UUIDv7Generator()
	if err != nil {
		return nil, fmt.Errorf("generate cross-ledger revert group id: %w", err)
	}

	if newGroupID == uuid.Nil {
		return nil, errors.New("cross-ledger revert UUIDv7 generator returned a nil group id")
	}

	span.SetAttributes(attribute.String("app.response.group_id", newGroupID.String()))

	batch, err := buildCrossLedgerRevertBatchInput(in, revertedGroupID, newGroupID, parts)
	if err != nil {
		return nil, err
	}

	result, err = uc.executeAtomicTransactionBatchV2(ctx, batch)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, errors.New("cross-ledger revert batch returned no result")
	}

	recordRevertReplay(ctx, span, logger, in.TransactionID, result.Replayed)

	if !result.Replayed {
		uc.recordCrossLedgerGroupLedgers(ctx, constant.ActionRevert, ledgers)
		uc.publishTransactionGroupEvent(ctx, transactionGroupEventReverted, newGroupID, &revertedGroupID, result.Transactions, crossLedgerGroupRole)
	}

	return result, nil
}

func (uc *UseCase) prepareCrossLedgerRevertPart(
	ctx context.Context,
	span trace.Span,
	member *transaction.Transaction,
) (preparedCrossLedgerRevertPart, error) {
	if member == nil {
		return preparedCrossLedgerRevertPart{}, pkg.ValidateBusinessError(
			constant.ErrCrossLedgerGroupIncomplete,
			constant.EntityTransaction,
		)
	}

	organizationID, err := uuid.Parse(member.OrganizationID)
	if err != nil {
		return preparedCrossLedgerRevertPart{}, fmt.Errorf("parse cross-ledger member organization id: %w", err)
	}

	ledgerID, err := uuid.Parse(member.LedgerID)
	if err != nil {
		return preparedCrossLedgerRevertPart{}, fmt.Errorf("parse cross-ledger member ledger id: %w", err)
	}

	transactionID, err := uuid.Parse(member.ID)
	if err != nil {
		return preparedCrossLedgerRevertPart{}, fmt.Errorf("parse cross-ledger member transaction id: %w", err)
	}

	reversal, _, err := uc.prepareRevertTransaction(ctx, span, RevertTransactionInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  transactionID,
	})
	if err != nil {
		return preparedCrossLedgerRevertPart{}, err
	}

	part := preparedCrossLedgerRevertPart{origin: member, reversal: reversal}

	resolution, err := resolveTransactionProjection(
		readrouting.WithPrimaryRead(ctx),
		uc.TransactionReader,
		organizationID,
		ledgerID,
		transactionID,
	)
	if err != nil {
		return preparedCrossLedgerRevertPart{}, err
	}
	// Durable origins can be reverted from their primary projection without
	// retaining engine evidence. Only an origin that is still pending projection
	// needs a causal dependency; completed evidence may already have been reaped
	// and, for cross-ledger groups, belongs to the origin execution's coordinator
	// scope rather than the reversed batch's first ledger.
	if resolution.Pending && resolution.ExecutionID != uuid.Nil {
		part.dependency = originDependencyReference(
			tmcore.GetTenantIDContext(ctx),
			organizationID,
			ledgerID,
			transactionID,
			resolution.ExecutionID,
		)
	}

	return part, nil
}

func buildCrossLedgerRevertBatchInput(
	in RevertTransactionInput,
	revertedGroupID, newGroupID uuid.UUID,
	parts []preparedCrossLedgerRevertPart,
) (CreateAtomicTransactionBatchV2Input, error) {
	items := make([]CreateAtomicTransactionBatchV2ItemInput, len(parts))
	for outputIndex := range parts {
		partIndex := len(parts) - 1 - outputIndex

		part := parts[partIndex]
		if part.origin == nil {
			return CreateAtomicTransactionBatchV2Input{}, errors.New("cross-ledger revert part has no origin")
		}

		organizationID, err := uuid.Parse(part.origin.OrganizationID)
		if err != nil {
			return CreateAtomicTransactionBatchV2Input{}, fmt.Errorf("parse cross-ledger member organization id: %w", err)
		}

		ledgerID, err := uuid.Parse(part.origin.LedgerID)
		if err != nil {
			return CreateAtomicTransactionBatchV2Input{}, fmt.Errorf("parse cross-ledger member ledger id: %w", err)
		}

		originID, err := uuid.Parse(part.origin.ID)
		if err != nil {
			return CreateAtomicTransactionBatchV2Input{}, fmt.Errorf("parse cross-ledger member transaction id: %w", err)
		}

		item := CreateAtomicTransactionBatchV2ItemInput{
			OrganizationID:      organizationID,
			LedgerID:            ledgerID,
			Transaction:         part.reversal,
			ParentTransactionID: &originID,
			Action:              constant.ActionRevert,
			Order:               outputIndex + 1,
			OriginalIndex:       partIndex,
		}
		if part.dependency.ExecutionID != uuid.Nil {
			item.Dependencies = []TransactionEvidenceReference{part.dependency}
		}

		if originID == in.TransactionID {
			item.AccountBlockExceptionID = cloneUUIDPointer(in.AccountBlockExceptionID)
		}

		items[outputIndex] = item
	}

	canonical := []byte("revert-group:" + revertedGroupID.String())
	fingerprintDigest := sha256.Sum256(canonical)

	return CreateAtomicTransactionBatchV2Input{
		Transactions:       items,
		GroupID:            &newGroupID,
		CrossLedgerGroup:   true,
		CanonicalRequest:   canonical,
		RequestFingerprint: hex.EncodeToString(fingerprintDigest[:]),
		IdempotencyKey:     crossLedgerRevertIdempotencyKey(revertedGroupID),
		IdempotencyTTL:     pkgHTTP.ParseIdempotencyTTL(""),
	}, nil
}

func validateCrossLedgerRevertMembers(
	requestedID, groupID uuid.UUID,
	members []*transaction.Transaction,
) error {
	if len(members) < 2 {
		return pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}

	foundRequested := false

	for _, member := range members {
		if member == nil || member.GroupID == nil || *member.GroupID != groupID.String() {
			return pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
		}

		if member.ID == requestedID.String() {
			foundRequested = true
		}
	}

	if !foundRequested {
		return pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}

	return nil
}

func (uc *UseCase) withCrossLedgerRevertMemberError(
	ctx context.Context,
	in RevertTransactionInput,
	target *transaction.Transaction,
	primary error,
) error {
	if target == nil || target.GroupID == nil {
		return primary
	}

	groupID, err := uuid.Parse(*target.GroupID)
	if err != nil {
		return primary
	}

	resolver, ok := uc.TransactionReader.(TransactionGroupMemberResolver)
	if !ok {
		return primary
	}

	members, err := resolver.ResolveTransactionGroupMembers(
		readrouting.WithPrimaryRead(ctx),
		in.OrganizationID,
		in.LedgerID,
		in.TransactionID,
		groupID,
	)
	if err != nil {
		return primary
	}

	for index, member := range members {
		if member == nil || member.ID != in.TransactionID.String() {
			continue
		}

		message := fmt.Sprintf("transaction %s in ledger %s is not revertible", member.ID, member.LedgerID)

		return withAtomicTransactionBatchItemError(primary, index, message)
	}

	return primary
}

func originDependencyReference(
	tenantID string,
	organizationID, ledgerID, transactionID, executionID uuid.UUID,
) TransactionEvidenceReference {
	return TransactionEvidenceReference{
		Kind:           TransactionDependencyOrigin,
		TenantID:       tenantID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  transactionID,
		ExecutionID:    executionID,
	}
}

func crossLedgerRevertIdempotencyKey(revertedGroupID uuid.UUID) string {
	// The Redis record already has the batch's deterministic coordination scope.
	// Including the path member here would let two members claim independently.
	return "revert-group:" + revertedGroupID.String()
}
