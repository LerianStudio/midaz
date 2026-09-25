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
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateCrossLedgerHoldV2 persists the complete normalized intent and reserves
// only the source-ledger parts. Destination-ledger parts are created on commit.
func (uc *UseCase) CreateCrossLedgerHoldV2(
	ctx context.Context,
	in CreateCrossLedgerTransactionV2Input,
) (result *CreateAtomicTransactionBatchV2Result, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_cross_ledger_hold_v2")
	defer span.End()

	start := time.Now()

	defer func() {
		recordCrossLedgerGroupError(ctx, span, logger, "Failed to create cross-ledger hold", err)
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", crossLedgerOperationCreateHold, start, err)
	}()

	span.SetAttributes(attribute.String("app.request.action", constant.ActionHold))

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if uc.TransactionGroupRepo == nil || uc.TransactionReader == nil {
		return nil, errors.New("cross-ledger hold dependencies are not configured")
	}

	if uc.UUIDv7Generator == nil || uc.Clock == nil {
		return nil, errors.New("cross-ledger hold identity dependencies are not configured")
	}

	groupID, err := uc.UUIDv7Generator()
	if err != nil {
		return nil, fmt.Errorf("generate cross-ledger hold group id: %w", err)
	}

	if groupID == uuid.Nil {
		return nil, errors.New("cross-ledger hold UUIDv7 generator returned a nil group id")
	}

	span.SetAttributes(attribute.String("app.response.group_id", groupID.String()))

	parts, err := decomposeCrossLedgerTransaction(in.Transaction, internalCrossLedgerScopes(in.Scopes))
	if err != nil {
		return nil, err
	}

	intent, err := buildCrossLedgerGroupIntent(in.Transaction.Send.Asset, parts)
	if err != nil {
		return nil, err
	}

	ledgers := setCrossLedgerGroupShape(span, crossLedgerIntentLedgerRefs(intent))

	if err := uc.validateCrossLedgerHoldSettings(ctx, intent); err != nil {
		return nil, err
	}

	if err := uc.persistCrossLedgerHoldIntent(ctx, groupID, intent); err != nil {
		return nil, err
	}

	batch, err := buildCrossLedgerHoldBatchInput(in, groupID, intent)
	if err != nil {
		uc.discardCrossLedgerHoldIntent(ctx, groupID)
		return nil, err
	}

	result, err = uc.executeAtomicTransactionBatchV2(ctx, batch)
	if err != nil {
		if isAtomicTransactionBatchPrePublication(err) {
			uc.discardCrossLedgerHoldIntent(ctx, groupID)
		}

		return nil, err
	}

	uc.settleCrossLedgerHoldResult(ctx, groupID, ledgers, result)

	return result, nil
}

func (uc *UseCase) settleCrossLedgerHoldResult(
	ctx context.Context,
	groupID uuid.UUID,
	ledgers int,
	result *CreateAtomicTransactionBatchV2Result,
) {
	if result == nil {
		return
	}

	if !result.Replayed {
		uc.recordCrossLedgerGroupLedgers(ctx, constant.ActionHold, ledgers)
		return
	}

	// A replay executes nothing and answers with the original group, so the
	// intent row persisted for this request never gains members.
	if result.BatchID != groupID {
		uc.discardCrossLedgerHoldIntent(ctx, groupID)
	}
}

// discardCrossLedgerHoldIntent removes the PENDING group row of a hold that
// executed nothing. It outlives the request context so a canceled request
// still cleans up; a failure leaves the row to the group reconciler.
func (uc *UseCase) discardCrossLedgerHoldIntent(ctx context.Context, groupID uuid.UUID) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncOperationTimeout)
	defer cancel()

	if err := uc.TransactionGroupRepo.Delete(cleanupCtx, groupID); err != nil {
		logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
		logger.Log(ctx, libLog.LevelWarn, "Failed to discard cross-ledger hold intent",
			libLog.String("group_id", groupID.String()), libLog.Err(err))
	}
}

