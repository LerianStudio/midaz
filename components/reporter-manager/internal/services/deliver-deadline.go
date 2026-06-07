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
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/deadline"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// DeliverDeadline marks a deadline as delivered or clears the delivery status.
func (uc *UseCase) DeliverDeadline(ctx context.Context, id uuid.UUID, input *deadline.DeliverDeadlineInput) (*deadline.Deadline, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.deadline.deliver")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.deadline_id", id.String()),
	)

	if input.Delivered == nil {
		validationErr := pkg.ValidateBusinessError(cnErr.ErrInvalidPathParameter, "", "delivered field is required")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Delivered field is nil", validationErr)

		return nil, validationErr
	}

	span.SetAttributes(attribute.Bool("app.request.delivered", *input.Delivered))
	uc.Logger.Log(ctx, log.LevelInfo, "Delivering deadline",
		log.String("id", id.String()),
		log.Bool("delivered", *input.Delivered),
	)

	setFields := bson.M{
		"updated_at": time.Now(),
	}

	if *input.Delivered {
		now := time.Now()
		setFields["delivered_at"] = &now
	} else {
		setFields["delivered_at"] = nil
	}

	updateFields := bson.M{"$set": setFields}

	if errUpdate := uc.DeadlineRepo.Update(ctx, id, &updateFields); errUpdate != nil {
		if nf := (pkg.EntityNotFoundError{}); errors.As(errUpdate, &nf) {
			errNotFound := pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionDeadline)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Deadline not found", errNotFound)

			return nil, errNotFound
		}

		libOpentelemetry.HandleSpanError(span, "Failed to update deadline delivery status", errUpdate)
		uc.Logger.Log(ctx, log.LevelError, "Error updating deadline delivery status", log.Err(errUpdate))

		return nil, errUpdate
	}

	// Fetch the updated deadline to return
	updated, err := uc.DeadlineRepo.FindByID(ctx, id)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve delivered deadline", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve delivered deadline", err)
		}

		uc.Logger.Log(ctx, log.LevelError, "Failed to retrieve deadline", log.String("id", id.String()), log.Err(err))

		return nil, err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Deadline delivery status updated", log.String("id", id.String()))

	return updated, nil
}
