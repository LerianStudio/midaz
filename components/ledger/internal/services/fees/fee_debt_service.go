// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"

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
	debt, err := s.Repo.FindByID(ctx, organizationID, ledgerID, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityFeeDebt)
	}

	return debt, err
}

// ListFeeDebts pages the ledger's debts oldest first; a cursor that does not decode,
// or that another listing issued, is an invalid query parameter.
func (s *FeeDebtService) ListFeeDebts(ctx context.Context, organizationID, ledgerID uuid.UUID, query fee_debt.ListQuery) ([]*model.FeeDebt, libHTTP.CursorPagination, error) {
	debts, pagination, err := s.Repo.FindAll(ctx, organizationID, ledgerID, query)
	if errors.Is(err, libHTTP.ErrInvalidCursor) || errors.Is(err, libHTTP.ErrInvalidCursorDirection) {
		return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityFeeDebt, "cursor")
	}

	return debts, pagination, err
}