// persistCrossLedgerHoldIntent stores the PENDING group row, owned by the first
// part's ledger, before any accounting runs.
func (uc *UseCase) persistCrossLedgerHoldIntent(ctx context.Context, groupID uuid.UUID, intent CrossLedgerGroupIntent) error {
	rawIntent, err := encodeCrossLedgerGroupIntent(intent)
	if err != nil {
		return err
	}

	now := uc.Clock()
	if now.IsZero() {
		return errors.New("cross-ledger hold clock returned a zero timestamp")
	}

	primary := intent.Parts[0]

	group := &transactiongroup.TransactionGroup{
		ID: groupID, OrganizationID: primary.OrganizationID, LedgerID: primary.LedgerID,
		Status: constant.PENDING, AssetCode: intent.Asset, Intent: rawIntent,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := uc.TransactionGroupRepo.Create(ctx, group); err != nil {
		return fmt.Errorf("persist cross-ledger hold intent: %w", err)
	}

	return nil
}

func (uc *UseCase) validateCrossLedgerHoldSettings(ctx context.Context, intent CrossLedgerGroupIntent) error {
	seen := make(map[atomicTransactionBatchLedgerRef]struct{}, len(intent.Parts))
	for index := range intent.Parts {
		part := intent.Parts[index]

		ref := atomicTransactionBatchLedgerRef{organizationID: part.OrganizationID, ledgerID: part.LedgerID}
		if _, ok := seen[ref]; ok {
			continue
		}

		seen[ref] = struct{}{}

		settings, err := uc.TransactionReader.GetParsedLedgerSettings(ctx, part.OrganizationID, part.LedgerID)
		if err != nil {
			return fmt.Errorf("get cross-ledger hold settings: %w", err)
		}

		if !settings.CrossLedger.Enabled {
			return pkg.ValidateBusinessError(constant.ErrCrossLedgerNotEnabled, constant.EntityLedger, part.LedgerID.String())
		}

		if settings.Accounting.ValidateRoutes {
			return pkg.ValidateBusinessError(constant.ErrCrossLedgerRouteValidationUnsupported, constant.EntityLedger)
		}
	}

	return nil
}

func buildCrossLedgerHoldBatchInput(
	in CreateCrossLedgerTransactionV2Input,
	groupID uuid.UUID,
	intent CrossLedgerGroupIntent,
) (CreateAtomicTransactionBatchV2Input, error) {
	items := make([]CreateAtomicTransactionBatchV2ItemInput, 0, len(intent.Parts))
	for index := range intent.Parts {
		part := intent.Parts[index]
		if part.Role != CrossLedgerGroupRoleOrigin {
			continue
		}

		transactionInput := part.Transaction
		transactionInput.Pending = true
		items = append(items, CreateAtomicTransactionBatchV2ItemInput{
			OrganizationID: part.OrganizationID,
			LedgerID:       part.LedgerID,
			Transaction:    transactionInput,
			Action:         constant.ActionHold,
			Order:          len(items) + 1,
			OriginalIndex:  index,
		})
	}

	if len(items) == 0 {
		return CreateAtomicTransactionBatchV2Input{}, errors.New("cross-ledger hold has no origin parts")
	}

	items[0].AccountBlockExceptionID = cloneUUIDPointer(in.AccountBlockExceptionID)

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

func (uc *UseCase) executeAtomicTransactionBatchV2(
	ctx context.Context,
	in CreateAtomicTransactionBatchV2Input,
) (*CreateAtomicTransactionBatchV2Result, error) {
	if uc.createAtomicTransactionBatchV2 != nil {
		return uc.createAtomicTransactionBatchV2(ctx, in)
	}

	return uc.CreateAtomicTransactionBatchV2(ctx, in)
}

func internalCrossLedgerScopes(scopes CrossLedgerTransactionScopes) crossLedgerTransactionScopes {
	result := crossLedgerTransactionScopes{
		from: make([]atomicTransactionBatchLedgerRef, len(scopes.Debits)),
		to:   make([]atomicTransactionBatchLedgerRef, len(scopes.Credits)),
	}
	for index := range scopes.Debits {
		result.from[index] = atomicTransactionBatchLedgerRef{
			organizationID: scopes.Debits[index].OrganizationID,
			ledgerID:       scopes.Debits[index].LedgerID,
		}
	}

	for index := range scopes.Credits {
		result.to[index] = atomicTransactionBatchLedgerRef{
			organizationID: scopes.Credits[index].OrganizationID,
			ledgerID:       scopes.Credits[index].LedgerID,
		}
	}

	return result
}
