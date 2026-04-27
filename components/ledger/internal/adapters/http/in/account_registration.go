// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
)

// AccountRegistrationHandler groups the command + query use cases behind the HTTP
// surface for the Ledger-owned account-registration saga. See
// components/ledger/internal/services/command/create_account_registration.go for the
// orchestration contract and pkg/mmodel/account_registration.go for the durable model.
type AccountRegistrationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// AccountRegistrationResponse is the public response envelope for a single saga
// record. It embeds the registration and attaches the downstream artifacts (Account
// and Alias) when they exist, so clients can get everything in one round-trip for
// happy-path creates without a second GET.
//
// swagger:model AccountRegistrationResponse
// @Description AccountRegistrationResponse payload
type AccountRegistrationResponse struct {
	Registration *mmodel.AccountRegistration `json:"registration"`
	Account      *mmodel.Account             `json:"account,omitempty"`
	Alias        *mmodel.Alias               `json:"alias,omitempty"`
} // @name AccountRegistrationResponse

// CreateAccountRegistration initiates the Ledger-owned account-registration saga.
//
//	@Summary		Create a new account registration
//	@Description	Initiates the Ledger-owned account-registration saga to coordinate Ledger account creation with CRM holder-alias creation. The response carries the durable saga record plus the created Account and Alias on happy path.
//	@Tags			Account Registrations
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string											true	"Authorization Bearer Token"
//	@Param			Idempotency-Key	header		string											true	"Idempotency key (required)"
//	@Param			X-Request-Id	header		string											false	"Request ID for tracing"
//	@Param			organization_id	path		string											true	"Organization ID (UUID)"
//	@Param			ledger_id		path		string											true	"Ledger ID (UUID)"
//	@Param			registration	body		mmodel.CreateAccountRegistrationInput			true	"Holder ID, Ledger account payload, CRM alias payload"
//	@Success		201				{object}	AccountRegistrationResponse
//	@Failure		400				{object}	mmodel.Error
//	@Failure		401				{object}	mmodel.Error
//	@Failure		409				{object}	mmodel.Error
//	@Failure		422				{object}	mmodel.Error
//	@Failure		503				{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-registrations [post]
func (h *AccountRegistrationHandler) CreateAccountRegistration(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account_registration")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload, ok := i.(*mmodel.CreateAccountRegistrationInput)
	if !ok {
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidRequestBody, constant.EntityAccountRegistration))
	}

	logSafePayload(ctx, logger, "Request to create an account registration", payload)
	recordSafePayloadAttributes(span, payload)

	idempotencyKey, _ := http.GetIdempotencyKeyAndTTL(c)
	if idempotencyKey == "" {
		err := pkg.ValidateBusinessError(constant.ErrIdempotencyKeyRequired, constant.EntityAccountRegistration)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Idempotency-Key header missing", err)

		return http.WithError(c, err)
	}

	token := c.Get("Authorization")

	reg, account, alias, err := h.Command.CreateAccountRegistration(ctx, organizationID, ledgerID, payload, idempotencyKey, token)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account registration saga returned an error", err)
		logger.Log(ctx, libLog.LevelWarn, "Account registration saga returned an error",
			libLog.String("organization_id", organizationID.String()),
			libLog.String("ledger_id", ledgerID.String()),
			libLog.Err(err))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Account registration saga completed",
		libLog.String("organization_id", organizationID.String()),
		libLog.String("ledger_id", ledgerID.String()),
		libLog.String("registration_id", reg.ID.String()),
		libLog.String("status", string(reg.Status)))

	return http.Created(c, AccountRegistrationResponse{
		Registration: reg,
		Account:      account,
		Alias:        alias,
	})
}

// GetAccountRegistration returns the durable saga record identified by path parameters.
//
//	@Summary		Get an account registration by ID
//	@Description	Fetches the durable state of an account-registration saga. Useful for polling a pending registration or inspecting a failed one.
//	@Tags			Account Registrations
//	@Produce		json
//	@Param			Authorization	header		string			true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID (UUID)"
//	@Param			ledger_id		path		string			true	"Ledger ID (UUID)"
//	@Param			id				path		string			true	"Account Registration ID (UUID)"
//	@Success		200				{object}	mmodel.AccountRegistration
//	@Failure		401				{object}	mmodel.Error
//	@Failure		404				{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-registrations/{id} [get]
func (h *AccountRegistrationHandler) GetAccountRegistration(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_registration")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	reg, err := h.Query.GetAccountRegistration(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to fetch account registration", err)

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Account registration retrieved",
		libLog.String("id", id.String()),
		libLog.String("status", string(reg.Status)))

	return http.OK(c, reg)
}
