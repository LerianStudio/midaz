// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type AliasHandler struct {
	Service *services.UseCase
}

// CreateAlias is a method that creates Alias information linked with a specified Holder.
//
//	@Summary		Create an Alias Account
//	@Description	Enables a creation of an alias account, which represents an account in the ledger. The alias account is linked to specific business information, making it easier to manage and abstract account data within the system.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string					true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string					true	"The unique identifier of the Holder."
//	@Param			alias				body		mmodel.CreateAliasInput	true	"Alias Input"
//	@Success		201					{object}	mmodel.Alias
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases [post]
func (handler *AliasHandler) CreateAlias(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_alias")
	defer span.End()

	payload, ok := p.(*mmodel.CreateAliasInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	out, err := handler.Service.CreateAlias(ctx, organizationID, holderID, payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create alias: %v", err))

		return http.WithError(c, err)
	}

	return http.Created(c, out)
}

// GetAliasByID retrieves Alias details by a given id
//
//	@Summary		Retrieve Alias details
//	@Description	Retrieves detailed information about a specific alias using its unique identifier.
//	@Tags			Aliases
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string	true	"The unique identifier of the Holder."
//	@Param			alias_id			path		string	true	"The unique identifier of the Alias account."
//	@Param			include_deleted		query		string	false	"Returns the alias even if it was logically deleted."
//	@Success		200					{object}	mmodel.Alias
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases/{alias_id} [get]
func (handler *AliasHandler) GetAliasByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_alias_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")
	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating retrieval of Alias with ID: %s", id.String()))

	alias, err := handler.Service.GetAliasByID(ctx, organizationID, holderID, id, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to retrieve alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Alias with ID: %s from Holder %s, Error: %s", id.String(), holderID.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.OK(c, alias)
}

// UpdateAlias is a method that updates Holder information.
//
//	@Summary		Update an Alias
//	@Description	Update details of an alias.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string					true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string					true	"The unique identifier of the Holder."
//	@Param			alias_id			path		string					true	"The unique identifier of the Alias account."
//	@Param			alias				body		mmodel.UpdateAliasInput	true	"Alias Input"
//	@Success		200					{object}	mmodel.Alias
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases/{alias_id} [patch]
func (handler *AliasHandler) UpdateAlias(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_alias")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")

	payload, ok := p.(*mmodel.UpdateAliasInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	fieldsToRemove, ok := c.Locals("patchRemove").([]string)
	if !ok {
		libOpenTelemetry.HandleSpanError(span, "Failed to get fields to remove", cn.ErrInternalServer)

		logger.Log(ctx, libLog.LevelError, "Failed to get fields to remove")

		return http.WithError(c, cn.ErrInternalServer)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	)

	err = libOpenTelemetry.SetSpanAttributesFromValue(span, "app.request.fields_to_remove", fieldsToRemove, nil)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert fields_to_remove to JSON string", err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Request to update alias %s from holder %s", id.String(), holderID.String()))

	alias, err := handler.Service.UpdateAliasByID(ctx, organizationID, holderID, id, payload, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to update alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update alias %s from holder %s, Error: %s", id.String(), holderID.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.OK(c, alias)
}

