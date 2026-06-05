// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/deadline"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// buildDeadlineUpdateFields maps non-nil input fields to BSON update fields, validating constrained values.
func buildDeadlineUpdateFields(input *deadline.UpdateDeadlineInput) (bson.M, error) {
	setFields := bson.M{}

	if input.Name != nil {
		setFields["name"] = *input.Name
	}

	if input.Description != nil {
		setFields["description"] = *input.Description
	}

	if input.Type != nil {
		if !deadline.ValidTypes[*input.Type] {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidDeadlineType, constant.MongoCollectionDeadline)
		}

		setFields["type"] = *input.Type
	}

	if input.DueDate != nil {
		today := time.Now().Truncate(24 * time.Hour)
		if input.DueDate.Truncate(24 * time.Hour).Before(today) {
			return nil, pkg.ValidateBusinessError(constant.ErrDueDateInPast, constant.MongoCollectionDeadline)
		}

		setFields["due_date"] = *input.DueDate
	}

	if input.Frequency != nil {
		if !deadline.ValidFrequencies[*input.Frequency] {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidDeadlineFrequency, constant.MongoCollectionDeadline)
		}

		setFields["frequency"] = *input.Frequency
	}

	if input.MonthsOfYear != nil {
		setFields["months_of_year"] = input.MonthsOfYear
	}

	if input.Active != nil {
		setFields["active"] = *input.Active
	}

	if input.NotifyDaysBefore != nil {
		setFields["notify_days_before"] = *input.NotifyDaysBefore
	}

	if input.Color != nil {
		if !deadline.ValidColorRegex.MatchString(*input.Color) {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidDeadlineColor, constant.MongoCollectionDeadline)
		}

		setFields["color"] = *input.Color
	}

	setFields["updated_at"] = time.Now()

	return setFields, nil
}

// validateAndCleanSchedule merges the input schedule fields with the current deadline state,
// validates the merged result, and returns bson fields to $unset for orphaned recurrence fields.
func validateAndCleanSchedule(current *deadline.Deadline, input *deadline.UpdateDeadlineInput, setFields bson.M) (bson.M, error) {
	mergedFreq := current.Frequency
	if input.Frequency != nil {
		mergedFreq = *input.Frequency
	}

	mergedMonthsOfYear := current.MonthsOfYear
	if input.MonthsOfYear != nil {
		mergedMonthsOfYear = input.MonthsOfYear
	}

	if err := deadline.ValidateScheduleFields(mergedFreq, mergedMonthsOfYear, true); err != nil {
		return nil, pkg.ValidateBusinessError(err, constant.MongoCollectionDeadline, mergedFreq)
	}

	unsetFields := bson.M{}

	if !deadline.FrequenciesWithMonthsOfYear[mergedFreq] && len(current.MonthsOfYear) > 0 {
		unsetFields["months_of_year"] = ""

		delete(setFields, "months_of_year")
	}

	return unsetFields, nil
}

// UpdateDeadlineByID updates an existing deadline with partial fields and returns the updated entity.
func (uc *UseCase) UpdateDeadlineByID(ctx context.Context, id uuid.UUID, input *deadline.UpdateDeadlineInput) (*deadline.Deadline, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.deadline.update")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.deadline_id", id.String()),
	)
	uc.Logger.Log(ctx, log.LevelInfo, "Updating deadline", log.String("id", id.String()))

	// Fetch current deadline to validate merged state
	current, err := uc.DeadlineRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			errNotFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.MongoCollectionDeadline)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Deadline not found", errNotFound)

			return nil, errNotFound
		}

		libOpentelemetry.HandleSpanError(span, "Failed to find deadline by ID", err)
		uc.Logger.Log(ctx, log.LevelError, "Error finding deadline by ID", log.Err(err))

		return nil, err
	}

	setFields, err := buildDeadlineUpdateFields(input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid deadline update input", err)
		return nil, err
	}

	// Validate merged schedule state and compute fields to $unset
	unsetFields, err := validateAndCleanSchedule(current, input, setFields)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid schedule fields after merge", err)
		return nil, err
	}

	// If templateId changes, resolve and sync the denormalized template_name
	if input.TemplateID != nil {
		setFields["template_id"] = *input.TemplateID

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

		setFields["template_name"] = tmpl.Description
	}

	updateFields := bson.M{"$set": setFields}

	if len(unsetFields) > 0 {
		updateFields["$unset"] = unsetFields
	}

	if errUpdate := uc.DeadlineRepo.Update(ctx, id, &updateFields); errUpdate != nil {
		if errors.Is(errUpdate, mongo.ErrNoDocuments) {
			errNotFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.MongoCollectionDeadline)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Deadline not found", errNotFound)

			return nil, errNotFound
		}

		if mongo.IsDuplicateKeyError(errUpdate) {
			errDuplicate := pkg.ValidateBusinessError(constant.ErrDuplicateDeadline, constant.MongoCollectionDeadline)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Duplicate deadline detected on update", errDuplicate)

			return nil, errDuplicate
		}

		libOpentelemetry.HandleSpanError(span, "Failed to update deadline in repository", errUpdate)
		uc.Logger.Log(ctx, log.LevelError, "Error updating deadline", log.Err(errUpdate))

		return nil, errUpdate
	}

	// Fetch the updated deadline to return
	updated, err := uc.DeadlineRepo.FindByID(ctx, id)
	if err != nil {
		if pkgHTTP.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve updated deadline", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve updated deadline", err)
		}

		uc.Logger.Log(ctx, log.LevelError, "Failed to retrieve deadline", log.String("id", id.String()), log.Err(err))

		return nil, err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Successfully updated deadline", log.String("id", id.String()))

	return updated, nil
}
