// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	libObs "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DeleteAliasByID removes an alias by its ID and holder ID.
func (uc *UseCase) DeleteAliasByID(ctx context.Context, organizationID string, holderID, id uuid.UUID, hardDelete bool) error {
	logger, tracer, reqId, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_alias_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Delete alias by id %v", id))

	err := uc.AliasRepo.Delete(ctx, organizationID, holderID, id, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to delete alias by id: %v", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete alias by id: %v", err))

		return err
	}

	deletedAt := time.Now().UTC()

	uc.emitAliasDeletedEvent(ctx, span, logger, id.String(), holderID.String(), organizationID, hardDelete, deletedAt)

	return nil
}

func (uc *UseCase) emitAliasDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, holderID, organizationID string, hardDelete bool, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AliasDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAliasDeleted(id, holderID, organizationID, hardDelete, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
