// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"os"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// OrganizationHandler struct contains an organization use case for managing organization related operations.
type OrganizationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createOrganization/updateOrganization/... methods below own the span, the
// service call and every organization-specific guard (the list status + name-filter
// checks and the delete production-environment guard). They
// take primitive args — the parsed UUID, the decoded payload, the query map — so
// nothing transport-shaped reaches them; the handlers in organization_handler.go pull
// those out of the request envelope. Every canonical Midaz error a core returns is
// rendered by its caller via http.HumaProblem, which fixes the code + HTTP status.

// createOrganization owns the span + service call for an already-decoded
// payload. Body decode+validation happens BEFORE this core, in the handler, via
// http.DecodeAndValidate.
//
// holderPolicy is the caller's route-version holder contract, carried explicitly from
// the transport shell down to the self-holder provisioning in the use case: the /v1
// shell passes command.HolderOffV1, the /v2 shell command.HolderOnV2.
func (handler *OrganizationHandler) createOrganization(ctx context.Context, payload *mmodel.CreateOrganizationInput, holderPolicy command.RouteHolderPolicy) (*mmodel.Organization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_organization")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	organization, err := handler.Command.CreateOrganization(ctx, payload, holderPolicy)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create organization on command", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create organization", libLog.Err(err))

		return nil, err
	}

	span.SetAttributes(attribute.String("app.organization.id", organization.ID))

	return organization, nil
}

// updateOrganization owns the span + service call for an already-decoded
// payload (see createOrganization for where the decode happens).
func (handler *OrganizationHandler) updateOrganization(ctx context.Context, id uuid.UUID, payload *mmodel.UpdateOrganizationInput) (*mmodel.Organization, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_organization")
	defer span.End()

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

// getAllOrganizations binds the query map imperatively via http.ValidateParameters,
// applies the organization-specific status + name-filter guards, then returns the
// assembled pagination envelope. A bad query or a rejected guard yields the canonical
// 400.
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
// guard: DELETE is rejected with the canonical ErrActionNotPermitted (422) when
// ENV_NAME == "production".
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
