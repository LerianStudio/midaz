// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb/template"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/net/http"

	"github.com/LerianStudio/lib-observability/log"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllTemplates fetch all Templates from the repository
func (uc *UseCase) GetAllTemplates(ctx context.Context, filters http.QueryHeader) ([]*template.Template, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.template.get_all")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.Int("app.request.page", filters.Page),
		attribute.Int("app.request.limit", filters.Limit),
		attribute.Bool("app.request.has_metadata", filters.Metadata != nil),
	)

	uc.Logger.Log(ctx, log.LevelInfo, "Retrieving templates",
		log.Int("page", filters.Page),
		log.Int("limit", filters.Limit),
		log.Bool("has_metadata", filters.Metadata != nil),
	)

	templates, errFind := uc.TemplateRepo.FindList(ctx, filters)
	if errFind != nil {
		opentelemetry.HandleSpanError(span, "Failed to get all templates on repo", errFind)

		return nil, errFind
	}

	return templates, nil
}
