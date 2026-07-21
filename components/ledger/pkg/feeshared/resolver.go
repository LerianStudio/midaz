// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"time"

	"github.com/google/uuid"
)

//go:generate mockgen --destination=./resolver_mock.go --package=pkg . MidazResolver

// MidazResolver resolves account and transaction read data from the ledger.
//
// It replaces the former outbound HTTP client: all reads are now served
// in-process by the ledger query use cases. The interface is consumed by the
// fee calculation, package-validation, and billing-calculation paths; the
// concrete implementation lives in the fees services package and is backed by
// the ledger query.UseCase.
type MidazResolver interface {
	// AccountExistsByAlias returns nil when an account with the given alias exists,
	// and a not-found business error otherwise. It is used to validate that fee
	// credit/debit accounts reference real accounts.
	AccountExistsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) error

	// GetAccountByAlias returns the account matching the given alias, or (nil, nil)
	// when no such account exists. A non-nil error signals a resolution failure
	// (not a missing account).
	GetAccountByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (*Account, error)

	// ListAccounts returns every account matching the optional segment/portfolio
	// filter, fully paginated. The implementation MUST traverse all pages so that
	// membership checks on segments larger than the page limit are not truncated.
	ListAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, segmentID, portfolioID *uuid.UUID) ([]Account, error)

	// CountTransactionsByRoute returns the number of transactions matching the
	// given route, status, and created-at window.
	CountTransactionsByRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, route, status string, startDate, endDate time.Time) (int64, error)
}
