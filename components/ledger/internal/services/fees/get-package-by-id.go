// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// GetPackageByID recover a package by ID
func (uc *UseCase) GetPackageByID(ctx context.Context, id, organizationID uuid.UUID) (*pack.Package, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_package_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieving package for id %v and organizationId %v.", id, organizationID))

	packModel, err := uc.packageRepo.FindByID(ctx, id, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get package on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting package on repo by id: %v", err))

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", reflect.TypeOf(pack.Package{}).Name())
		}

		return nil, err
	}

	return packModel, nil
}
