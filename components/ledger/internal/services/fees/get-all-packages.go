// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllPackages fetch all Packages from the repository
func (uc *UseCase) GetAllPackages(ctx context.Context, filters http.QueryHeader, organizationID uuid.UUID) ([]*pack.Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_packages")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	filters.OrganizationID = organizationID

	span.SetAttributes(
		attribute.Int("app.request.limit", filters.Limit),
		attribute.Int("app.request.page", filters.Page),
		attribute.Bool("app.request.has_segment_id", filters.SegmentID != uuid.Nil),
		attribute.Bool("app.request.has_ledger_id", filters.LedgerID != uuid.Nil),
		attribute.Bool("app.request.has_transaction_route", filters.TransactionRoute != nil),
		attribute.Bool("app.request.has_enable", filters.Enable != nil),
	)

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
