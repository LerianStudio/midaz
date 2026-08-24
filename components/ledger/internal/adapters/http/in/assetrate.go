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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// AssetRateHandler struct contains a cqrs use case for managing asset rate.
type AssetRateHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createOrUpdateAssetRate/getAssetRateByExternalID/getAllAssetRatesByAssetCode
// methods below own the span, the service call and the success log. They take
// primitive args — parsed UUIDs, the raw asset-code string, the decoded payload, the
// query map — so nothing transport-shaped reaches them; the handlers in
// assetrate_handler.go pull those out of the request envelope. Every canonical Midaz
// error a core returns is rendered by its caller via http.HumaProblem, which fixes
// the code + HTTP status. assetrate is MONEY-adjacent (exchange rates).

// createOrUpdateAssetRate owns the span + service call + success log for an
// already-decoded payload. Body decode+validation happens BEFORE this core, in the
// handler, via http.DecodeAndValidate(RawBody).
func (handler *AssetRateHandler) createOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *assetrate.CreateAssetRateInput) (*assetrate.AssetRate, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset_rate")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create an AssetRate", payload)
	recordSafePayloadAttributes(span, payload)

	assetRate, err := handler.Command.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create AssetRate on command", err)

		return nil, err
	}

	return assetRate, nil
}

// getAssetRateByExternalID retrieves a single asset rate by its external id.
func (handler *AssetRateHandler) getAssetRateByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*assetrate.AssetRate, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_external_id")
	defer span.End()

	assetRate, err := handler.Query.GetAssetRateByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to get AssetRate on query", err)

		return nil, err
	}

	return assetRate, nil
}

// getAllAssetRatesByAssetCode binds the query map imperatively via
// http.ValidateParameters so a bad query yields the canonical 400, then returns the
// cursor-paginated envelope. assetCode is a free-form string path segment (NOT a
// UUID), so it is passed through verbatim.
func (handler *AssetRateHandler) getAllAssetRatesByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, assetCode string, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_asset_code")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	headerParams.Metadata = &bson.M{}

	assetRates, cur, err := handler.Query.GetAllAssetRatesByAssetCode(ctx, organizationID, ledgerID, assetCode, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to get AssetRate on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(assetRates)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}