// DeleteAliasByID removes an alias by a given id
//
//	@Summary		Delete an Alias
//	@Description	Delete an Alias. **Note:** By default, the delete endpoint performs a logical deletion (soft delete) of the entity in the system. If a physical deletion (hard delete) is required, you can use the query parameter outlined in the documentation.
//	@Tags			Aliases
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path	string	true	"The unique identifier of the Holder."
//	@Param			alias_id			path	string	true	"The unique identifier of the Alias account."
//	@Param			hard_delete			query	string	false	"Use only to perform a physical deletion of the data. This action is irreversible."
//	@Success		204
//	@Failure		400	{object}	pkg.HTTPError
//	@Failure		404	{object}	pkg.HTTPError
//	@Failure		500	{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases/{alias_id} [delete]
func (handler *AliasHandler) DeleteAliasByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_alias_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")
	hardDelete := http.GetBooleanParam(c, "hard_delete")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating removal of alias with ID: %s", id.String()))

	err = handler.Service.DeleteAliasByID(ctx, organizationID, holderID, id, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to delete alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete alias with ID: %s, Error: %s", id.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// GetAllAliases retrieves aliases
//
//	@Summary		List Aliases
//	@Description	List all Aliases with or without filters. CRM listing endpoints support pagination using the page, limit, and sort parameters. The sort parameter orders results by the entity ID using the UUID v7 standard, which is time-sortable, ensuring chronological ordering of the results.
//	@Tags			Aliases
//	@Produce		json
//	@Param			Authorization			header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id		header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id				query		string	false	"The unique identifier of the Holder."
//	@Param			metadata				query		string	false	"Metadata"
//	@Param			limit					query		int		false	"Limit"			default(10)
//	@Param			page					query		int		false	"Page"			default(1)
//	@Param			sort_order				query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			include_deleted			query		string	false	"Return includes logically deleted aliases."
//	@Param			account_id				query		string	false	"Filter alias by accountID"
//	@Param			ledger_id				query		string	false	"Filter alias by ledgerID"
//	@Param			document				query		string	false	"Filter alias by document"
//	@Param			banking_details_branch					query		string	false	"Filter alias by banking details branch"
//	@Param			banking_details_account					query		string	false	"Filter alias by banking details account"
//	@Param			banking_details_iban					query		string	false	"Filter alias by banking details iban"
//	@Param			banking_details_bank_id					query		string	false	"Filter alias by banking details bank ID"
//	@Param			banking_details_type					query		string	false	"Filter alias by banking details type"
//	@Param			regulatory_fields_participant_document	query		string	false	"Filter alias by regulatory fields participant document"
//	@Param			related_party_document					query		string	false	"Filter alias by related party document"
//	@Param			related_party_role						query		string	false	"Filter alias by related party role"
//	@Success		200										{object}	http.Pagination{items=[]mmodel.Alias}
//	@Failure		400						{object}	pkg.HTTPError
//	@Failure		404						{object}	pkg.HTTPError
//	@Failure		500						{object}	pkg.HTTPError
//	@Router			/v1/aliases [get]
func (handler *AliasHandler) GetAllAliases(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_aliases")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate query parameters, Error: %s", err.Error()))

		return http.WithError(c, err)
	}

	var holderID uuid.UUID
	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		holderID, err = uuid.Parse(*headerParams.HolderID)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to parse holder ID", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to parse holder ID, Error: %s", err.Error()))

			return http.WithError(c, err)
		}
	}

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
	}

	organizationID := c.Get("X-Organization-Id")
	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		span.SetAttributes(
			attribute.String("app.request.holder_id", holderID.String()),
		)
	}

	recordSafeQueryAttributes(span, headerParams)

	aliases, err := handler.Service.GetAllAliases(ctx, organizationID, holderID, *headerParams, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get all aliases", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get all aliases, Error: %v", err.Error()))

		return http.WithError(c, err)
	}

	pagination.SetItems(aliases)

	return http.OK(c, pagination)
}

