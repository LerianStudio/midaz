// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/ctxutil"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/reporter/net/http"

	"github.com/LerianStudio/lib-observability/log"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteTemplateByID delete a template from the repository and cascades the deletion
// to all deadlines linked to that template, so they are no longer deliverable.
func (uc *UseCase) DeleteTemplateByID(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.template.delete")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", id.String()),
	)
	uc.Logger.Log(ctx, log.LevelInfo, "Remove template", log.String("id", id.String()))

	// Cascade-delete linked deadlines BEFORE removing the template so that a
	// failure here leaves the system in a consistent state: the template still
	// exists and a client retry can safely repeat the full operation.
	// If we deleted the template first and then the cascade failed, the orphan
	// deadlines would become permanent because retries would be blocked by a
	// template-not-found error.
	if uc.DeadlineRepo != nil {
		deleted, err := uc.DeadlineRepo.DeleteByTemplateID(ctx, id)
		if err != nil {
			opentelemetry.HandleSpanError(span, "Failed to cascade delete deadlines by template_id", err)
			uc.Logger.Log(ctx, log.LevelError, "Failed to cascade delete deadlines for template",
				log.String("template_id", id.String()), log.Err(err))

			return err
		}

		uc.Logger.Log(ctx, log.LevelInfo, "Cascade deleted deadlines for template",
			log.String("template_id", id.String()),
			log.Any("deadlines_deleted", deleted))
	}

	if err := uc.TemplateRepo.Delete(ctx, id, hardDelete); err != nil {
		if pkgHTTP.IsBusinessError(err) {
			opentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete template on repo by id", err)
		} else {
			opentelemetry.HandleSpanError(span, "Failed to delete template on repo by id", err)
		}

		uc.Logger.Log(ctx, log.LevelError, "Error deleting template on repo by id", log.Err(err))

		return err
	}

	return nil
}
