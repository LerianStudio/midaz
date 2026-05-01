// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUpdateSegmentByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := segment.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		SegmentRepo:            mockSegmentRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		segmentID      uuid.UUID
		input          *mmodel.UpdateSegmentInput
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Segment updated with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Updated Segment",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Original Segment"}, nil)
				mockSegmentRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Updated Segment").
					Return(false, nil)
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Updated Segment", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Success - Same segment name skips duplicate lookup",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Same Segment",
				Status:   mmodel.Status{Code: "active"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Same Segment"}, nil)
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Same Segment", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Segment not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Nonexistent Segment",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Duplicate segment name",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Existing Segment",
				Status:   mmodel.Status{Code: "active"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Original Segment"}, nil)
				mockSegmentRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Existing Segment").
					Return(true, pkg.ValidateBusinessError(constant.ErrDuplicateSegmentName, reflect.TypeOf(mmodel.Segment{}).Name(), "Existing Segment", uuid.New()))
			},
			expectErr: true,
		},
		{
			name:           "Error - Failed to update metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Segment with Metadata Error",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Original Segment"}, nil)
				mockSegmentRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Segment with Metadata Error").
					Return(false, nil)
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Segment with Metadata Error", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			expectErr: true,
		},
		{
			name:           "Error - Failure to update segment",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			segmentID:      uuid.New(),
			input: &mmodel.UpdateSegmentInput{
				Name:     "Update Failure Segment",
				Status:   mmodel.Status{Code: "inactive"},
				Metadata: nil,
			},
			mockSetup: func() {
				mockSegmentRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{ID: "123", Name: "Original Segment"}, nil)
				mockSegmentRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Update Failure Segment").
					Return(false, nil)
				mockSegmentRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("update error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.UpdateSegmentByID(ctx, tt.organizationID, tt.ledgerID, tt.segmentID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Name, result.Name)
				assert.Equal(t, tt.input.Status, result.Status)
			}
		})
	}
}