// ResolveBankAccount resolves an active alias by tenant-wide bank-account identity.
//
//	@Summary		Resolve Alias by Bank Account
//	@Description	Resolves an active alias across the current tenant by holder document and exact bank-account identity. Does not require X-Organization-Id.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							false	"The authorization token in the 'Bearer access_token' format. Only required when auth plugin is enabled."
//	@Param			resolver		body		mmodel.ResolveBankAccountInput	true	"Bank Account Resolver Input"
//	@Success		200				{object}	mmodel.ResolveAliasResponse
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		404				{object}	pkg.HTTPError
//	@Failure		409				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/aliases/resolve-bank-account [post]
func (handler *AliasHandler) ResolveBankAccount(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.resolve_bank_account")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	payload, ok := p.(*mmodel.ResolveBankAccountInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	result, err := handler.Service.ResolveBankAccount(ctx, payload)
	if err != nil {
		handleAliasResolverHandlerError(ctx, span, logger, "Failed to resolve bank account", err)

		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// ResolveAccount resolves an active alias by tenant-wide ledger account ID.
//
//	@Summary		Resolve Alias by Account ID
//	@Description	Resolves an active alias across the current tenant by account ID. Does not require X-Organization-Id.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					false	"The authorization token in the 'Bearer access_token' format. Only required when auth plugin is enabled."
//	@Param			resolver		body		mmodel.ResolveAccountInput	true	"Account Resolver Input"
//	@Success		200				{object}	mmodel.ResolveAliasResponse
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		404				{object}	pkg.HTTPError
//	@Failure		409				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/aliases/resolve-account [post]
func (handler *AliasHandler) ResolveAccount(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.resolve_account")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	payload, ok := p.(*mmodel.ResolveAccountInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	result, err := handler.Service.ResolveAccount(ctx, payload)
	if err != nil {
		handleAliasResolverHandlerError(ctx, span, logger, "Failed to resolve account", err)

		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// BackfillBankAccountIndex repairs the tenant-wide alias bank-account resolver index.
//
//	@Summary		Backfill Alias Bank Account Resolver Index
//	@Description	Scans tenant alias collections and rebuilds alias_bank_account_index rows. Report contains counts and alias IDs only; no document, account, or bank identity values.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							false	"The authorization token in the 'Bearer access_token' format. Only required when auth plugin is enabled."
//	@Param			backfill		body		mmodel.BackfillBankAccountIndexInput	true	"Backfill Input"
//	@Success		200				{object}	mmodel.BankAccountIndexBackfillReport
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/aliases/backfill-bank-account-index [post]
func (handler *AliasHandler) BackfillBankAccountIndex(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.backfill_bank_account_index")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	payload, ok := p.(*mmodel.BackfillBankAccountIndexInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	result, err := handler.Service.BackfillBankAccountIndex(ctx, payload.DryRun)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to backfill bank account index", err)
		logger.Log(ctx, libLog.LevelError, "Failed to backfill bank account index", libLog.Err(err))

		return http.WithError(c, err)
	}

	return http.OK(c, result)
}

// DeleteRelatedParty removes a related party from an alias
//
//	@Summary		Delete a Related Party
//	@Description	Delete a Related Party from an Alias. This operation performs a physical deletion (hard delete) of the related party.
//	@Tags			Aliases
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path	string	true	"The unique identifier of the Holder."
//	@Param			alias_id			path	string	true	"The unique identifier of the Alias account."
//	@Param			related_party_id	path	string	true	"The unique identifier of the Related Party."
//	@Success		204
//	@Failure		400	{object}	pkg.HTTPError
//	@Failure		404	{object}	pkg.HTTPError
//	@Failure		500	{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases/{alias_id}/related-parties/{related_party_id} [delete]
func (handler *AliasHandler) DeleteRelatedParty(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_related_party")
	defer span.End()

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	aliasID, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	relatedPartyID, err := http.GetUUIDFromLocals(c, "related_party_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating removal of related party with ID: %s from alias: %s", relatedPartyID.String(), aliasID.String()))

	err = handler.Service.DeleteRelatedPartyByID(ctx, organizationID, holderID, aliasID, relatedPartyID)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to delete related party", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete related party with ID: %s, Error: %s", relatedPartyID.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

func handleAliasResolverHandlerError(ctx context.Context, span trace.Span, logger libLog.Logger, message string, err error) {
	if isBusinessError(err) {
		libOpenTelemetry.HandleSpanBusinessErrorEvent(span, message, err)
		logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))

		return
	}

	libOpenTelemetry.HandleSpanError(span, message, err)
	logger.Log(ctx, libLog.LevelError, message, libLog.Err(err))
}

func isBusinessError(err error) bool {
	var validationErr pkg.ValidationError
	if errors.As(err, &validationErr) {
		return true
	}

	var knownValidationErr pkg.ValidationKnownFieldsError
	if errors.As(err, &knownValidationErr) {
		return true
	}

	var unknownValidationErr pkg.ValidationUnknownFieldsError
	if errors.As(err, &unknownValidationErr) {
		return true
	}

	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return true
	}

	var conflictErr pkg.EntityConflictError

	return errors.As(err, &conflictErr)
}
