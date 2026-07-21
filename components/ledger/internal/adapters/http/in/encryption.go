// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// EncryptionHandler handles HTTP requests for encryption provisioning operations.
type EncryptionHandler struct {
	ProvisioningService encryption.ProvisioningService
}

// Provision handles the provisioning of an organization for envelope encryption.
func (handler *EncryptionHandler) Provision(p any, c *fiber.Ctx) error {
	payload, ok := p.(*mmodel.ProvisionEncryptionInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	response, err := handler.provision(c.UserContext(), organizationID, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, response)
}

// provision is the transport-agnostic core for the encryption provisioning
// operation. Both the Fiber wrapper (Provision) and the Huma shell
// (ProvisionHuma) delegate here after resolving the org id and decoding the
// payload, so neither touches the other's request/response object.
//
// The tenant id is resolved from the context: the Fiber tenant
// PostAuthMiddlewares run BEFORE this core on both transports (the Huma terminal
// sits behind the same middleware chain), so ctx already carries the tenant id
// where one applies. In single-tenant mode the middleware does not run, the
// context carries no tenant id (empty), and the helper substitutes the reserved
// "default" flat-base sentinel. In multi-tenant mode the middleware always
// populates a non-empty tenant id from the JWT; a real tenant literally named
// "default" would collide with the single-tenant sentinel and resolve to the
// flat base mount, breaking tenant isolation, so the helper rejects it with
// ErrReservedTenantID. The lazy autoProvision path uses the same helper, so both
// ingress points share one source of truth.
func (handler *EncryptionHandler) provision(ctx context.Context, organizationID uuid.UUID, payload *mmodel.ProvisionEncryptionInput) (*mmodel.ProvisionEncryptionResponse, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.provision_encryption")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	if err := payload.Validate(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Validation failed", err)

		logger.Log(ctx, libLog.LevelWarn, "Validation failed", libLog.Err(err))

		return nil, err
	}

	tenantID, err := encryption.ResolveProvisionTenantID(ctx)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "reserved tenant id supplied", err)

		logger.Log(ctx, libLog.LevelWarn, "reserved tenant id supplied")

		return nil, err
	}

	input := encryption.ProvisionInput{
		TenantID:       tenantID,
		OrganizationID: organizationID.String(),
		Actor:          payload.Actor,
		Reason:         payload.Reason,
	}

	result, err := handler.ProvisioningService.Provision(ctx, input)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to provision encryption", err)

		logger.Log(ctx, libLog.LevelError, "Failed to provision encryption", libLog.Err(err))

		return nil, err
	}

	return &mmodel.ProvisionEncryptionResponse{
		OrganizationID:   result.OrganizationID,
		KEKPath:          result.KEKPath,
		AEADPrimaryKeyID: result.AEADPrimaryKeyID,
		PRFPrimaryKeyID:  result.PRFPrimaryKeyID,
		Status:           string(result.RegistryStatus),
	}, nil
}

// GetProvisioningStatus handles the retrieval of an organization's provisioning status.
func (handler *EncryptionHandler) GetProvisioningStatus(c *fiber.Ctx) error {
	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	response, err := handler.getProvisioningStatus(c.UserContext(), organizationID)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, response)
}

// getProvisioningStatus is the transport-agnostic core for the provisioning
// status read. Both the Fiber wrapper (GetProvisioningStatus) and the Huma shell
// (GetProvisioningStatusHuma) delegate here after resolving the org id.
func (handler *EncryptionHandler) getProvisioningStatus(ctx context.Context, organizationID uuid.UUID) (*mmodel.ProvisioningStatusResponse, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_provisioning_status")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	status, err := handler.ProvisioningService.GetProvisioningStatus(ctx, organizationID.String())
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get provisioning status", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get provisioning status", libLog.Err(err))

		return nil, err
	}

	response := &mmodel.ProvisioningStatusResponse{
		OrganizationID: organizationID.String(),
		Provisioned:    status != nil,
	}

	if status != nil {
		response.Status = string(*status)
	}

	return response, nil
}
