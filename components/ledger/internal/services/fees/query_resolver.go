// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
)

// feeQueryPort is the narrow slice of the ledger query.UseCase that fee
// resolution depends on. It exists so the resolver can be unit-tested with a
// fake; *query.UseCase satisfies it directly.
type feeQueryPort interface {
	GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, alias string) (*mmodel.Account, error)
	GetAllAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID, segmentID *uuid.UUID, filter libHTTP.QueryHeader) ([]*mmodel.Account, error)
	CountTransactionsByFilters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter transaction.CountFilter) (int64, error)
}

// Compile-time assurance that the concrete query.UseCase satisfies the port.
var _ feeQueryPort = (*query.UseCase)(nil)

// accountListPageSize is the page size used when paginating GetAllAccount.
// It is the maximum page limit allowed by the ledger query layer; a smaller
// value would only increase round-trips.
const accountListPageSize = 100

// resolverFarFutureEndDate bounds the created_at filter on GetAllAccount so
// that every account is returned regardless of creation time. Account
// membership resolution must not be time-windowed.
var resolverFarFutureEndDate = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// queryResolver is the in-process implementation of feeshared.MidazResolver, backed
// by the ledger query.UseCase. It serves the account/transaction reads that the
// fee paths previously obtained over HTTP.
type queryResolver struct {
	query feeQueryPort
}

// NewQueryResolver returns a MidazResolver backed by the given query use case.
func NewQueryResolver(q *query.UseCase) (feeshared.MidazResolver, error) {
	if q == nil {
		return nil, errors.New("query use case is required and cannot be nil")
	}

	return &queryResolver{query: q}, nil
}

func (r *queryResolver) AccountExistsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) error {
	if _, err := r.query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias); err != nil {
		return err
	}

	return nil
}

func (r *queryResolver) GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (*feeshared.Account, error) {
	account, err := r.query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias)
	if err != nil {
		// A missing alias is not a resolution failure for callers that treat a
		// non-existent account as "not exempt"; collapse it to (nil, nil).
		if errors.Is(err, constant.ErrAccountAliasNotFound) {
			return nil, nil
		}

		return nil, err
	}

	if account == nil {
		return nil, nil
	}

	return toFeeAccount(account), nil
}

func (r *queryResolver) ListAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, segmentID, portfolioID *uuid.UUID) ([]feeshared.Account, error) {
	var accounts []feeshared.Account

	page := 1

	for {
		filter := libHTTP.QueryHeader{
			Limit:     accountListPageSize,
			Page:      page,
			SortOrder: "asc",
			StartDate: time.Unix(0, 0).UTC(),
			EndDate:   resolverFarFutureEndDate,
		}

		batch, err := r.query.GetAllAccount(ctx, organizationID, ledgerID, portfolioID, segmentID, filter)
		if err != nil {
			// An empty segment/portfolio surfaces as a not-found business error
			// from the query layer; treat it as an empty result set, not failure.
			if errors.Is(err, constant.ErrNoAccountsFound) {
				return accounts, nil
			}

			return nil, err
		}

		for _, a := range batch {
			if a == nil {
				continue
			}

			accounts = append(accounts, *toFeeAccount(a))
		}

		// Fewer than a full page signals the last page.
		if len(batch) < accountListPageSize {
			break
		}

		page++
	}

	return accounts, nil
}

func (r *queryResolver) CountTransactionsByRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, route, status string, startDate, endDate time.Time) (int64, error) {
	return r.query.CountTransactionsByFilters(ctx, organizationID, ledgerID, transaction.CountFilter{
		Route:     route,
		Status:    status,
		StartDate: startDate,
		EndDate:   endDate,
	})
}

// toFeeAccount maps a ledger domain account onto the fee-side account shape.
func toFeeAccount(a *mmodel.Account) *feeshared.Account {
	out := &feeshared.Account{
		ID:   a.ID,
		Type: a.Type,
		Status: &feeshared.AccountStatus{
			Code: a.Status.Code,
		},
	}

	if a.Status.Description != nil {
		out.Status.Description = *a.Status.Description
	}

	if a.Alias != nil {
		out.Alias = *a.Alias
	}

	if a.SegmentID != nil {
		if id, err := uuid.Parse(*a.SegmentID); err == nil {
			out.SegmentID = &id
		}
	}

	if a.PortfolioID != nil {
		if id, err := uuid.Parse(*a.PortfolioID); err == nil {
			out.PortfolioID = &id
		}
	}

	return out
}
