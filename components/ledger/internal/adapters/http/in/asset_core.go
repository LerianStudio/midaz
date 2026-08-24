// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// AssetHandler struct contains a cqrs use case for managing asset in related operations.
type AssetHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createAsset/updateAsset/... methods below own the span, imperative body
// decode+validation and the service call. They take primitive args — parsed
// UUIDs, raw body bytes, the query map — so nothing transport-shaped
// reaches them; the handlers in asset_handler.go pull those out of the request
// envelope. Every canonical Midaz error a core returns is rendered by its caller
// via http.HumaProblem, which fixes the code + HTTP status.

// createAsset owns the span + service call for an already-decoded
// payload. Body decode+validation happens BEFORE this core, in the handler, via
// http.DecodeAndValidate(RawBody).
func (handler *AssetHandler) createAsset(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAssetInput, token string) (*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	asset, err := handler.Command.CreateAsset(ctx, organizationID, ledgerID, payload, token)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Asset on command", err)

		return nil, err
	}

	return asset, nil
}

// getAllAssets binds the query map imperatively via http.ValidateParameters so a
// bad query yields the canonical 400, then returns the assembled pagination
// envelope.
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

// updateAsset owns the span + service call for an already-decoded
// payload (see createAsset for the decode split across transports).
func (handler *AssetHandler) updateAsset(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateAssetInput) (*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_asset")
	defer span.End()

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
