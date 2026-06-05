// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// EncryptionHandler handles HTTP requests for encryption provisioning operations.
type EncryptionHandler struct {
	ProvisioningService encryption.ProvisioningService
}

// Provision handles the provisioning of an organization for envelope encryption.
//
//	@Summary		Provision an Organization for Envelope Encryption
//	@Description	Provisions an organization for envelope encryption by generating keysets and registering the organization.
//	@Tags			Encryption
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string							false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string							true	"The unique identifier of the Organization."
//	@Param			input				body		mmodel.ProvisionEncryptionInput	true	"Provision Input"
//	@Success		201					{object}	mmodel.ProvisionEncryptionResponse
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		409					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/organizations/{organization_id}/encryption/provision [post]
func (handler *EncryptionHandler) Provision(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.provision_encryption")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload, ok := p.(*mmodel.ProvisionEncryptionInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	if err := payload.Validate(); err != nil {
		libOpenTelemetry.HandleSpanError(span, "Validation failed", err)

		logger.Log(ctx, libLog.LevelWarn, "Validation failed", libLog.Err(err))

		return http.WithError(c, err)
	}

	// Get tenant ID from context (defaults to "default" for single-tenant mode)
	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		tenantID = "default"
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

		return http.WithError(c, err)
	}

	response := mmodel.ProvisionEncryptionResponse{
		OrganizationID:   result.OrganizationID,
		KEKPath:          result.KEKPath,
		AEADPrimaryKeyID: result.AEADPrimaryKeyID,
		MACPrimaryKeyID:  result.MACPrimaryKeyID,
		Status:           string(result.RegistryStatus),
	}

	return http.Created(c, response)
}

// GetProvisioningStatus handles the retrieval of an organization's provisioning status.
//
//	@Summary		Get Provisioning Status
//	@Description	Retrieves the current provisioning status for an organization's envelope encryption.
//	@Tags			Encryption
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string	true	"The unique identifier of the Organization."
//	@Success		200					{object}	mmodel.ProvisioningStatusResponse
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/organizations/{organization_id}/encryption/status [get]
func (handler *EncryptionHandler) GetProvisioningStatus(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_provisioning_status")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	status, err := handler.ProvisioningService.GetProvisioningStatus(ctx, organizationID.String())
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get provisioning status", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get provisioning status", libLog.Err(err))

		return http.WithError(c, err)
	}

	response := mmodel.ProvisioningStatusResponse{
		OrganizationID: organizationID.String(),
		Provisioned:    status != nil,
	}

	if status != nil {
		response.Status = string(*status)
	}

	return http.OK(c, response)
}
