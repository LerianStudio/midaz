// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestValidatePackageMaxAndMinAmountRange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	segmentID := uuid.New()
	packageID := uuid.New()

	tests := []struct {
		name             string
		maxAmount        string
		minAmount        string
		transactionRoute string
		segmentID        *uuid.UUID
		packageID        *uuid.UUID
		mockSetup        func(*pack.MockRepository)
		wantErr          bool
		errCode          string
	}{
		{
			name:             "Valid range with no existing packages",
			maxAmount:        "1000",
			minAmount:        "100",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{}, nil)
			},
			wantErr: false,
		},
		{
			name:             "Invalid max amount format",
			maxAmount:        "invalid",
			minAmount:        "100",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				existingPackage := &pack.Package{
					ID:               uuid.New(),
					MinimumAmount:    decimal.NewFromInt(50),
					MaximumAmount:    decimal.NewFromInt(500),
					TransactionRoute: stringPtr("debitoted"),
					SegmentID:        &segmentID,
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{existingPackage}, nil)
			},
			wantErr: true,
			errCode: constant.ErrConvertToDecimal.Error(),
		},
		{
			name:             "Invalid min amount format",
			maxAmount:        "1000",
			minAmount:        "invalid",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				existingPackage := &pack.Package{
					ID:               uuid.New(),
					MinimumAmount:    decimal.NewFromInt(50),
					MaximumAmount:    decimal.NewFromInt(500),
					TransactionRoute: stringPtr("debitoted"),
					SegmentID:        &segmentID,
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{existingPackage}, nil)
			},
			wantErr: true,
			errCode: constant.ErrConvertToDecimal.Error(),
		},
		{
			name:             "Duplicate package",
			maxAmount:        "1000",
			minAmount:        "100",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				existingPackage := &pack.Package{
					ID:               uuid.New(),
					MinimumAmount:    decimal.NewFromInt(100),
					MaximumAmount:    decimal.NewFromInt(1000),
					TransactionRoute: stringPtr("debitoted"),
					SegmentID:        &segmentID,
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{existingPackage}, nil)
			},
			wantErr: true,
			errCode: constant.ErrDuplicatePackage.Error(),
		},
		{
			name:             "Range overlap",
			maxAmount:        "500",
			minAmount:        "50",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				existingPackage := &pack.Package{
					ID:               uuid.New(),
					MinimumAmount:    decimal.NewFromInt(100),
					MaximumAmount:    decimal.NewFromInt(1000),
					TransactionRoute: stringPtr("debitoted"),
					SegmentID:        &segmentID,
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{existingPackage}, nil)
			},
			wantErr: true,
			errCode: constant.ErrPackageRange.Error(),
		},
		{
			name:             "No overlap - different segment",
			maxAmount:        "500",
			minAmount:        "50",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				otherSegmentID := uuid.New()
				existingPackage := &pack.Package{
					ID:               uuid.New(),
					MinimumAmount:    decimal.NewFromInt(100),
					MaximumAmount:    decimal.NewFromInt(1000),
					TransactionRoute: stringPtr("debitoted"),
					SegmentID:        &otherSegmentID,
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{existingPackage}, nil)
			},
			wantErr: false,
		},
		{
			name:             "No overlap - different route",
			maxAmount:        "500",
			minAmount:        "50",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				existingPackage := &pack.Package{
					ID:               uuid.New(),
					MinimumAmount:    decimal.NewFromInt(100),
					MaximumAmount:    decimal.NewFromInt(1000),
					TransactionRoute: stringPtr("creditfrom"),
					SegmentID:        &segmentID,
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{existingPackage}, nil)
			},
			wantErr: false,
		},
		{
			name:             "Repository error propagates to caller",
			maxAmount:        "1000",
			minAmount:        "100",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database connection failed"))
			},
			wantErr: true,
		},
		{
			name:             "Empty list from repository should not fail",
			maxAmount:        "1000",
			minAmount:        "100",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        nil,
			mockSetup: func(mockRepo *pack.MockRepository) {
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{}, nil)
			},
			wantErr: false,
		},
		{
			name:             "Update existing package - same ID should skip validation",
			maxAmount:        "1000",
			minAmount:        "100",
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			packageID:        &packageID,
			mockSetup: func(mockRepo *pack.MockRepository) {
				existingPackage := &pack.Package{
					ID:               packageID,
					MinimumAmount:    decimal.NewFromInt(100),
					MaximumAmount:    decimal.NewFromInt(1000),
					TransactionRoute: stringPtr("debitoted"),
					SegmentID:        &segmentID,
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*pack.Package{existingPackage}, nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := pack.NewMockRepository(ctrl)
			mockMidaz := pkg.NewMockMidazResolver(ctrl)

			uc := &UseCase{
				packageRepo: mockRepo,
				resolver:    mockMidaz,
			}

			tt.mockSetup(mockRepo)

			err := uc.ValidatePackageMaxAndMinAmountRange(
				ctx, nil,
				tt.maxAmount, tt.minAmount, tt.transactionRoute,
				orgID, ledgerID,
				tt.segmentID, tt.packageID,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if httpErr, ok := err.(*pkg.HTTPError); ok {
						assert.Contains(t, httpErr.Code, tt.errCode)
					} else if validationErr, ok := err.(*pkg.ValidationError); ok {
						assert.Contains(t, validationErr.Code, tt.errCode)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetFilterPackage(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	segmentID := uuid.New()

	tests := []struct {
		name             string
		segmentID        *uuid.UUID
		transactionRoute string
		expected         http.QueryHeader
	}{
		{
			name:             "With segment ID and route",
			segmentID:        &segmentID,
			transactionRoute: "debitoted",
			expected: http.QueryHeader{
				OrganizationID:   orgID,
				LedgerID:         ledgerID,
				SegmentID:        segmentID,
				TransactionRoute: stringPtr("debitoted"),
			},
		},
		{
			name:             "Without segment ID",
			segmentID:        nil,
			transactionRoute: "debitoted",
			expected: http.QueryHeader{
				OrganizationID:   orgID,
				LedgerID:         ledgerID,
				TransactionRoute: stringPtr("debitoted"),
			},
		},
		{
			name:             "Empty transaction route",
			segmentID:        &segmentID,
			transactionRoute: "",
			expected: http.QueryHeader{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				SegmentID:      segmentID,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := getFilterPackage(orgID, ledgerID, tt.segmentID, tt.transactionRoute)

			assert.Equal(t, tt.expected.OrganizationID, result.OrganizationID)
			assert.Equal(t, tt.expected.LedgerID, result.LedgerID)
			if tt.segmentID != nil {
				assert.Equal(t, tt.expected.SegmentID, result.SegmentID)
			}
			if tt.transactionRoute != "" {
				assert.NotNil(t, result.TransactionRoute)
				assert.Equal(t, *tt.expected.TransactionRoute, *result.TransactionRoute)
			} else {
				assert.Nil(t, result.TransactionRoute)
			}
		})
	}
}

func TestIsSamePackage(t *testing.T) {
	t.Parallel()

	segmentID := uuid.New()
	otherSegmentID := uuid.New()

	p := &pack.Package{
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		TransactionRoute: stringPtr("debitoted"),
		SegmentID:        &segmentID,
	}

	tests := []struct {
		name             string
		newMin           decimal.Decimal
		newMax           decimal.Decimal
		transactionRoute string
		segmentID        *uuid.UUID
		expected         bool
	}{
		{
			name:             "Same package",
			newMin:           decimal.NewFromInt(100),
			newMax:           decimal.NewFromInt(1000),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         true,
		},
		{
			name:             "Different min amount",
			newMin:           decimal.NewFromInt(200),
			newMax:           decimal.NewFromInt(1000),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         false,
		},
		{
			name:             "Different max amount",
			newMin:           decimal.NewFromInt(100),
			newMax:           decimal.NewFromInt(2000),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         false,
		},
		{
			name:             "Different segment",
			newMin:           decimal.NewFromInt(100),
			newMax:           decimal.NewFromInt(1000),
			transactionRoute: "debitoted",
			segmentID:        &otherSegmentID,
			expected:         false,
		},
		{
			name:             "Different route",
			newMin:           decimal.NewFromInt(100),
			newMax:           decimal.NewFromInt(1000),
			transactionRoute: "creditfrom",
			segmentID:        &segmentID,
			expected:         false,
		},
		{
			name:             "Nil segment ID",
			newMin:           decimal.NewFromInt(100),
			newMax:           decimal.NewFromInt(1000),
			transactionRoute: "debitoted",
			segmentID:        nil,
			expected:         false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isSamePackage(p, tt.newMin, tt.newMax, tt.transactionRoute, tt.segmentID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRangeOverlap(t *testing.T) {
	t.Parallel()

	segmentID := uuid.New()
	otherSegmentID := uuid.New()

	p := &pack.Package{
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		TransactionRoute: stringPtr("debitoted"),
		SegmentID:        &segmentID,
	}

	tests := []struct {
		name             string
		newMin           decimal.Decimal
		newMax           decimal.Decimal
		transactionRoute string
		segmentID        *uuid.UUID
		expected         bool
	}{
		{
			name:             "Overlap - new range inside existing",
			newMin:           decimal.NewFromInt(200),
			newMax:           decimal.NewFromInt(800),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         true,
		},
		{
			name:             "Overlap - new range overlaps start",
			newMin:           decimal.NewFromInt(50),
			newMax:           decimal.NewFromInt(500),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         true,
		},
		{
			name:             "Overlap - new range overlaps end",
			newMin:           decimal.NewFromInt(500),
			newMax:           decimal.NewFromInt(1500),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         true,
		},
		{
			name:             "No overlap - before existing range",
			newMin:           decimal.NewFromInt(10),
			newMax:           decimal.NewFromInt(50),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         false,
		},
		{
			name:             "No overlap - after existing range",
			newMin:           decimal.NewFromInt(2000),
			newMax:           decimal.NewFromInt(3000),
			transactionRoute: "debitoted",
			segmentID:        &segmentID,
			expected:         false,
		},
		{
			name:             "No overlap - different segment",
			newMin:           decimal.NewFromInt(200),
			newMax:           decimal.NewFromInt(800),
			transactionRoute: "debitoted",
			segmentID:        &otherSegmentID,
			expected:         false,
		},
		{
			name:             "No overlap - different route",
			newMin:           decimal.NewFromInt(200),
			newMax:           decimal.NewFromInt(800),
			transactionRoute: "creditfrom",
			segmentID:        &segmentID,
			expected:         false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isRangeOverlap(p, tt.newMin, tt.newMax, tt.transactionRoute, tt.segmentID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}
