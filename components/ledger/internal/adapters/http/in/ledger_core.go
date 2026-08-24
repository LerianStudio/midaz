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

// LedgerHandler struct contains a ledger use case for managing ledger related operations.
type LedgerHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createLedger/updateLedger/... methods below own the span and the service
// call. They take primitive args — parsed UUIDs, the already-decoded payload, the
// query map — so nothing transport-shaped reaches them; the handlers in
// ledger_handler.go pull those out of the request envelope. Every canonical Midaz
// error a core returns is rendered by its caller via http.HumaProblem, which fixes
// the code + HTTP status; none of them is a native Huma 422.

// createLedger owns the span + service call for an already-decoded
// payload. Body decode+validation happens BEFORE this core (Fiber: WithBody
// decorator; Huma: http.DecodeAndValidate), so create is identical across transports.
func (handler *LedgerHandler) createLedger(ctx context.Context, organizationID uuid.UUID, payload *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_ledger")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	ledger, err := handler.Command.CreateLedger(ctx, organizationID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create ledger on command", err)

		return nil, err
	}

	return ledger, nil
}

// getLedgerByID retrieves a single ledger.
func (handler *LedgerHandler) getLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_ledger_by_id")
	defer span.End()

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve ledger on query", err)

		return nil, err
	}

	return ledger, nil
}

// getAllLedgers binds the query map imperatively via http.ValidateParameters,
// enforces the ledger-specific status allowlist and the metadata/name-filter mutual
// exclusion, then returns the assembled pagination envelope. Every rejection is a
// canonical 400, never a native Huma 422.
func (handler *LedgerHandler) getAllLedgers(ctx context.Context, organizationID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_ledgers")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	if headerParams.Status != nil && !isValidStatus(*headerParams.Status, ledgerAllowedStatuses) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLedger, "status")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: invalid ledger status", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate ledger status query parameter", libLog.String("status", *headerParams.Status), libLog.Err(err))

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
			err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLedger, "metadata cannot be combined with name filters (name)")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: metadata and name filters are mutually exclusive", err)

			return http.Pagination{}, err
		}

		ledgers, err := handler.Query.GetAllMetadataLedgers(ctx, organizationID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all ledgers by metadata", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(ledgers)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	ledgers, err := handler.Query.GetAllLedgers(ctx, organizationID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all ledgers on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(ledgers)

	return pagination, nil
}

// updateLedger owns the span + service call for an already-decoded
// payload (see createLedger for the decode split across transports).
func (handler *LedgerHandler) updateLedger(ctx context.Context, organizationID, id uuid.UUID, payload *mmodel.UpdateLedgerInput) (*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_ledger")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	ledger, err := handler.Command.UpdateLedgerByID(ctx, organizationID, id, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update ledger on command", err)

		return nil, err
	}

	return ledger, nil
}

// deleteLedger removes a ledger. The production-environment guard (ENV_NAME) is
// enforced HERE, in the core, so no caller can route around it: in production the
// delete is refused with the canonical 0008 / 403 forbidden.
func (handler *LedgerHandler) deleteLedger(ctx context.Context, organizationID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_ledger_by_id")
	defer span.End()

	if os.Getenv("ENV_NAME") == "production" {
		err := pkg.ValidateBusinessError(constant.ErrActionNotPermitted, constant.EntityLedger)

		handleSpanByErrorClass(span, "Failed to remove ledger on command", err)

		return err
	}

	if err := handler.Command.DeleteLedgerByID(ctx, organizationID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to remove ledger on command", err)

		return err
	}

	return nil
}

// countLedgers returns the total ledger count for the organization.
func (handler *LedgerHandler) countLedgers(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_ledgers")
	defer span.End()

	count, err := handler.Query.CountLedgers(ctx, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count ledgers", err)

		return 0, err
	}

	return count, nil
}

// getLedgerSettings returns the parsed settings for a ledger.
func (handler *LedgerHandler) getLedgerSettings(ctx context.Context, organizationID, id uuid.UUID) (mmodel.LedgerSettings, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_ledger_settings")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", id.String()),
	)

	ledgerSettings, err := handler.Query.GetParsedLedgerSettings(ctx, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get ledger settings", err)

		return mmodel.LedgerSettings{}, err
	}

	return ledgerSettings, nil
}

// updateLedgerSettings owns the span + the schema-aware merge-patch service call.
//
// LANDMINE: the settings body is a free-form map[string]any, NOT a validated
// struct — the allowlist merge-patch (unknown fields -> 0147, wrong types -> 0148)
// lives in Command.UpdateLedgerSettings (via mmodel.ValidateSettings). Those are
// canonical business errors classified to a 400, so the caller renders them via
// HumaProblem as 400 problem+json — never a native Huma 422. Body decode happens
// BEFORE this core (http.DecodeAndValidate into a map), which is where the
// null-byte/depth/key-count guards run.
func (handler *LedgerHandler) updateLedgerSettings(ctx context.Context, organizationID, id uuid.UUID, settings map[string]any) (mmodel.LedgerSettings, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_ledger_settings")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", id.String()),
	)

	updatedSettings, err := handler.Command.UpdateLedgerSettings(ctx, organizationID, id, settings)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update ledger settings", err)

		return mmodel.LedgerSettings{}, err
	}

	return mmodel.ParseLedgerSettings(updatedSettings), nil
}
