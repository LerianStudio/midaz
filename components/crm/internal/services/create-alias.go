// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CreateAlias creates a new alias under a holder. When idempotencyKey is
// non-empty, the call is wrapped in the idempotency guard: identical
// (key, payload) tuples return the cached alias; same key with a different
// payload is rejected as ErrIdempotencyKey.
//
// When idempotencyKey is empty, the guard is bypassed and create-alias still
// enjoys domain-level idempotency from the alias repository (same
// (ledger_id, account_id, holder_id) under the SAME holder returns the
// existing alias; a DIFFERENT holder yields ErrAliasHolderConflict).
func (uc *UseCase) CreateAlias(ctx context.Context, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput, idempotencyKey string) (*mmodel.Alias, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.Bool("app.request.has_idempotency_key", idempotencyKey != ""),
	)

	requestHash, err := utils.CanonicalHashJSON(struct {
		HolderID string                   `json:"holder_id"`
		Input    *mmodel.CreateAliasInput `json:"input"`
	}{
		HolderID: holderID.String(),
		Input:    cai,
	})
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to canonicalize create-alias payload", err)
		logger.Log(ctx, libLog.LevelError, "Failed to canonicalize create-alias payload", libLog.Err(err))

		return nil, fmt.Errorf("create alias: canonical hash: %w", err)
	}

	return ExecuteIdempotent(ctx, uc.IdempotencyRepo, idempotencyKey, requestHash, 0,
		func(ctx context.Context) (*mmodel.Alias, error) {
			return uc.createAliasCore(ctx, organizationID, holderID, cai)
		},
	)
}

// createAliasCore is the original create-alias logic, preserved unchanged so
// behavior is identical for callers that bypass the idempotency guard (no
// header) and for cache-miss execution through the guard.
func (uc *UseCase) createAliasCore(ctx context.Context, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput) (*mmodel.Alias, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_alias.core")
	defer span.End()

	aliasID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to generate alias id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate alias id", libLog.Err(err))

		return nil, err
	}

	// Capture once and reuse so CreatedAt and UpdatedAt are guaranteed identical.
	now := time.Now().UTC()

	alias := &mmodel.Alias{
		ID:        &aliasID,
		LedgerID:  &cai.LedgerID,
		AccountID: &cai.AccountID,
		HolderID:  &holderID,
		Metadata:  cai.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if cai.BankingDetails != nil {
		alias.BankingDetails = &mmodel.BankingDetails{
			Branch:      cai.BankingDetails.Branch,
			Account:     cai.BankingDetails.Account,
			Type:        cai.BankingDetails.Type,
			OpeningDate: cai.BankingDetails.OpeningDate,
			ClosingDate: cai.BankingDetails.ClosingDate,
			IBAN:        cai.BankingDetails.IBAN,
			CountryCode: cai.BankingDetails.CountryCode,
			BankID:      cai.BankingDetails.BankID,
		}
	}

	if cai.RegulatoryFields != nil {
		alias.RegulatoryFields = &mmodel.RegulatoryFields{
			ParticipantDocument: cai.RegulatoryFields.ParticipantDocument,
		}
	}

	if len(cai.RelatedParties) > 0 {
		if err := uc.ValidateRelatedParties(ctx, cai.RelatedParties); err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to validate related parties", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to validate related parties", libLog.Err(err))

			return nil, err
		}

		for _, rp := range cai.RelatedParties {
			rpID, rpErr := libCommons.GenerateUUIDv7()
			if rpErr != nil {
				libOpenTelemetry.HandleSpanError(span, "Failed to generate related party id", rpErr)
				logger.Log(ctx, libLog.LevelError, "Failed to generate related party id", libLog.Err(rpErr))

				return nil, rpErr
			}

			rp.ID = &rpID
		}

		alias.RelatedParties = cai.RelatedParties
	}

	holderEntity, err := uc.GetHolderByID(ctx, organizationID, holderID, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get holder by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get holder by id",
			libLog.String("holder_id", holderID.String()), libLog.Err(err))

		return nil, err
	}

	alias.Document = holderEntity.Document
	alias.Type = holderEntity.Type

	createdAlias, err := uc.AliasRepo.Create(ctx, organizationID, alias)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create alias", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create alias", libLog.Err(err))

		return nil, err
	}

	return createdAlias, nil
}
