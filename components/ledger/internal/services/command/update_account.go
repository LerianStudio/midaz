// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"slices"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UpdateAccount updates an account from the repository by the given ID.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", id.String()),
	)

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find account by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to find account by id", libLog.Err(err))

		return nil, err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return nil, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount)
	}

	account := &mmodel.Account{
		Name:        uai.Name,
		Status:      uai.Status,
		EntityID:    uai.EntityID,
		SegmentID:   uai.SegmentID,
		PortfolioID: uai.PortfolioID,
		Metadata:    uai.Metadata,
		NullFields:  uai.NullFields,
		Blocked:     uai.Blocked,
	}

	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

			logger.Log(ctx, libLog.LevelWarn, "Account ID not found on update", libLog.String("account_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to update account on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update account on repo by id", libLog.Err(err))

		return nil, err
	}

	// AccountRepo.Update returns an input-derived record with bogus
	// identity fields; mirror the SQL merge in-memory instead.
	// Follow-up: fix the repo to RETURNING * so this dance is unneeded.
	uc.emitAccountUpdatedEvent(ctx, span, logger, mergePatchAccount(accFound, account, accountUpdated.UpdatedAt))

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntityAccount, id.String(), uai.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update metadata", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update metadata", libLog.Err(err))

		return nil, err
	}

	accountUpdated.Metadata = metadataUpdated

	return accountUpdated, nil
}

// mergePatchAccount builds the post-update view of an account in-memory
// by applying the PATCH-style mutation rules SQL-side from the repo
// Update (see account.postgresql.go: Update + applyNullableFields).
// Mirrors RFC 7396 semantics: a non-empty input field overrides, an
// empty input field with the key in NullFields nulls the row, otherwise
// the pre-update value is preserved. Caller passes the persisted
// UpdatedAt from the repo so the event carries the same timestamp the
// row now has.
//
// Uses libCommons.IsNilOrEmpty for *string fields to match the repo's
// applyNullableFields; PROJECT_RULES prefers `!= nil` for PATCH inputs,
// but the emission contract must reflect what was actually persisted —
// so this helper stays consistent with the SQL until the repo migrates.
func mergePatchAccount(pre, in *mmodel.Account, updatedAt time.Time) *mmodel.Account {
	out := *pre
	out.UpdatedAt = updatedAt

	if in.Name != "" {
		out.Name = in.Name
	}

	if !in.Status.IsEmpty() {
		out.Status = in.Status
	}

	if in.Blocked != nil {
		out.Blocked = in.Blocked
	}

	if !libCommons.IsNilOrEmpty(in.SegmentID) {
		out.SegmentID = in.SegmentID
	} else if slices.Contains(in.NullFields, "segmentId") {
		out.SegmentID = nil
	}

	if !libCommons.IsNilOrEmpty(in.EntityID) {
		out.EntityID = in.EntityID
	} else if slices.Contains(in.NullFields, "entityId") {
		out.EntityID = nil
	}

	if !libCommons.IsNilOrEmpty(in.PortfolioID) {
		out.PortfolioID = in.PortfolioID
	} else if slices.Contains(in.NullFields, "portfolioId") {
		out.PortfolioID = nil
	}

	return &out
}

// emitAccountUpdatedEvent publishes the account.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked between the AccountRepo.Update success branch and the
// metadata-write call in UpdateAccount, so a downstream Mongo failure
// cannot mask the event and an update rollback cannot leak it.
//
// Wire-format mapping lives in pkg/streaming/events/account_updated.go;
// changes to the payload contract belong there, not here. This function
// stays a thin emit-and-log adapter.
func (uc *UseCase) emitAccountUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, acc *mmodel.Account) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AccountUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountUpdated(acc).ToEmitRequest(tenantID, acc.UpdatedAt)
		})
}
