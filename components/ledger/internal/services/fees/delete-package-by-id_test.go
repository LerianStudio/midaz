// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

func TestDeletePackageById(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	packID := uuid.New()
	orgId := uuid.New()
	packSvc := &UseCase{
		packageRepo: mockPackRepo,
	}

	tests := []struct {
		name           string
		packageID      uuid.UUID
		orgId          uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult error
	}{
		{
			name:      "Success - Delete a package",
			packageID: packID,
			orgId:     orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name:      "Error Bad Request - Delete a package",
			packageID: packID,
			orgId:     orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(constant.ErrBadRequest)
			},
			expectErr:      true,
			expectedResult: constant.ErrBadRequest,
		},
		{
			name:      "Error Document Not found - Delete a package",
			packageID: packID,
			orgId:     orgId,
			mockSetup: func() {
				mockPackRepo.EXPECT().
					SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(mongo.ErrNoDocuments)
			},
			expectErr:      true,
			expectedResult: mongo.ErrNoDocuments,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			err := packSvc.DeletePackageByID(ctx, tt.packageID, tt.orgId)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedResult, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
