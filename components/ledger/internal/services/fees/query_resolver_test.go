// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	pkgconstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// fakeQueryPort is a hand-rolled feeQueryPort double that records the filters it
// was called with and returns scripted results.
type fakeQueryPort struct {
	getByAliasFn func(ctx context.Context, org, ledger uuid.UUID, portfolio *uuid.UUID, alias string) (*mmodel.Account, error)
	getAllFn     func(ctx context.Context, org, ledger uuid.UUID, portfolio, segment *uuid.UUID, filter libHTTP.QueryHeader) ([]*mmodel.Account, error)
	countFn      func(ctx context.Context, org, ledger uuid.UUID, filter transaction.CountFilter) (int64, error)

	getAllCalls []libHTTP.QueryHeader
}

func (f *fakeQueryPort) GetAccountByAlias(ctx context.Context, org, ledger uuid.UUID, portfolio *uuid.UUID, alias string) (*mmodel.Account, error) {
	return f.getByAliasFn(ctx, org, ledger, portfolio, alias)
}

func (f *fakeQueryPort) GetAllAccount(ctx context.Context, org, ledger uuid.UUID, portfolio, segment *uuid.UUID, filter libHTTP.QueryHeader) ([]*mmodel.Account, error) {
	f.getAllCalls = append(f.getAllCalls, filter)
	return f.getAllFn(ctx, org, ledger, portfolio, segment, filter)
}

func (f *fakeQueryPort) CountTransactionsByFilters(ctx context.Context, org, ledger uuid.UUID, filter transaction.CountFilter) (int64, error) {
	return f.countFn(ctx, org, ledger, filter)
}

func newResolverWithPort(port feeQueryPort) *queryResolver {
	return &queryResolver{query: port}
}

func mmodelAccount(id, alias string, statusCode string, segmentID *string) *mmodel.Account {
	a := &mmodel.Account{
		ID:     id,
		Type:   "deposit",
		Status: mmodel.Status{Code: statusCode},
	}
	a.Alias = &alias
	a.SegmentID = segmentID

	return a
}

func TestQueryResolver_GetAccountByAlias_MapsFields(t *testing.T) {
	t.Parallel()

	org, ledger := uuid.New(), uuid.New()
	segID := uuid.New()
	segStr := segID.String()

	port := &fakeQueryPort{
		getByAliasFn: func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, alias string) (*mmodel.Account, error) {
			assert.Equal(t, "alice", alias)
			return mmodelAccount("acc-1", "alice", "ACTIVE", &segStr), nil
		},
	}

	r := newResolverWithPort(port)

	acc, err := r.GetAccountByAlias(context.Background(), org, ledger, "alice")
	assert.NoError(t, err)
	assert.NotNil(t, acc)
	assert.Equal(t, "acc-1", acc.ID)
	assert.Equal(t, "alice", acc.Alias)
	assert.NotNil(t, acc.Status)
	assert.Equal(t, "ACTIVE", acc.Status.Code)
	assert.NotNil(t, acc.SegmentID)
	assert.Equal(t, segID, *acc.SegmentID)
}

func TestQueryResolver_GetAccountByAlias_NotFoundCollapsesToNil(t *testing.T) {
	t.Parallel()

	port := &fakeQueryPort{
		getByAliasFn: func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ string) (*mmodel.Account, error) {
			return nil, pkgconstant.ErrAccountAliasNotFound
		},
	}

	r := newResolverWithPort(port)

	acc, err := r.GetAccountByAlias(context.Background(), uuid.New(), uuid.New(), "ghost")
	assert.NoError(t, err)
	assert.Nil(t, acc)
}

func TestQueryResolver_GetAccountByAlias_OtherErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	port := &fakeQueryPort{
		getByAliasFn: func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ string) (*mmodel.Account, error) {
			return nil, wantErr
		},
	}

	r := newResolverWithPort(port)

	acc, err := r.GetAccountByAlias(context.Background(), uuid.New(), uuid.New(), "alice")
	assert.ErrorIs(t, err, wantErr)
	assert.Nil(t, acc)
}

func TestQueryResolver_AccountExistsByAlias(t *testing.T) {
	t.Parallel()

	t.Run("exists returns nil", func(t *testing.T) {
		t.Parallel()
		port := &fakeQueryPort{
			getByAliasFn: func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ string) (*mmodel.Account, error) {
				return mmodelAccount("acc-1", "alice", "ACTIVE", nil), nil
			},
		}
		assert.NoError(t, newResolverWithPort(port).AccountExistsByAlias(context.Background(), uuid.New(), uuid.New(), "alice"))
	})

	t.Run("not found propagates error", func(t *testing.T) {
		t.Parallel()
		port := &fakeQueryPort{
			getByAliasFn: func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ string) (*mmodel.Account, error) {
				return nil, pkgconstant.ErrAccountAliasNotFound
			},
		}
		assert.Error(t, newResolverWithPort(port).AccountExistsByAlias(context.Background(), uuid.New(), uuid.New(), "ghost"))
	})
}

