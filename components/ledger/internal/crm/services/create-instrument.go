// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (uc *UseCase) CreateInstrument(ctx context.Context, organizationID string, holderID uuid.UUID, cai *mmodel.CreateInstrumentInput) (_ *mmodel.Instrument, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_instrument")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "create_instrument", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	instrumentID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate instrument id", err)

		return nil, err
	}

	now := time.Now()

	instrument := &mmodel.Instrument{
		ID:        &instrumentID,
		LedgerID:  &cai.LedgerID,
		AccountID: &cai.AccountID,
		HolderID:  &holderID,
		Metadata:  cai.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if cai.BankingDetails != nil {
		instrument.BankingDetails = &mmodel.BankingDetails{
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
		instrument.RegulatoryFields = &mmodel.RegulatoryFields{
			ParticipantDocument: cai.RegulatoryFields.ParticipantDocument,
		}
	}

	if len(cai.RelatedParties) > 0 {
		if err := uc.ValidateRelatedParties(ctx, cai.RelatedParties); err != nil {
			recordSpanError(span, "Failed to validate related parties", err)

			return nil, err
		}

		for _, rp := range cai.RelatedParties {
			rpID, rpErr := libCommons.GenerateUUIDv7()
			if rpErr != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to generate related party id", rpErr)

				return nil, rpErr
			}

			rp.ID = &rpID
		}

		instrument.RelatedParties = cai.RelatedParties
	}

	holder, err := uc.GetHolderByID(ctx, organizationID, holderID, false)
	if err != nil {
		recordSpanError(span, "Failed to get holder by id", err)

		return nil, err
	}

	instrument.Document = holder.Document
	instrument.Type = holder.Type

	// organizationID is a route-validated path param already consumed by
	// GetHolderByID above, so it is a well-formed UUID here; parse it at this
	// boundary and hand the helper a uuid.UUID so the helper stays focused on
	// the genuinely untrusted body references.
	organizationUUID, err := uuid.Parse(organizationID)
	if err != nil {
		bErr := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityInstrument, "organizationId")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid organization id for instrument", bErr)

		return nil, bErr
	}

	if err = uc.validateInstrumentReferences(ctx, span, organizationUUID, cai.LedgerID, cai.AccountID); err != nil {
		return nil, err
	}

	createdInstrument, err := uc.InstrumentRepo.Create(ctx, organizationID, instrument)
	if err != nil {
		recordSpanError(span, "Failed to create instrument", err)

		return nil, err
	}

	return createdInstrument, nil
}

// validateInstrumentReferences verifies the body-supplied ledger and account
// exist within the request organization before the instrument is persisted.
// Malformed body UUIDs return a 400-class validation error; a non-existent
// ledger or account returns the 422 referential sentinel (NOT the query layer's
// 404, since the addressed instrument route is well-formed — only its
// references are invalid). Validation order is ledger first, then account within
// that ledger, because the account lookup is ledger-partitioned. The
// organization is passed pre-parsed because it is a route-validated path param,
// not untrusted body input.
func (uc *UseCase) validateInstrumentReferences(ctx context.Context, span trace.Span, organizationUUID uuid.UUID, ledgerIDStr, accountIDStr string) error {
	ledgerUUID, err := uuid.Parse(ledgerIDStr)
	if err != nil {
		bErr := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityInstrument, "ledgerId")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ledger id for instrument references", bErr)

		return bErr
	}

	accountUUID, err := uuid.Parse(accountIDStr)
	if err != nil {
		bErr := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityInstrument, "accountId")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account id for instrument references", bErr)

		return bErr
	}

	ledgerExists, err := uc.LedgerAccounts.LedgerExists(ctx, organizationUUID, ledgerUUID)
	if err != nil {
		recordSpanError(span, "Failed to verify instrument ledger reference", err)

		return err
	}

	if !ledgerExists {
		bErr := pkg.ValidateBusinessError(constant.ErrInstrumentLedgerReferenceNotFound, constant.EntityInstrument)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Instrument ledger reference not found", bErr)

		return bErr
	}

	accountExists, err := uc.LedgerAccounts.AccountExists(ctx, organizationUUID, ledgerUUID, accountUUID)
	if err != nil {
		recordSpanError(span, "Failed to verify instrument account reference", err)

		return err
	}

	if !accountExists {
		bErr := pkg.ValidateBusinessError(constant.ErrInstrumentAccountReferenceNotFound, constant.EntityInstrument)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Instrument account reference not found", bErr)

		return bErr
	}

	return nil
}
