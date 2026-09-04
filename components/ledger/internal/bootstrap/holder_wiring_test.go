// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// fakeHolderByIDReader stubs GetHolderByID for the Exists discrimination test.
type fakeHolderByIDReader struct {
	holder *mmodel.Holder
	err    error
}

func (f fakeHolderByIDReader) GetHolderByID(_ context.Context, _ string, _ uuid.UUID, _ bool) (*mmodel.Holder, error) {
	return f.holder, f.err
}

func TestHolderReaderAdapter_Exists(t *testing.T) {
	id := uuid.New()
	holder := &mmodel.Holder{ID: &id}

	infraErr := errors.New("mongo timeout")

	tests := []struct {
		name       string
		reader     fakeHolderByIDReader
		wantExists bool
		wantErr    bool
		wantErrIs  error
	}{
		{
			name:       "holder found",
			reader:     fakeHolderByIDReader{holder: holder},
			wantExists: true,
		},
		{
			name: "holder-not-found business error maps to (false, nil)",
			reader: fakeHolderByIDReader{err: pkg.EntityNotFoundError{
				Code: constant.ErrHolderNotFound.Error(),
			}},
			wantExists: false,
		},
		{
			name: "different EntityNotFoundError code propagates",
			reader: fakeHolderByIDReader{err: pkg.EntityNotFoundError{
				Code: constant.ErrOrganizationIDNotFound.Error(),
			}},
			wantExists: false,
			wantErr:    true,
		},
		{
			name:       "infrastructure error propagates",
			reader:     fakeHolderByIDReader{err: infraErr},
			wantExists: false,
			wantErr:    true,
			wantErrIs:  infraErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := holderReaderAdapter{service: tt.reader}

			exists, err := adapter.Exists(context.Background(), "org-1", id)

			assert.Equal(t, tt.wantExists, exists)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestHolderAccountsReaderAdapter_ListAccountsByHolder(t *testing.T) {
	organizationID := uuid.New()
	holderID := uuid.New()
	ledgerID := uuid.New()

	found := []*mmodel.Account{{ID: "acc1"}}
	repoErr := errors.New("connection refused")

	tests := []struct {
		name string
		// ledgerIDParam is the raw ?ledger_id= value; nil means the parameter was absent.
		ledgerIDParam *string
		expect        func(*account.MockRepository)
		wantAccounts  []*mmodel.Account
		wantErrCode   string
		wantErrText   string
	}{
		{
			name:          "no ledger_id lists across every ledger",
			ledgerIDParam: nil,
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), gomock.Any(), mmodel.HolderOnV2).
					Return(found, nil).
					Times(1)
			},
			wantAccounts: found,
		},
		{
			name:          "empty ledger_id counts as absent",
			ledgerIDParam: libPointers.String(""),
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), gomock.Any(), mmodel.HolderOnV2).
					Return(found, nil).
					Times(1)
			},
			wantAccounts: found,
		},
		{
			name:          "valid ledger_id narrows the listing",
			ledgerIDParam: libPointers.String(ledgerID.String()),
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, matchLedgerIDPtr(ledgerID), gomock.Any(), mmodel.HolderOnV2).
					Return(found, nil).
					Times(1)
			},
			wantAccounts: found,
		},
		{
			name:          "malformed ledger_id is a query parameter validation error",
			ledgerIDParam: libPointers.String("not-a-uuid"),
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().FindAllByHolder(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			wantErrCode: constant.ErrInvalidQueryParameter.Error(),
		},
		{
			name:          "repository error propagates",
			ledgerIDParam: nil,
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), gomock.Any(), mmodel.HolderOnV2).
					Return(nil, repoErr).
					Times(1)
			},
			wantErrText: repoErr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			accountRepo := account.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			tt.expect(accountRepo)

			if len(tt.wantAccounts) > 0 {
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), constant.EntityAccount, gomock.Any()).
					Return(nil, nil).
					Times(1)
			}

			adapter := holderAccountsReaderAdapter{
				query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo},
			}

			filter := http.QueryHeader{Limit: 10, Page: 1, SortOrder: "asc", LedgerID: tt.ledgerIDParam}

			got, err := adapter.ListAccountsByHolder(context.Background(), organizationID.String(), holderID, filter)

			switch {
			case tt.wantErrCode != "":
				require.Error(t, err)

				var validation pkg.ValidationError
				require.ErrorAs(t, err, &validation)
				assert.Equal(t, tt.wantErrCode, validation.Code)
				assert.Nil(t, got)
			case tt.wantErrText != "":
				require.EqualError(t, err, tt.wantErrText)
				assert.Nil(t, got)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.wantAccounts, got)
			}
		})
	}
}

// TestHolderAccountsReaderAdapter_RejectsMalformedOrganizationID pins the org
// parse guard, which runs before any ledger_id handling.
func TestHolderAccountsReaderAdapter_RejectsMalformedOrganizationID(t *testing.T) {
	adapter := holderAccountsReaderAdapter{query: &query.UseCase{}}

	got, err := adapter.ListAccountsByHolder(context.Background(), "not-a-uuid", uuid.New(), http.QueryHeader{})

	require.Error(t, err)
	assert.Nil(t, got)

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, constant.ErrOrganizationIDNotFound.Error(), notFound.Code)
}

// matchLedgerIDPtr matches a *uuid.UUID by the value it points at.
func matchLedgerIDPtr(want uuid.UUID) gomock.Matcher {
	return gomock.Cond(func(arg *uuid.UUID) bool {
		return arg != nil && *arg == want
	})
}
