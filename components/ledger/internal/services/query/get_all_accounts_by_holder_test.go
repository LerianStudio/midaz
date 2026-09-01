// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestGetAllAccountsByHolder(t *testing.T) {
	organizationID := uuid.New()
	holderID := uuid.New()
	ledgerID := uuid.New()

	filter := http.QueryHeader{Limit: 10, Page: 1, SortOrder: "asc"}

	tests := []struct {
		name             string
		ledgerID         *uuid.UUID
		setupMocks       func(*account.MockRepository, *mongodb.MockRepository)
		expectedErrCode  string
		expectedErrText  string
		expectedAccounts []*mmodel.Account
	}{
		{
			name:     "success - org-wide listing enriched with metadata",
			ledgerID: nil,
			setupMocks: func(accounts *account.MockRepository, metadata *mongodb.MockRepository) {
				accounts.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), filter, mmodel.HolderOnV2).
					Return([]*mmodel.Account{{ID: "acc1"}, {ID: "acc2"}}, nil).
					Times(1)

				metadata.EXPECT().
					FindByEntityIDs(gomock.Any(), constant.EntityAccount, []string{"acc1", "acc2"}).
					Return([]*mongodb.Metadata{{EntityID: "acc1", Data: map[string]any{"k": "v"}}}, nil).
					Times(1)
			},
			expectedAccounts: []*mmodel.Account{
				{ID: "acc1", Metadata: map[string]any{"k": "v"}},
				{ID: "acc2"},
			},
		},
		{
			name:     "success - ledger id narrows the listing",
			ledgerID: &ledgerID,
			setupMocks: func(accounts *account.MockRepository, metadata *mongodb.MockRepository) {
				accounts.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, matchLedgerID(ledgerID), filter, mmodel.HolderOnV2).
					Return([]*mmodel.Account{{ID: "acc1"}}, nil).
					Times(1)

				metadata.EXPECT().
					FindByEntityIDs(gomock.Any(), constant.EntityAccount, []string{"acc1"}).
					Return([]*mongodb.Metadata{}, nil).
					Times(1)
			},
			expectedAccounts: []*mmodel.Account{{ID: "acc1"}},
		},
		{
			name:     "empty result skips the metadata lookup",
			ledgerID: nil,
			setupMocks: func(accounts *account.MockRepository, metadata *mongodb.MockRepository) {
				accounts.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), filter, mmodel.HolderOnV2).
					Return([]*mmodel.Account{}, nil).
					Times(1)

				metadata.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedAccounts: []*mmodel.Account{},
		},
		{
			name:     "database item not found becomes ErrNoAccountsFound",
			ledgerID: nil,
			setupMocks: func(accounts *account.MockRepository, metadata *mongodb.MockRepository) {
				accounts.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), filter, mmodel.HolderOnV2).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErrCode: constant.ErrNoAccountsFound.Error(),
		},
		{
			name:     "generic repository error propagates",
			ledgerID: nil,
			setupMocks: func(accounts *account.MockRepository, metadata *mongodb.MockRepository) {
				accounts.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), filter, mmodel.HolderOnV2).
					Return(nil, errors.New("connection refused")).
					Times(1)
			},
			expectedErrText: "connection refused",
		},
		{
			name:     "metadata failure becomes ErrNoAccountsFound",
			ledgerID: nil,
			setupMocks: func(accounts *account.MockRepository, metadata *mongodb.MockRepository) {
				accounts.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), filter, mmodel.HolderOnV2).
					Return([]*mmodel.Account{{ID: "acc1"}}, nil).
					Times(1)

				metadata.EXPECT().
					FindByEntityIDs(gomock.Any(), constant.EntityAccount, []string{"acc1"}).
					Return(nil, errors.New("mongo down")).
					Times(1)
			},
			expectedErrCode: constant.ErrNoAccountsFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			accountRepo := account.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(accountRepo, metadataRepo)

			uc := &UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo}

			got, err := uc.GetAllAccountsByHolder(context.Background(), organizationID, holderID, tt.ledgerID, filter, mmodel.HolderOnV2)

			if tt.expectedErrCode != "" {
				require.Error(t, err)

				var notFound pkg.EntityNotFoundError
				require.ErrorAs(t, err, &notFound)
				assert.Equal(t, tt.expectedErrCode, notFound.Code)
				assert.Nil(t, got)

				return
			}

			if tt.expectedErrText != "" {
				require.EqualError(t, err, tt.expectedErrText)
				assert.Nil(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedAccounts, got)
		})
	}
}

// matchLedgerID matches a *uuid.UUID argument by the value it points at, so the
// expectation pins the narrowing ledger rather than the pointer identity.
func matchLedgerID(want uuid.UUID) gomock.Matcher {
	return gomock.Cond(func(arg *uuid.UUID) bool {
		return arg != nil && *arg == want
	})
}
