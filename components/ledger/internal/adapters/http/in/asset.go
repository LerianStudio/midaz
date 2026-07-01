// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// AssetHandler struct contains a cqrs use case for managing asset in related operations.
type AssetHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createAsset/updateAsset/... methods below own the span, imperative body
// decode+validation, the service call and the success log. They take primitive
// args (parsed UUIDs, raw body bytes, the query map) so BOTH transports feed them:
// the Fiber wrappers pull those from *fiber.Ctx (Locals + c.Body + c.Queries) and
// the Huma handlers (asset_handler_huma.go) pull them from the request envelope.
// Every canonical Midaz error the cores return is rendered by the caller —
// http.WithError on the Fiber path, http.HumaProblem on the Huma path — so the
// code + HTTP status are identical across both transports.

// createAsset owns the span + service call + success log for an already-decoded
// payload. Body decode+validation happens BEFORE this core: the Fiber path decodes
// via the WithBody decorator (passing the struct as `a`), the Huma path decodes via
// http.DecodeAndValidate(RawBody). Both feed the SAME validated *CreateAssetInput
// here, so create is identical across transports.
func (handler *AssetHandler) createAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAssetInput, token string) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create an asset", payload)
	recordSafePayloadAttributes(span, payload)

	asset, err := handler.Command.CreateAsset(ctx, organizationID, ledgerID, payload, token)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Asset on command", err)

		return nil, err
	}

	return asset, nil
}

// getAllAssets binds the query map imperatively (http.ValidateParameters — the
// SAME binder the Fiber path used) so a bad query yields the canonical 400, then
// returns the assembled pagination envelope.
func (handler *AssetHandler) getAllAssets(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_assets")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

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
		assets, err := handler.Query.GetAllMetadataAssets(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Assets on query", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(assets)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	assets, err := handler.Query.GetAllAssets(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all Assets on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(assets)

	return pagination, nil
}

// getAssetByID retrieves a single asset.
func (handler *AssetHandler) getAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_by_id")
	defer span.End()

	asset, err := handler.Query.GetAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Asset on query", err)

		return nil, err
	}

	return asset, nil
}

// updateAsset owns the span + service call + success log for an already-decoded
// payload (see createAsset for the decode split across transports).
func (handler *AssetHandler) updateAsset(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateAssetInput) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_asset")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update asset", payload)
	recordSafePayloadAttributes(span, payload)

	asset, err := handler.Command.UpdateAssetByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update Asset on command", err)

		return nil, err
	}

	return asset, nil
}

// deleteAsset removes an asset.
func (handler *AssetHandler) deleteAsset(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_asset_by_id")
	defer span.End()

	if err := handler.Command.DeleteAssetByID(ctx, organizationID, ledgerID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to remove Asset on command", err)

		return err
	}

	return nil
}

// countAssets returns the total asset count for the ledger.
func (handler *AssetHandler) countAssets(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_assets")
	defer span.End()

	count, err := handler.Query.CountAssets(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count assets", err)

		return 0, err
	}

	return count, nil
}

// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the
// handler methods directly; each pulls the transport inputs from *fiber.Ctx
// (Locals set by ParseUUIDPathParameters, the WithBody-decoded payload as `a`) and
// delegates to the shared core. The swaggo doc-comments below are preserved
// verbatim (the migration is ADDITIVE; swaggo is unchanged) so the generated api/
// spec keeps its per-op security. NOTE: the LIVE asset routes are Huma now (see
// asset_handler_huma.go + RegisterAssetRoutesToApp); these Fiber wrappers are not
// mounted by the unified server.

// CreateAsset is a method that creates asset information.
func (handler *AssetHandler) CreateAsset(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	asset, err := handler.createAsset(ctx, organizationID, ledgerID, a.(*mmodel.CreateAssetInput), c.Get("Authorization"))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, asset)
}

// GetAllAssets is a method that retrieves all Assets.
func (handler *AssetHandler) GetAllAssets(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllAssets(ctx, organizationID, ledgerID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// GetAssetByID is a method that retrieves Asset information by a given id.
func (handler *AssetHandler) GetAssetByID(c *fiber.Ctx) error {
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

	asset, err := handler.getAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, asset)
}

// UpdateAsset is a method that updates Asset information.
func (handler *AssetHandler) UpdateAsset(a any, c *fiber.Ctx) error {
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

	asset, err := handler.updateAsset(ctx, organizationID, ledgerID, id, a.(*mmodel.UpdateAssetInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, asset)
}

// DeleteAssetByID is a method that removes Asset information by a given ids.
func (handler *AssetHandler) DeleteAssetByID(c *fiber.Ctx) error {
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

	if err := handler.deleteAsset(ctx, organizationID, ledgerID, id); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// CountAssets is a method that returns the total count of assets for a specific ledger in an organization.
func (handler *AssetHandler) CountAssets(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	count, err := handler.countAssets(ctx, organizationID, ledgerID)
	if err != nil {
		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
