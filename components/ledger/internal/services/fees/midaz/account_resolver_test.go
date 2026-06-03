// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package midaz

import (
	"context"
	"errors"
	"testing"

	pkg "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"

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

// newTestAccountResolver creates an AccountResolver with a mock MidazResolver for testing.
func newTestAccountResolver(t *testing.T) (AccountResolver, *pkg.MockMidazResolver) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockResolver := pkg.NewMockMidazResolver(ctrl)

	resolver, err := NewAccountResolver(mockResolver)
	assert.NoError(t, err)
	assert.NotNil(t, resolver)

	return resolver, mockResolver
}

func TestNewAccountResolver_NilResolver(t *testing.T) {
	t.Parallel()

	resolver, err := NewAccountResolver(nil)

	assert.Nil(t, resolver)
	assert.Error(t, err)
	assert.Equal(t, ErrNilResolver, err)
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
		setupMock       func(mock *pkg.MockMidazResolver)
		expectedCount   int
		expectedAliases []string
		expectErr       bool
	}{
		{
			name: "resolve by segmentId - all active",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					ListAccounts(gomock.Any(), orgID, ledgerID, &segmentID, nil).
					Return([]pkg.Account{
						{ID: "acc-1", Alias: "alice", Status: activeStatus()},
						{ID: "acc-2", Alias: "bob", Status: activeStatus()},
					}, nil).
					Times(1)
			},
			expectedCount:   2,
			expectedAliases: []string{"alice", "bob"},
			expectErr:       false,
		},
		{
			name: "resolve by portfolioId - all active",
			target: model.AccountTarget{
				PortfolioID: &portfolioID,
			},
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					ListAccounts(gomock.Any(), orgID, ledgerID, nil, &portfolioID).
					Return([]pkg.Account{
						{ID: "acc-3", Alias: "charlie", Status: activeStatus()},
					}, nil).
					Times(1)
			},
			expectedCount:   1,
			expectedAliases: []string{"charlie"},
			expectErr:       false,
		},
		{
			name: "resolve by aliases - all active (with dedup)",
			target: model.AccountTarget{
				Aliases: []string{"alice", "bob", "alice"},
			},
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "alice").
					Return(&pkg.Account{ID: "acc-1", Alias: "alice", Status: activeStatus()}, nil).
					Times(1)

				mock.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "bob").
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
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					ListAccounts(gomock.Any(), orgID, ledgerID, &segmentID, nil).
					Return([]pkg.Account{
						{ID: "acc-1", Alias: "alice", Status: activeStatus()},
						{ID: "acc-2", Alias: "bob", Status: inactiveStatus()},
						{ID: "acc-3", Alias: "charlie", Status: activeStatus()},
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
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "alice").
					Return(&pkg.Account{ID: "acc-1", Alias: "alice", Status: activeStatus()}, nil).
					Times(1)

				mock.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "bob").
					Return(&pkg.Account{ID: "acc-2", Alias: "bob", Status: inactiveStatus()}, nil).
					Times(1)
			},
			expectedCount:   1,
			expectedAliases: []string{"alice"},
			expectErr:       false,
		},
		{
			name: "aliases - account not found is skipped",
			target: model.AccountTarget{
				Aliases: []string{"alice", "ghost"},
			},
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "alice").
					Return(&pkg.Account{ID: "acc-1", Alias: "alice", Status: activeStatus()}, nil).
					Times(1)

				mock.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "ghost").
					Return(nil, nil).
					Times(1)
			},
			expectedCount:   1,
			expectedAliases: []string{"alice"},
			expectErr:       false,
		},
		{
			name: "empty result - no matching active accounts",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					ListAccounts(gomock.Any(), orgID, ledgerID, &segmentID, nil).
					Return([]pkg.Account{}, nil).
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
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					ListAccounts(gomock.Any(), orgID, ledgerID, &segmentID, nil).
					Return(nil, errors.New("query failed")).
					Times(1)
			},
			expectedCount: 0,
			expectErr:     true,
		},
		{
			name: "error propagation - GetAccountByAlias returns error",
			target: model.AccountTarget{
				Aliases: []string{"alice"},
			},
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					GetAccountByAlias(gomock.Any(), orgID, ledgerID, "alice").
					Return(nil, errors.New("query failed")).
					Times(1)
			},
			expectedCount: 0,
			expectErr:     true,
		},
		{
			name:          "empty account target returns error",
			target:        model.AccountTarget{},
			setupMock:     func(_ *pkg.MockMidazResolver) {},
			expectedCount: 0,
			expectErr:     true,
		},
		{
			name: "filter accounts with nil status",
			target: model.AccountTarget{
				SegmentID: &segmentID,
			},
			setupMock: func(mock *pkg.MockMidazResolver) {
				mock.EXPECT().
					ListAccounts(gomock.Any(), orgID, ledgerID, &segmentID, nil).
					Return([]pkg.Account{
						{ID: "acc-1", Alias: "alice", Status: activeStatus()},
						{ID: "acc-2", Alias: "bob", Status: nil},
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

			resolver, mockResolver := newTestAccountResolver(t)
			tt.setupMock(mockResolver)

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
