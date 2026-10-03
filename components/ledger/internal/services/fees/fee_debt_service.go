// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// FeeDebtService reads the Fees record of fee debts. The record lags the engine's
// live debts by completion latency.
type FeeDebtService struct {
	Repo *fee_debt.Repository
	// Accounts reads the debtors a scoped listing is confined to.
	Accounts DebtorAccounts
}

// DebtorAccounts reads the live accounts of a ledger by id.
type DebtorAccounts interface {
	ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error)
}

// errDebtorConfinementUnavailable refuses a scoped listing the service has no
// way to confine.
var errDebtorConfinementUnavailable = errors.New("fee debt listing is confined to accounts but no account reader is configured")

// GetFeeDebt returns the ledger's debt with id; a debt of another ledger is not found.
func (s *FeeDebtService) GetFeeDebt(ctx context.Context, organizationID, ledgerID uuid.UUID, id string) (*model.FeeDebt, error) {
	debt, err := s.Repo.FindByID(ctx, organizationID, ledgerID, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityFeeDebt)
	}

	return debt, err
}

// ListFeeDebts pages the ledger's debts oldest first, confined to the debtors
// scope allows; a cursor that does not decode, or that another listing issued,
// is an invalid query parameter.
func (s *FeeDebtService) ListFeeDebts(ctx context.Context, organizationID, ledgerID uuid.UUID, query fee_debt.ListQuery, scope http.ScopeConfinement) ([]*model.FeeDebt, libHTTP.CursorPagination, error) {
	query, listsNothing, err := s.confineDebtors(ctx, organizationID, ledgerID, query, scope)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	if listsNothing {
		return []*model.FeeDebt{}, libHTTP.CursorPagination{}, nil
	}

	debts, pagination, err := s.Repo.FindAll(ctx, organizationID, ledgerID, query)
	if errors.Is(err, libHTTP.ErrInvalidCursor) || errors.Is(err, libHTTP.ErrInvalidCursorDirection) {
		return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityFeeDebt, "cursor")
	}

	return debts, pagination, err
}

// OpenFeeDebtTotal sums what the debtor still owes over all its open debts.
func (s *FeeDebtService) OpenFeeDebtTotal(ctx context.Context, organizationID, ledgerID uuid.UUID, debtorBalanceRef string) (decimal.Decimal, error) {
	return s.Repo.OpenTotal(ctx, organizationID, ledgerID, debtorBalanceRef)
}

// confineDebtors narrows query to the debts of the accounts scope allows. A debt
// names its debtor by alias, so the allowed accounts are read for theirs; an
// allowed account that no longer exists names no debtor. listsNothing reports a
// confinement nothing can satisfy.
func (s *FeeDebtService) confineDebtors(ctx context.Context, organizationID, ledgerID uuid.UUID, query fee_debt.ListQuery, scope http.ScopeConfinement) (fee_debt.ListQuery, bool, error) {
	if len(scope) == 0 {
		return query, false, nil
	}

	ids, confined := scope.IDs("accountId")
	if !confined || len(scope) > 1 || len(ids) == 0 {
		return query, true, nil
	}

	if s.Accounts == nil {
		return query, false, errDebtorConfinementUnavailable
	}

	accounts, err := s.Accounts.ListAccountsByIDs(ctx, organizationID, ledgerID, ids)
	if err != nil {
		return query, false, err
	}

	aliases := make([]string, 0, len(accounts))

	for _, acc := range accounts {
		if acc != nil && acc.Alias != nil && *acc.Alias != "" {
			aliases = append(aliases, *acc.Alias)
		}
	}

	if len(aliases) == 0 {
		return query, true, nil
	}

	query.ConfineDebtors = true
	query.DebtorAliases = aliases

	return query, false, nil
}
