// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
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
		recordCommandError(ctx, span, logger, "Failed to find account by id", err)

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
		}

		recordCommandError(ctx, span, logger, "Failed to update account on repo by id", err)

		return nil, err
	}

	if uai.Blocked != nil {
		if err := uc.writeAccountConfiguration(ctx, span, logger, organizationID, ledgerID, accountUpdated); err != nil {
			return nil, err
		}
	}

	// The event is built from the pre-update read merged with the patch, so it
	// keeps the holder fields that AccountRepo.Update's projection leaves out.
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

// writeAccountConfiguration makes the blocked flag an account UPDATE committed
// the one transactions obey. It writes the account's configuration key with the
// row the UPDATE returned and its version, then confirms the copy of the flag in
// every cached blob of the account's balances, which releases reading the flag
// from the blob obey (a compatibility bridge, removable once no rollback target
// reads it there). The blobs are read in one MGET and only the ones holding an
// older or a divergent copy are written, through a compare-and-set that never
// touches money. Blobs not in cache are skipped: the next load reads the flag
// from PostgreSQL.
//
// Fail-closed: Blocked is a security control, so a listing or Redis failure, or a
// copy still unconfirmed after the bounded compare-and-set attempts, fails the
// PATCH even though PostgreSQL already committed. The client's retry is
// idempotent and completes the write. These failures are TECHNICAL
// (infrastructure, not caller error), so they flip the span red and log at Error
// for operator attention. Unblocking an account that was never blocked follows
// the same path and is a natural no-op (RF-02).
func (uc *UseCase) writeAccountConfiguration(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, account *mmodel.Account) error {
	config, err := balancecache.AccountConfigFromAccount(account)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to read the committed account configuration", err)

		return err
	}

	if _, _, err := uc.TransactionRedisRepo.WriteAccountConfig(ctx, organizationID, ledgerID, config); err != nil {
		recordCommandError(ctx, span, logger, "Failed to write the account configuration key", err,
			libLog.String("account_id", config.AccountID.String()), libLog.Any("config_version", config.Version))

		return err
	}

	balances, err := uc.BalanceRepo.ListByAccountID(ctx, organizationID, ledgerID, config.AccountID)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to list balances for the account configuration copy", err)

		return err
	}

	// Dedupe: a legacy balance row with an empty key normalizes to the
	// default key and may collide with an explicit "default" balance.
	seen := make(map[string]struct{}, len(balances))
	refs := make([]string, 0, len(balances))

	for _, b := range balances {
		ref := balanceCacheRef(b.Alias, b.Key)
		if _, ok := seen[ref]; ok {
			continue
		}

		seen[ref] = struct{}{}

		refs = append(refs, ref)
	}

	if len(refs) == 0 {
		return nil
	}

	outcomes, err := uc.TransactionRedisRepo.CopyAccountConfigToBlobs(ctx, organizationID, ledgerID, refs, config)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to copy the account configuration to the balance cache", err,
			libLog.String("account_id", config.AccountID.String()))

		return err
	}

	unconfirmed := 0

	for _, outcome := range outcomes {
		if !outcome.Confirmed() {
			unconfirmed++
		}
	}

	span.SetAttributes(attribute.Int("app.account_config.blob_copy_unconfirmed", unconfirmed))

	if unconfirmed > 0 {
		err := fmt.Errorf("account %s version %d, %d balances: %w", config.AccountID, config.Version, unconfirmed, errConfigurationCopyNotConfirmed)
		recordCommandError(ctx, span, logger, "Account configuration copy not confirmed", err)

		return err
	}

	return nil
}

// emitAccountUpdatedEvent publishes the account.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// The persisted database mutation is durable; this helper does not make broker delivery transactional.
//
// Anchor: invoked after the fail-closed blocked configuration write and
// before the metadata-write call in UpdateAccount, so a propagation failure
// short-circuits before the event is announced, a downstream Mongo failure
// cannot mask the event, and an update rollback cannot leak it.
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
