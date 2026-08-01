// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"

	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// GetPackageByID recover a package by ID within the given ledger. A ledgerID of
// uuid.Nil reads the package on whichever ledger of the organization owns it.
func (uc *UseCase) GetPackageByID(ctx context.Context, id, organizationID, ledgerID uuid.UUID) (_ *pack.Package, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_package_by_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "fees", "get_package", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
		attribute.Bool("app.request.has_ledger_id", ledgerID != uuid.Nil),
	)

	packModel, err := uc.packageRepo.FindByID(ctx, id, organizationID, ledgerID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			bizErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPackage)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Package not found", bizErr)

			return nil, bizErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get package on repo by id", err)

		return nil, err
	}

	return packModel, nil
}
