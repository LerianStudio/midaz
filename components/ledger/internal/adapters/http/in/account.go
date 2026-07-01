// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// AccountHandler struct contains an account use case for managing account related operations.
type AccountHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createAccount/updateAccount/... cores below own the span, the imperative
// query binding, the service call(s) and the success log/metric. They take
// primitive args (parsed UUIDs, the already-decoded payload, the query map) so
// BOTH transports feed them: the Fiber wrappers pull those from *fiber.Ctx
// (Locals + WithBody-decoded payload + c.Queries) and the Huma handlers
// (account_handler_huma.go) pull them from the request envelope. Every canonical
// Midaz error the cores return is rendered by the caller — http.WithError on the
// Fiber path, http.HumaProblem on the Huma path — so the code + HTTP status are
// identical across both transports.

// createAccount owns the span + service call + success log + created metric for an
// already-decoded payload. Body decode+validation happens BEFORE this core: the
// Fiber path decodes via the WithBody decorator (passing the struct as `i`), the
// Huma path decodes via http.DecodeAndValidate(RawBody). Both feed the SAME
// validated *CreateAccountInput here. The RecordAccountCreated metric lives here
// (not in a transport wrapper) so both transports emit it identically.
func (handler *AccountHandler) createAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountInput, token string) (*mmodel.Account, error) {
	logger, tracer, _, metricFactory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create an account", payload)
	recordSafePayloadAttributes(span, payload)

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, payload, token)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Account on command", err)

		return nil, err
	}

	if err := metricFactory.RecordAccountCreated(
		ctx,
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to record account created metric", err)
	}

	return account, nil
}

// getAllAccounts binds the query map imperatively (http.ValidateParameters — the
// SAME binder the Fiber path used), validates the account-specific status enum,
// resolves the optional portfolio_id/segment_id UUID filters, then branches on
// metadata exactly as the pre-Huma handler did. A bad query / status yields the
// canonical 400.
func (handler *AccountHandler) getAllAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
	defer span.End()

	var (
		portfolioID *uuid.UUID
		segmentID   *uuid.UUID
	)

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	if headerParams.Status != nil && !isValidStatus(*headerParams.Status, accountAllowedStatuses) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityAccount, "status")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: invalid account status", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate account status query parameter", libLog.String("status", *headerParams.Status), libLog.Err(err))

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if !libCommons.IsNilOrEmpty(&headerParams.PortfolioID) {
		parsedID := uuid.MustParse(headerParams.PortfolioID)
		portfolioID = &parsedID
	}

	if !libCommons.IsNilOrEmpty(&headerParams.SegmentID) {
		parsedID := uuid.MustParse(headerParams.SegmentID)
		segmentID = &parsedID
	}

	if headerParams.Metadata != nil {
		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, portfolioID, segmentID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Accounts on query", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(accounts)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, portfolioID, segmentID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all Accounts on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(accounts)

	return pagination, nil
}

// getAccountByID retrieves a single account by its UUID.
func (handler *AccountHandler) getAccountByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_by_id")
	defer span.End()

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return nil, err
	}

	return account, nil
}

// getAccountByAlias retrieves a single account by its alias. The external-by-code
// path resolves the alias (DefaultExternalAccountAliasPrefix + code) BEFORE this
// core, so both the alias and external-code ops share one implementation. The span
// name carries the caller so the two callers stay distinguishable in traces.
func (handler *AccountHandler) getAccountByAlias(ctx context.Context, spanName string, organizationID, ledgerID uuid.UUID, alias string) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	account, err := handler.Query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return nil, err
	}

	return account, nil
}

// updateAccount owns the span + update-then-get flow for an already-decoded
// payload (see createAccount for the decode split across transports). It updates,
// then re-reads so the caller receives the freshly persisted account.
func (handler *AccountHandler) updateAccount(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update account", payload)
	recordSafePayloadAttributes(span, payload)

	if _, err := handler.Command.UpdateAccount(ctx, organizationID, ledgerID, nil, id, payload); err != nil {
		handleSpanByErrorClass(span, "Failed to update Account on command", err)

		return nil, err
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return nil, err
	}

	return account, nil
}

// deleteAccount removes an account by its UUID.
func (handler *AccountHandler) deleteAccount(ctx context.Context, organizationID, ledgerID, id uuid.UUID, token string) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_by_id")
	defer span.End()

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, nil, id, token); err != nil {
		handleSpanByErrorClass(span, "Failed to remove Account on command", err)

		return err
	}

	return nil
}

// countAccounts returns the total account count for the ledger.
func (handler *AccountHandler) countAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_accounts")
	defer span.End()

	count, err := handler.Query.CountAccounts(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count accounts", err)

		return 0, err
	}

	return count, nil
}

// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the
// handler methods directly; each pulls the transport inputs from *fiber.Ctx
// (Locals set by ParseUUIDPathParameters, the WithBody-decoded payload as `i`) and
// delegates to the shared core. The swaggo doc-comments below are preserved
// verbatim (the migration is ADDITIVE; swaggo is unchanged) so the generated api/
// spec keeps its per-op security. NOTE: the LIVE account routes are Huma now (see
// account_handler_huma.go + RegisterAccountRoutesToApp); these Fiber wrappers are
// not mounted by the unified server.

// CreateAccount is a method that creates account information.
func (handler *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	account, err := handler.createAccount(ctx, organizationID, ledgerID, i.(*mmodel.CreateAccountInput), c.Get("Authorization"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, account)
}

// GetAllAccounts is a method that retrieves all Accounts.
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllAccounts(ctx, organizationID, ledgerID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// GetAccountByID is a method that retrieves Account information by a given account id.
func (handler *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	account, err := handler.getAccountByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// GetAccountExternalByCode is a method that retrieves External Account information by a given asset code.
func (handler *AccountHandler) GetAccountExternalByCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	alias := constant.DefaultExternalAccountAliasPrefix + c.Params("code")

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_external_by_code", organizationID, ledgerID, alias)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// GetAccountByAlias is a method that retrieves Account information by a given account alias.
func (handler *AccountHandler) GetAccountByAlias(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	account, err := handler.getAccountByAlias(ctx, "handler.get_account_by_alias", organizationID, ledgerID, c.Params("alias"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// UpdateAccount is a method that updates Account information.
func (handler *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	account, err := handler.updateAccount(ctx, organizationID, ledgerID, id, i.(*mmodel.UpdateAccountInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// DeleteAccountByID is a method that removes Account information by a given account id.
func (handler *AccountHandler) DeleteAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	if err := handler.deleteAccount(ctx, organizationID, ledgerID, id, c.Get("Authorization")); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// CountAccounts is a method that counts all accounts for a given organization and ledger, with an optional portfolio ID.
func (handler *AccountHandler) CountAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	count, err := handler.countAccounts(ctx, organizationID, ledgerID)
	if err != nil {
		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