// TestQueryResolver_ListAccounts_FullPaginationNoTruncation is the T06 acceptance
// test: a segment with more than one page of accounts must be fully traversed.
func TestQueryResolver_ListAccounts_FullPaginationNoTruncation(t *testing.T) {
	t.Parallel()

	org, ledger := uuid.New(), uuid.New()
	segID := uuid.New()

	// Page 1: a full page (100), Page 2: a full page (100), Page 3: partial (37) -> end.
	page1 := make([]*mmodel.Account, accountListPageSize)
	for i := range page1 {
		page1[i] = mmodelAccount(fmt.Sprintf("p1-%03d", i), fmt.Sprintf("alias-p1-%03d", i), "ACTIVE", nil)
	}

	page2 := make([]*mmodel.Account, accountListPageSize)
	for i := range page2 {
		page2[i] = mmodelAccount(fmt.Sprintf("p2-%03d", i), fmt.Sprintf("alias-p2-%03d", i), "ACTIVE", nil)
	}

	page3 := make([]*mmodel.Account, 37)
	for i := range page3 {
		page3[i] = mmodelAccount(fmt.Sprintf("p3-%03d", i), fmt.Sprintf("alias-p3-%03d", i), "ACTIVE", nil)
	}

	port := &fakeQueryPort{
		getAllFn: func(_ context.Context, gotOrg, gotLedger uuid.UUID, portfolio, segment *uuid.UUID, filter libHTTP.QueryHeader) ([]*mmodel.Account, error) {
			assert.Equal(t, org, gotOrg)
			assert.Equal(t, ledger, gotLedger)
			assert.Nil(t, portfolio)
			assert.NotNil(t, segment)
			assert.Equal(t, segID, *segment)
			assert.Equal(t, accountListPageSize, filter.Limit, "must respect the max-limit-100 page size")

			switch filter.Page {
			case 1:
				return page1, nil
			case 2:
				return page2, nil
			case 3:
				return page3, nil
			default:
				return []*mmodel.Account{}, nil
			}
		},
	}

	r := newResolverWithPort(port)

	accounts, err := r.ListAccounts(context.Background(), org, ledger, &segID, nil)
	assert.NoError(t, err)
	// 100 + 100 + 37 = 237: full coverage, no truncation at the 100-row boundary.
	assert.Len(t, accounts, 237)
	assert.Len(t, port.getAllCalls, 3, "must page until a short page is returned")

	// Spot-check that accounts from every page are present (no page dropped).
	aliases := make(map[string]struct{}, len(accounts))
	for _, a := range accounts {
		aliases[a.Alias] = struct{}{}
	}
	assert.Contains(t, aliases, "alias-p1-000")
	assert.Contains(t, aliases, "alias-p2-099")
	assert.Contains(t, aliases, "alias-p3-036")
}

func TestQueryResolver_ListAccounts_NoAccountsFoundCollapsesToEmpty(t *testing.T) {
	t.Parallel()

	port := &fakeQueryPort{
		getAllFn: func(_ context.Context, _, _ uuid.UUID, _, _ *uuid.UUID, _ libHTTP.QueryHeader) ([]*mmodel.Account, error) {
			return nil, pkgconstant.ErrNoAccountsFound
		},
	}

	r := newResolverWithPort(port)

	accounts, err := r.ListAccounts(context.Background(), uuid.New(), uuid.New(), nil, nil)
	assert.NoError(t, err)
	assert.Empty(t, accounts)
}

func TestQueryResolver_ListAccounts_ErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("query failed")
	port := &fakeQueryPort{
		getAllFn: func(_ context.Context, _, _ uuid.UUID, _, _ *uuid.UUID, _ libHTTP.QueryHeader) ([]*mmodel.Account, error) {
			return nil, wantErr
		},
	}

	r := newResolverWithPort(port)

	accounts, err := r.ListAccounts(context.Background(), uuid.New(), uuid.New(), nil, nil)
	assert.ErrorIs(t, err, wantErr)
	assert.Nil(t, accounts)
}

func TestQueryResolver_CountTransactionsByRoute_MapsToCountFilter(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	var captured transaction.CountFilter

	port := &fakeQueryPort{
		countFn: func(_ context.Context, _, _ uuid.UUID, filter transaction.CountFilter) (int64, error) {
			captured = filter
			return 42, nil
		},
	}

	r := newResolverWithPort(port)

	count, err := r.CountTransactionsByRoute(context.Background(), uuid.New(), uuid.New(), "route-x", "APPROVED", start, end)
	assert.NoError(t, err)
	assert.Equal(t, int64(42), count)
	assert.Equal(t, "route-x", captured.Route)
	assert.Equal(t, "APPROVED", captured.Status)
	assert.Equal(t, start, captured.StartDate)
	assert.Equal(t, end, captured.EndDate)
}
