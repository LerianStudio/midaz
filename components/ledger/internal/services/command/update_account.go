// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"slices"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// UpdateAccount updates an account from the repository by the given ID.
func (uc *UseCase) UpdateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, uai *mmodel.UpdateAccountInput, holderPolicy mmodel.HolderPolicy) (_ *mmodel.Account, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_account", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", id.String()),
	)

	// The lookup carries the route's policy even though nothing downstream reads
	// the projected holder — holderId is immutable, absent from the update SET
	// list, and account.updated has no holder field. It matters for ORDERING: on a
	// schema without the holder columns a /v2 request has to fail here, before the
	// row is mutated and account.updated is emitted, rather than after. /v1 needs
	// no such column, so it proceeds and completes.
	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id, holderPolicy)
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

	if uai.Blocked != nil {
		uc.propagateAccountBlockedToCache(ctx, span, logger, organizationID, ledgerID, id, *uai.Blocked)
	}

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

// propagateAccountBlockedToCache rewrites the Blocked flag in place on every
// cached balance blob of the account after a PATCH that carries blocked.
// PostgreSQL was updated first (source of truth); the rewrite runs as ONE
// atomic multi-key Lua EVAL, preserving live transactional state pending
// write-behind sync (no DEL). Keys not in cache are skipped — the on-demand
// hydration covers them with the new value on the next miss.
//
// Best-effort: a database or Redis failure never fails the request — the
// persisted row is durable, a stale cached flag heals on the next cache miss
// or TTL expiry, and failing here would report an update that DID happen as
// failed. Those failures are still TECHNICAL (infrastructure, not caller
// error), so they flip the span red and log at Error for operator attention.
// Unblocking an account that was never blocked follows the same path and is
// a natural no-op (RF-02).
func (uc *UseCase) propagateAccountBlockedToCache(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID, blocked bool) {
	balances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list balances for blocked cache propagation", err)
		logger.Log(ctx, libLog.LevelError, "Failed to list balances for blocked cache propagation", libLog.Err(err))

		return
	}

	if len(balances) == 0 {
		return
	}

	// Dedupe: a legacy balance row with an empty key normalizes to the
	// default key and may collide with an explicit "default" balance.
	seen := make(map[string]struct{}, len(balances))
	cacheKeys := make([]string, 0, len(balances))

	for _, b := range balances {
		balanceKey := b.Key
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		cacheKey := b.Alias + "#" + balanceKey
		if _, ok := seen[cacheKey]; ok {
			continue
		}

		seen[cacheKey] = struct{}{}

		cacheKeys = append(cacheKeys, cacheKey)
	}

	if err := uc.TransactionRedisRepo.UpdateBalanceCacheBlocked(ctx, organizationID, ledgerID, cacheKeys, blocked); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update balance cache blocked flag", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update balance cache blocked flag", libLog.Err(err))
	}
}

// emitAccountUpdatedEvent publishes the account.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// The persisted database mutation is durable; this helper does not make broker delivery transactional.
//
// Anchor: invoked between the AccountRepo.Update success branch and the
// metadata-write call in UpdateAccount, so a downstream Mongo failure
// cannot mask the event and an update rollback cannot leak it.
//
// Wire-format mapping lives in pkg/streaming/events/account_updated.go;
// changes to the payload contract belong there, not here. This function
// stays a thin emit-and-log adapter.
func (uc *UseCase) emitAccountUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, acc *mmodel.Account) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AccountUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountUpdated(acc).ToEmitRequest(tenantID, acc.UpdatedAt)
		})
}
