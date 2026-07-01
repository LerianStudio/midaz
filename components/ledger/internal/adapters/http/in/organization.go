// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"os"

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

// OrganizationHandler struct contains an organization use case for managing organization related operations.
type OrganizationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createOrganization/updateOrganization/... methods below own the span, the
// service call, the success log and every organization-specific guard (the list
// status + name-filter checks and the delete production-environment guard). They
// take primitive args (parsed UUIDs, raw body payload, the query map) so BOTH
// transports feed them: the Fiber wrappers pull those from *fiber.Ctx (Locals +
// the WithBody-decoded payload) and the Huma handlers (organization_handler_huma.go)
// pull them from the request envelope. Every canonical Midaz error the cores return
// is rendered by the caller — http.WithError on the Fiber path, http.HumaProblem on
// the Huma path — so the code + HTTP status are identical across both transports.

// createOrganization owns the span + service call + success log for an already-decoded
// payload. Body decode+validation happens BEFORE this core (Fiber via WithBody, Huma
// via http.DecodeAndValidate), so create is identical across transports.
func (handler *OrganizationHandler) createOrganization(ctx context.Context, payload *mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_organization")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create an organization", payload)
	recordSafePayloadAttributes(span, payload)

	organization, err := handler.Command.CreateOrganization(ctx, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create organization on command", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create organization", libLog.Err(err))

		return nil, err
	}

	span.SetAttributes(attribute.String("app.organization.id", organization.ID))

	return organization, nil
}

// updateOrganization owns the span + service call + success log for an already-decoded
// payload (see createOrganization for the decode split across transports).
func (handler *OrganizationHandler) updateOrganization(ctx context.Context, id uuid.UUID, payload *mmodel.UpdateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_organization")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update an organization", payload)
	recordSafePayloadAttributes(span, payload)

	organization, err := handler.Command.UpdateOrganizationByID(ctx, id, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update organization on command", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update organization", libLog.Err(err))

		return nil, err
	}

	return organization, nil
}

// getOrganizationByID retrieves a single organization.
func (handler *OrganizationHandler) getOrganizationByID(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_organization_by_id")
	defer span.End()

	organization, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve organization on query", err)

		return nil, err
	}

	return organization, nil
}

// getAllOrganizations binds the query map imperatively (http.ValidateParameters — the
// SAME binder the Fiber path used), applies the organization-specific status +
// name-filter guards, then returns the assembled pagination envelope. A bad query or
// a rejected guard yields the canonical 400.
func (handler *OrganizationHandler) getAllOrganizations(ctx context.Context, queries map[string]string) (http.Pagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_organizations")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	if headerParams.Status != nil && !isValidStatus(*headerParams.Status, organizationAllowedStatuses) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityOrganization, "status")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: invalid organization status", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate organization status query parameter", libLog.String("status", *headerParams.Status), libLog.Err(err))

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

	if headerParams.Metadata != nil {
		if headerParams.HasNameFilters() {
			err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityOrganization, "metadata cannot be combined with name filters (legal_name, doing_business_as)")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: metadata and name filters are mutually exclusive", err)

			return http.Pagination{}, err
		}

		organizations, err := handler.Query.GetAllMetadataOrganizations(ctx, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all organizations by metadata", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(organizations)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	organizations, err := handler.Query.GetAllOrganizations(ctx, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all organizations", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(organizations)

	return pagination, nil
}

// deleteOrganization removes an organization. It owns the production-environment
// guard: DELETE is rejected with the canonical ErrActionNotPermitted (403) when
// ENV_NAME == "production", identical across transports.
func (handler *OrganizationHandler) deleteOrganization(ctx context.Context, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_organization_by_id")
	defer span.End()

	if os.Getenv("ENV_NAME") == "production" {
		err := pkg.ValidateBusinessError(constant.ErrActionNotPermitted, constant.EntityOrganization)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to remove organization in production environment", err)

		return err
	}

	if err := handler.Command.DeleteOrganizationByID(ctx, id); err != nil {
		handleSpanByErrorClass(span, "Failed to remove organization on command", err)

		return err
	}

	return nil
}

// countOrganizations returns the total organization count.
func (handler *OrganizationHandler) countOrganizations(ctx context.Context) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_organizations")
	defer span.End()

	count, err := handler.Query.CountOrganizations(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count organizations", err)

		return 0, err
	}

	return count, nil
}

// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the handler
// methods directly; each pulls the transport inputs from *fiber.Ctx (Locals set by
// ParseUUIDPathParameters, the WithBody-decoded payload) and delegates to the shared
// core. The swaggo doc-comments below are preserved verbatim (the migration is
// ADDITIVE). NOTE: the LIVE organization routes are Huma now (see
// organization_handler_huma.go + RegisterOrganizationRoutesToApp); these Fiber
// wrappers are not mounted by the unified server.

// CreateOrganization is a method that creates Organization information.
func (handler *OrganizationHandler) CreateOrganization(p any, c *fiber.Ctx) error {
	organization, err := handler.createOrganization(c.UserContext(), p.(*mmodel.CreateOrganizationInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, organization)
}

// UpdateOrganization is a method that updates Organization information.
func (handler *OrganizationHandler) UpdateOrganization(p any, c *fiber.Ctx) error {
	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organization, err := handler.updateOrganization(c.UserContext(), id, p.(*mmodel.UpdateOrganizationInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, organization)
}

// GetOrganizationByID is a method that retrieves Organization information by a given id.
func (handler *OrganizationHandler) GetOrganizationByID(c *fiber.Ctx) error {
	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organization, err := handler.getOrganizationByID(c.UserContext(), id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, organization)
}

// GetAllOrganizations is a method that retrieves all Organizations.
func (handler *OrganizationHandler) GetAllOrganizations(c *fiber.Ctx) error {
	pagination, err := handler.getAllOrganizations(c.UserContext(), c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// DeleteOrganizationByID is a method that removes Organization information by a given id.
func (handler *OrganizationHandler) DeleteOrganizationByID(c *fiber.Ctx) error {
	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	if err := handler.deleteOrganization(c.UserContext(), id); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// CountOrganizations is a method that returns the total count of organizations.
func (handler *OrganizationHandler) CountOrganizations(c *fiber.Ctx) error {
	count, err := handler.countOrganizations(c.UserContext())
	if err != nil {
		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
