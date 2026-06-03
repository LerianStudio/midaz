// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	pkgHTTP "github.com/LerianStudio/midaz/v3/components/reporter/pkg/net/http"

	"github.com/LerianStudio/lib-observability/log"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteDeadlineByID performs a soft delete on a deadline by setting its deleted_at field.
func (uc *UseCase) DeleteDeadlineByID(ctx context.Context, id uuid.UUID) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.deadline.delete")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.deadline_id", id.String()),
	)
	uc.Logger.Log(ctx, log.LevelInfo, "Remove deadline", log.String("id", id.String()))

	if err := uc.DeadlineRepo.Delete(ctx, id); err != nil {
		if pkgHTTP.IsBusinessError(err) {
			opentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete deadline on repo by id", err)
		} else {
			opentelemetry.HandleSpanError(span, "Failed to delete deadline on repo by id", err)
		}

		uc.Logger.Log(ctx, log.LevelError, "Error deleting deadline on repo by id", log.Err(err))

		return err
	}

	return nil
}
