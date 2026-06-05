// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package composition orchestrates the two existing primitives that open a
// holder-owned account: the onboarding account-create use case and the CRM
// instrument-create use case. It composes; it never reimplements. Double-entry,
// the holder gate, and instrument validation are inherited unchanged from the
// composed use cases. The orchestrator owns no repositories and depends only on
// the two use-case surfaces through narrow ports, so dependencies flow inward.
package composition

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// instrumentFailureStatus is the typed partial-failure status surfaced when the
// account committed but the instrument write failed.
const instrumentFailureStatus = "FAILED"

// instrumentFailureFallbackReason is the stable, client-actionable reason code
// returned when the instrument error carries no recognizable business code. It
// is the internal-server sentinel so the client sees a stable token, never the
// raw internal error text.
var instrumentFailureFallbackReason = constant.ErrInternalServer.Error()

// AccountCreator is the narrow port for the onboarding account-create use case.
// It is satisfied by *command.UseCase.
type AccountCreator interface {
	CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, in *mmodel.CreateAccountInput, token string) (*mmodel.Account, error)
}

// InstrumentCreator is the narrow port for the CRM instrument-create use case.
// It is satisfied by *crmservices.UseCase.
type InstrumentCreator interface {
	CreateInstrument(ctx context.Context, organizationID string, holderID uuid.UUID, in *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error)
}

// Service orchestrates account creation and optional instrument creation behind
// a single call. It holds the two composed use-case surfaces as narrow ports
// and no repositories of its own.
type Service struct {
	Accounts    AccountCreator
	Instruments InstrumentCreator
}

// NewService builds a composition Service from the two composed use-case
// surfaces.
func NewService(accounts AccountCreator, instruments InstrumentCreator) *Service {
	return &Service{Accounts: accounts, Instruments: instruments}
}

// CreateHolderAccount opens a holder-owned account and, when instrument fields
// are present, an instrument linked to it.
//
// Ownership is path-sourced: the account's HolderID is set exclusively from the
// holderID argument (the path :id), never from the request body. The account
// create runs first; on error it is returned verbatim and no instrument is
// attempted. When the account commits but the instrument write fails the
// account REMAINS persisted and usable: no compensating delete fires, the
// failure is logged at Warn and span-recorded, and a typed InstrumentError
// block is surfaced for client-driven retry.
func (s *Service) CreateHolderAccount(ctx context.Context, organizationID, ledgerID, holderID uuid.UUID, in *mmodel.CreateHolderAccountInput, token string) (*mmodel.HolderAccountResponse, error) {
	logger, tracer, requestID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "composition.create_holder_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", requestID),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	account, err := s.Accounts.CreateAccount(ctx, organizationID, ledgerID, in.ToCreateAccountInput(holderID.String()), token)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create account", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create account", libLog.Err(err))

		return nil, err
	}

	resp := &mmodel.HolderAccountResponse{Account: account}

	if !hasInstrumentFields(in) {
		return resp, nil
	}

	instrumentInput := &mmodel.CreateInstrumentInput{
		LedgerID:         ledgerID.String(),
		AccountID:        account.ID,
		Metadata:         in.Metadata,
		BankingDetails:   in.BankingDetails,
		RegulatoryFields: in.RegulatoryFields,
		RelatedParties:   in.RelatedParties,
	}

	instrument, err := s.Instruments.CreateInstrument(ctx, organizationID.String(), holderID, instrumentInput)
	if err != nil {
		// Non-compensating partial failure: the account stays persisted and
		// usable, no delete fires, and the failure is surfaced for retry.
		reason := instrumentFailureReason(err)

		libOpentelemetry.HandleSpanError(span, "Failed to create instrument; account remains persisted", err)
		logger.Log(ctx, libLog.LevelWarn, "Instrument create failed after account committed; account remains persisted",
			libLog.String("instrument_failure_reason", reason),
			libLog.Err(err),
		)

		resp.InstrumentError = &mmodel.InstrumentFailure{
			Status: instrumentFailureStatus,
			Reason: reason,
		}

		return resp, nil
	}

	resp.Instrument = instrument

	return resp, nil
}

// hasInstrumentFields reports whether the composite request carries instrument
// fields, gating instrument creation (D-8: no banking details, no instrument).
//
// The predicate is nil-pointer based: an empty-but-present bankingDetails{} (a
// non-nil pointer to a zero value) DOES trigger an instrument, because the
// caller explicitly asked for one. This is the documented contract, not an
// accident; the unit test locks it.
func hasInstrumentFields(in *mmodel.CreateHolderAccountInput) bool {
	return in.BankingDetails != nil || in.RegulatoryFields != nil || len(in.RelatedParties) > 0
}

// instrumentFailureReason extracts a stable, client-actionable reason code from
// the instrument-create error. It returns the business error's Code when the
// error is a recognized typed midaz error, and a stable fallback sentinel
// otherwise. It never returns the raw internal error text.
func instrumentFailureReason(err error) string {
	var (
		notFound      pkg.EntityNotFoundError
		validation    pkg.ValidationError
		conflict      pkg.EntityConflictError
		unprocessable pkg.UnprocessableOperationError
		failedPre     pkg.FailedPreconditionError
		unavailable   pkg.ServiceUnavailableError
		internal      pkg.InternalServerError
		response      pkg.ResponseError
	)

	switch {
	case errors.As(err, &notFound) && notFound.Code != "":
		return notFound.Code
	case errors.As(err, &validation) && validation.Code != "":
		return validation.Code
	case errors.As(err, &conflict) && conflict.Code != "":
		return conflict.Code
	case errors.As(err, &unprocessable) && unprocessable.Code != "":
		return unprocessable.Code
	case errors.As(err, &failedPre) && failedPre.Code != "":
		return failedPre.Code
	case errors.As(err, &unavailable) && unavailable.Code != "":
		return unavailable.Code
	case errors.As(err, &internal) && internal.Code != "":
		return internal.Code
	case errors.As(err, &response) && response.Code != "":
		return response.Code
	}

	return instrumentFailureFallbackReason
}
