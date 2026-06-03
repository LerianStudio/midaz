// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package midaz

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// activeStatus returns an AccountStatus with code "active".
func activeStatus() *pkg.AccountStatus {
	return &pkg.AccountStatus{Code: "active", Description: "Active account"}
}

// inactiveStatus returns an AccountStatus with code "inactive".
func inactiveStatus() *pkg.AccountStatus {
	return &pkg.AccountStatus{Code: "inactive", Description: "Inactive account"}
}

// newTestAccountResolver creates an AccountResolver with a mock MidazClient for testing.
func newTestAccountResolver(t *testing.T) (AccountResolver, *http.MockMidazClient) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockClient := http.NewMockMidazClient(ctrl)

	resolver, err := NewAccountResolver(mockClient)
	assert.NoError(t, err)
	assert.NotNil(t, resolver)

	return resolver, mockClient
}

func TestNewAccountResolver_NilClient(t *testing.T) {
	t.Parallel()

	resolver, err := NewAccountResolver(nil)

	assert.Nil(t, resolver)
	assert.Error(t, err)
	assert.Equal(t, ErrNilMidazClient, err)
}

func TestResolveAccounts(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	segmentID := uuid.New()
	portfolioID := uuid.New()

	tests := []struct {
		name            string
		target          model.AccountTarget
		setupMock       func(mock *http.MockMidazClient)
		expectedCount   int
		expectedAliases []string
		expectErr       bool
	}{
		{
			name: "resolve by segmentId - single page all active",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{SegmentID: &segmentID},
						1,
						100,
					).
					Return(&http.AccountPage{
						Items: []pkg.Account{
							{ID: "acc-1", Alias: "alice", Status: activeStatus()},
							{ID: "acc-2", Alias: "bob", Status: activeStatus()},
						},
						Page:  1,
						Limit: 100,
					}, nil).
					Times(1)
			},
			expectedCount:   2,
			expectedAliases: []string{"alice", "bob"},
			expectErr:       false,
		},
		{
			name: "resolve by portfolioId - single page all active",
			target: model.AccountTarget{
				PortfolioID: &portfolioID,
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{PortfolioID: &portfolioID},
						1,
						100,
					).
					Return(&http.AccountPage{
						Items: []pkg.Account{
							{ID: "acc-3", Alias: "charlie", Status: activeStatus()},
						},
						Page:  1,
						Limit: 100,
					}, nil).
					Times(1)
			},
			expectedCount:   1,
			expectedAliases: []string{"charlie"},
			expectErr:       false,
		},
		{
			name: "resolve by aliases - all active",
			target: model.AccountTarget{
				Aliases: []string{"alice", "bob"},
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					GetAccountDetailsByAlias(gomock.Any(), orgID.String(), ledgerID.String(), "alice").
					Return(&pkg.Account{ID: "acc-1", Alias: "alice", Status: activeStatus()}, nil).
					Times(1)

				mock.EXPECT().
					GetAccountDetailsByAlias(gomock.Any(), orgID.String(), ledgerID.String(), "bob").
					Return(&pkg.Account{ID: "acc-2", Alias: "bob", Status: activeStatus()}, nil).
					Times(1)
			},
			expectedCount:   2,
			expectedAliases: []string{"alice", "bob"},
			expectErr:       false,
		},
		{
			name: "filter inactive accounts - segmentId",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{SegmentID: &segmentID},
						1,
						100,
					).
					Return(&http.AccountPage{
						Items: []pkg.Account{
							{ID: "acc-1", Alias: "alice", Status: activeStatus()},
							{ID: "acc-2", Alias: "bob", Status: inactiveStatus()},
							{ID: "acc-3", Alias: "charlie", Status: activeStatus()},
						},
						Page:  1,
						Limit: 100,
					}, nil).
					Times(1)
			},
			expectedCount:   2,
			expectedAliases: []string{"alice", "charlie"},
			expectErr:       false,
		},
		{
			name: "filter inactive accounts - aliases",
			target: model.AccountTarget{
				Aliases: []string{"alice", "bob"},
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					GetAccountDetailsByAlias(gomock.Any(), orgID.String(), ledgerID.String(), "alice").
					Return(&pkg.Account{ID: "acc-1", Alias: "alice", Status: activeStatus()}, nil).
					Times(1)

				mock.EXPECT().
					GetAccountDetailsByAlias(gomock.Any(), orgID.String(), ledgerID.String(), "bob").
					Return(&pkg.Account{ID: "acc-2", Alias: "bob", Status: inactiveStatus()}, nil).
					Times(1)
			},
			expectedCount:   1,
			expectedAliases: []string{"alice"},
			expectErr:       false,
		},
		{
			name: "pagination - two pages of accounts",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *http.MockMidazClient) {
				// Page 1: full page (100 items)
				page1Items := make([]pkg.Account, 100)
				for i := range page1Items {
					page1Items[i] = pkg.Account{
						ID:     fmt.Sprintf("acc-p1-%03d", i),
						Alias:  fmt.Sprintf("alias-p1-%03d", i),
						Status: activeStatus(),
					}
				}

				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{SegmentID: &segmentID},
						1,
						100,
					).
					Return(&http.AccountPage{
						Items: page1Items,
						Page:  1,
						Limit: 100,
					}, nil).
					Times(1)

				// Page 2: partial page (2 items, signals end of pagination)
				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{SegmentID: &segmentID},
						2,
						100,
					).
					Return(&http.AccountPage{
						Items: []pkg.Account{
							{ID: "acc-p2-a", Alias: "alias-p2-a", Status: activeStatus()},
							{ID: "acc-p2-b", Alias: "alias-p2-b", Status: activeStatus()},
						},
						Page:  2,
						Limit: 100,
					}, nil).
					Times(1)
			},
			expectedCount: 102,
			expectErr:     false,
		},
		{
			name: "empty result - no matching active accounts",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{SegmentID: &segmentID},
						1,
						100,
					).
					Return(&http.AccountPage{
						Items: []pkg.Account{},
						Page:  1,
						Limit: 100,
					}, nil).
					Times(1)
			},
			expectedCount: 0,
			expectErr:     false,
		},
		{
			name: "error propagation - ListAccounts returns error",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{SegmentID: &segmentID},
						1,
						100,
					).
					Return(nil, errors.New("midaz unavailable")).
					Times(1)
			},
			expectedCount: 0,
			expectErr:     true,
		},
		{
			name: "error propagation - GetAccountDetailsByAlias returns error",
			target: model.AccountTarget{
				Aliases: []string{"alice"},
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					GetAccountDetailsByAlias(gomock.Any(), orgID.String(), ledgerID.String(), "alice").
					Return(nil, errors.New("midaz unavailable")).
					Times(1)
			},
			expectedCount: 0,
			expectErr:     true,
		},
		{
			name:          "empty account target returns error",
			target:        model.AccountTarget{},
			setupMock:     func(_ *http.MockMidazClient) {},
			expectedCount: 0,
			expectErr:     true,
		},
		{
			name: "filter accounts with nil status",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *http.MockMidazClient) {
				mock.EXPECT().
					ListAccounts(
						gomock.Any(),
						orgID,
						ledgerID,
						http.AccountFilters{SegmentID: &segmentID},
						1,
						100,
					).
					Return(&http.AccountPage{
						Items: []pkg.Account{
							{ID: "acc-1", Alias: "alice", Status: activeStatus()},
							{ID: "acc-2", Alias: "bob", Status: nil},
						},
						Page:  1,
						Limit: 100,
					}, nil).
					Times(1)
			},
			expectedCount:   1,
			expectedAliases: []string{"alice"},
			expectErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver, mockClient := newTestAccountResolver(t)
			tt.setupMock(mockClient)

			accounts, err := resolver.ResolveAccounts(context.Background(), orgID, ledgerID, tt.target)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, accounts)

				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, accounts)
			assert.Len(t, accounts, tt.expectedCount)

			if tt.expectedAliases != nil {
				actualAliases := make([]string, len(accounts))
				for i, acc := range accounts {
					actualAliases[i] = acc.Alias
				}

				assert.Equal(t, tt.expectedAliases, actualAliases)
			}
		})
	}
}
