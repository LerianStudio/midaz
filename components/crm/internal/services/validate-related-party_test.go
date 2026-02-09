// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
)

func TestValidateRelatedParty(t *testing.T) {
	uc := &UseCase{}

	now := time.Now()
	past := now.Add(-48 * time.Hour)
	future := now.Add(48 * time.Hour)

	testCases := []struct {
		name        string
		party       *mmodel.RelatedParty
		expectErr   bool
		expectedErr error
	}{
		{
			name: "Success with valid related party",
			party: &mmodel.RelatedParty{
				Document:  "12345678900",
				Name:      "Maria de Jesus",
				Role:      "PRIMARY_HOLDER",
				StartDate: mmodel.Date{Time: now},
			},
			expectErr: false,
		},
		{
			name: "Error when document is empty",
			party: &mmodel.RelatedParty{
				Document:  "",
				Name:      "Maria de Jesus",
				Role:      "PRIMARY_HOLDER",
				StartDate: mmodel.Date{Time: now},
			},
			expectErr:   true,
			expectedErr: cn.ErrRelatedPartyDocumentRequired,
		},
		{
			name: "Error when document is whitespace",
			party: &mmodel.RelatedParty{
				Document:  "   ",
				Name:      "Maria de Jesus",
				Role:      "PRIMARY_HOLDER",
				StartDate: mmodel.Date{Time: now},
			},
			expectErr:   true,
			expectedErr: cn.ErrRelatedPartyDocumentRequired,
		},
		{
			name: "Error when name is empty",
			party: &mmodel.RelatedParty{
				Document:  "12345678900",
				Name:      "",
				Role:      "PRIMARY_HOLDER",
				StartDate: mmodel.Date{Time: now},
			},
			expectErr:   true,
			expectedErr: cn.ErrRelatedPartyNameRequired,
		},
		{
			name: "Error when role is invalid",
			party: &mmodel.RelatedParty{
				Document:  "12345678900",
				Name:      "Maria de Jesus",
				Role:      "INVALID_ROLE",
				StartDate: mmodel.Date{Time: now},
			},
			expectErr:   true,
			expectedErr: cn.ErrInvalidRelatedPartyRole,
		},
		{
			name: "Error when role is empty",
			party: &mmodel.RelatedParty{
				Document:  "12345678900",
				Name:      "Maria de Jesus",
				Role:      "",
				StartDate: mmodel.Date{Time: now},
			},
			expectErr:   true,
			expectedErr: cn.ErrInvalidRelatedPartyRole,
		},
		{
			name: "Error when start date is zero",
			party: &mmodel.RelatedParty{
				Document: "12345678900",
				Name:     "Maria de Jesus",
				Role:     "PRIMARY_HOLDER",
			},
			expectErr:   true,
			expectedErr: cn.ErrRelatedPartyStartDateRequired,
		},
		{
			name: "Error when end date is before start date",
			party: &mmodel.RelatedParty{
				Document:  "12345678900",
				Name:      "Maria de Jesus",
				Role:      "PRIMARY_HOLDER",
				StartDate: mmodel.Date{Time: now},
				EndDate:   &mmodel.Date{Time: past},
			},
			expectErr:   true,
			expectedErr: cn.ErrRelatedPartyEndDateInvalid,
		},
		{
			name: "Success when end date is after start date",
			party: &mmodel.RelatedParty{
				Document:  "12345678900",
				Name:      "Maria de Jesus",
				Role:      "LEGAL_REPRESENTATIVE",
				StartDate: mmodel.Date{Time: now},
				EndDate:   &mmodel.Date{Time: future},
			},
			expectErr: false,
		},
		{
			name: "Success with RESPONSIBLE_PARTY role",
			party: &mmodel.RelatedParty{
				Document:  "12345678900",
				Name:      "Maria de Jesus",
				Role:      "RESPONSIBLE_PARTY",
				StartDate: mmodel.Date{Time: now},
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			err := uc.ValidateRelatedParty(ctx, tc.party)

			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRelatedParties(t *testing.T) {
	uc := &UseCase{}

	now := time.Now()

	testCases := []struct {
		name      string
		parties   []*mmodel.RelatedParty
		expectErr bool
	}{
		{
			name: "Success with multiple valid parties",
			parties: []*mmodel.RelatedParty{
				{
					Document:  "12345678900",
					Name:      "Maria de Jesus",
					Role:      "PRIMARY_HOLDER",
					StartDate: mmodel.Date{Time: now},
				},
				{
					Document:  "99988877766",
					Name:      "Joao Silva",
					Role:      "LEGAL_REPRESENTATIVE",
					StartDate: mmodel.Date{Time: now},
				},
			},
			expectErr: false,
		},
		{
			name: "Error stops at first invalid party",
			parties: []*mmodel.RelatedParty{
				{
					Document:  "12345678900",
					Name:      "Maria de Jesus",
					Role:      "PRIMARY_HOLDER",
					StartDate: mmodel.Date{Time: now},
				},
				{
					Document:  "",
					Name:      "Invalid Party",
					Role:      "PRIMARY_HOLDER",
					StartDate: mmodel.Date{Time: now},
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			err := uc.ValidateRelatedParties(ctx, tc.parties)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
