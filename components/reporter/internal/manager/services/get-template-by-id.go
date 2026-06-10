// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/template"

	"github.com/LerianStudio/lib-observability/log"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetTemplateByID recover a package by ID
func (uc *UseCase) GetTemplateByID(ctx context.Context, id uuid.UUID) (_ *template.Template, err error) {
	start := time.Now()

	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.template.get_by_id")
	defer span.End()
	defer func() { uc.recordDomainOp(ctx, opGetTemplate, start, err) }()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", id.String()),
	)
	uc.Logger.Log(ctx, log.LevelDebug, "Retrieving template", log.String("id", id.String()))

	templateModel, err := uc.TemplateRepo.FindByID(ctx, id)
	if err != nil {
		if nf := (pkg.EntityNotFoundError{}); errors.As(err, &nf) {
			errNotFound := pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionTemplate)

			opentelemetry.HandleSpanBusinessErrorEvent(span, "Template not found", errNotFound)

			return nil, errNotFound
		}

		opentelemetry.HandleSpanError(span, "Failed to get template on repo by id", err)

		return nil, err
	}

	return templateModel, nil
}
