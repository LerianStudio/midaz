// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/deadline"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// CreateDeadline creates a new deadline entity after validating the input and persisting to the repository.
func (uc *UseCase) CreateDeadline(ctx context.Context, input *deadline.CreateDeadlineInput) (*deadline.Deadline, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.deadline.create")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.deadline_name", input.Name),
		attribute.String("app.request.deadline_type", input.Type),
	)
	uc.Logger.Log(ctx, log.LevelInfo, "Creating deadline",
		log.String("deadline_name", input.Name),
		log.String("deadline_type", input.Type),
	)

	// Build domain entity with invariant validation
	deadlineID, err := commons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate deadline ID", err)

		return nil, err
	}

	entity, err := deadline.NewDeadline(deadlineID, input.Name, input.Type, input.DueDate, input.Frequency, input.Color)
	if err != nil {
		businessErr := pkg.ValidateBusinessError(err, constant.MongoCollectionDeadline)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create deadline entity", businessErr)

		return nil, businessErr
	}

	// Validate schedule field compatibility
	if err := deadline.ValidateScheduleFields(input.Frequency, input.MonthsOfYear, true); err != nil {
		businessErr := pkg.ValidateBusinessError(err, constant.MongoCollectionDeadline, input.Frequency)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid schedule fields for frequency", businessErr)

		return nil, businessErr
	}

	// Set optional fields
	entity.Description = input.Description
	entity.MonthsOfYear = input.MonthsOfYear
	entity.NotifyDaysBefore = input.NotifyDaysBefore

	if input.Active != nil {
		entity.Active = *input.Active
	}

	// If templateId is provided, resolve the template name
	if input.TemplateID != nil {
		entity.TemplateID = input.TemplateID

		tmpl, errFind := uc.TemplateRepo.FindByID(ctx, *input.TemplateID)
		if errFind != nil {
			if errors.Is(errFind, mongo.ErrNoDocuments) {
				errNotFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.MongoCollectionTemplate)

				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Template not found", errNotFound)

				return nil, errNotFound
			}

			libOpentelemetry.HandleSpanError(span, "Failed to find template by ID", errFind)
			uc.Logger.Log(ctx, log.LevelError, "Error finding template by ID", log.Err(errFind))

			return nil, errFind
		}

		if tmpl == nil {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.MongoCollectionTemplate)
		}

		entity.TemplateName = tmpl.Description
	}

	// Convert to MongoDB model and persist
	model := deadline.FromDeadlineEntity(entity)

	result, err := uc.DeadlineRepo.Create(ctx, model)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			errDuplicate := pkg.ValidateBusinessError(constant.ErrDuplicateDeadline, constant.MongoCollectionDeadline)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Duplicate deadline detected", errDuplicate)

			return nil, errDuplicate
		}

		libOpentelemetry.HandleSpanError(span, "Failed to create deadline in repository", err)
		uc.Logger.Log(ctx, log.LevelError, "Error creating deadline", log.Err(err))

		return nil, err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Successfully created deadline", log.String("id", result.ID.String()))

	return result, nil
}
