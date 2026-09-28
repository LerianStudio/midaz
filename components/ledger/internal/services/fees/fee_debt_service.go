// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// FeeDebtService reads the Fees record of fee debts. The record lags the engine's
// live debts by completion latency.
type FeeDebtService struct {
	Repo *fee_debt.Repository
}

// GetFeeDebt returns the ledger's debt with id; a debt of another ledger is not found.
func (s *FeeDebtService) GetFeeDebt(ctx context.Context, organizationID, ledgerID uuid.UUID, id string) (*model.FeeDebt, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.fee_debt.get")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.fee_debt_id", id),
	)

	debt, err := s.Repo.FindByID(ctx, organizationID, ledgerID, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		notFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityFeeDebt)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Fee debt not found", notFound)

		return nil, notFound
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get fee debt", err)

		return nil, err
	}

	return debt, nil
}

// ListFeeDebts pages the ledger's debts oldest first; an undecodable cursor is an
// invalid query parameter.
func (s *FeeDebtService) ListFeeDebts(ctx context.Context, organizationID, ledgerID uuid.UUID, query fee_debt.ListQuery) ([]*model.FeeDebt, libHTTP.CursorPagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.fee_debt.list")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	debts, pagination, err := s.Repo.FindAll(ctx, organizationID, ledgerID, query)
	if errors.Is(err, libHTTP.ErrInvalidCursor) || errors.Is(err, libHTTP.ErrInvalidCursorDirection) {
		invalid := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityFeeDebt, "cursor")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid fee debt cursor", invalid)

		return nil, libHTTP.CursorPagination{}, invalid
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list fee debts", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	return debts, pagination, nil
}
