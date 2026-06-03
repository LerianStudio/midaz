// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllPackages fetch all Packages from the repository
func (uc *UseCase) GetAllPackages(ctx context.Context, filters http.QueryHeader, organizationID uuid.UUID) ([]*pack.Package, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_packages")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, "Retrieving packages")

	filters.OrganizationID = organizationID

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", filters, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert payload to JSON string", err)
	}

	packs, err := uc.packageRepo.FindList(ctx, filters)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get packages on repo", err)

		return nil, err
	}

	if packs == nil {
		return []*pack.Package{}, nil
	}

	return packs, nil
}
