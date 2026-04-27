// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CloseAlias marks an alias closed by setting banking_details.closing_date
// (and deleted_at) to "now". The operation is:
//
//   - Naturally idempotent at the domain level: a second close on an already
//     closed alias is a no-op that returns the existing record. The repository
//     short-circuits before the UpdateOne.
//   - Protected by the idempotency guard when Idempotency-Key is supplied, so
//     the saga's compensation path (Phase 4) can retry safely without fear of
//     double-writes even if the first call's response was lost in transit.
//
// Lookup is by (holder_id, alias_id). A missing alias yields ErrAliasNotFound.
func (uc *UseCase) CloseAlias(ctx context.Context, organizationID string, holderID, aliasID uuid.UUID, idempotencyKey string) (*mmodel.Alias, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.close_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.Bool("app.request.has_idempotency_key", idempotencyKey != ""),
	)

	// The hash binds the idempotency key to the specific (holder, alias) pair
	// so that reusing the same key for a different close target is surfaced as
	// a hash mismatch rather than silently returning the unrelated record.
	requestHash, err := utils.CanonicalHashJSON(struct {
		Op       string `json:"op"`
		HolderID string `json:"holder_id"`
		AliasID  string `json:"alias_id"`
	}{
		Op:       "close-alias",
		HolderID: holderID.String(),
		AliasID:  aliasID.String(),
	})
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to canonicalize close-alias payload", err)
		logger.Log(ctx, libLog.LevelError, "Failed to canonicalize close-alias payload", libLog.Err(err))

		return nil, fmt.Errorf("close alias: canonical hash: %w", err)
	}

	return ExecuteIdempotent(ctx, uc.IdempotencyRepo, idempotencyKey, requestHash, 0,
		func(ctx context.Context) (*mmodel.Alias, error) {
			return uc.closeAliasCore(ctx, organizationID, holderID, aliasID)
		},
	)
}

func (uc *UseCase) closeAliasCore(ctx context.Context, organizationID string, holderID, aliasID uuid.UUID) (*mmodel.Alias, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.close_alias.core")
	defer span.End()

	closingAt := time.Now().UTC()

	closed, err := uc.AliasRepo.CloseByID(ctx, organizationID, holderID, aliasID, closingAt)
	if err != nil {
		// Business not-found is Warn (caller's problem), infra errors are
		// Error; the repository returns ValidateBusinessError for not-found
		// which is cleanly distinguishable at log-analysis time from infra
		// failures because it carries a stable error code (CRM-0008).
		libOpenTelemetry.HandleSpanError(span, "Failed to close alias", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to close alias", libLog.Err(err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Alias closed",
		libLog.String("alias_id", aliasID.String()),
		libLog.String("holder_id", holderID.String()))

	return closed, nil
}
