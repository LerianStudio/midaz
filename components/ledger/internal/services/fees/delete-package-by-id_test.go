// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&pack.Package{ID: packID, LedgerID: uuid.New()}, nil)

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
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&pack.Package{ID: packID, LedgerID: uuid.New()}, nil)

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
					FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&pack.Package{ID: packID, LedgerID: uuid.New()}, nil)

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

// TestDeletePackageByID_EmitsFeesPackageDeleted asserts a successful delete emits
// the fees-package.deleted event, using the ledger resolved by an independent
// FindByID call (DECISION 2).
func TestDeletePackageByID_EmitsFeesPackageDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	orgID := uuid.New()
	packID := uuid.New()
	ledgerID := uuid.New()

	found := &pack.Package{ID: packID, LedgerID: ledgerID}

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(found, nil)
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	svc := &UseCase{
		packageRepo: mockPackRepo,
		Streaming:   mockEmitter,
	}

	err := svc.DeletePackageByID(context.Background(), packID, orgID)
	require.NoError(t, err)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "fees-package", "deleted")

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)
	req := emitted[0]
	assert.Equal(t, packID.String(), req.Subject)

	payload := unmarshalPayload(t, req.Payload)
	assert.Equal(t, orgID.String(), payload["organizationId"])
	// The delete event must carry the ledger resolved by FindByID, not any
	// cache-resolved ledger (DECISION 2 regression guard).
	assert.Equal(t, found.LedgerID.String(), payload["ledgerId"])
}

// TestDeletePackageByID_FindByIDFailure_SkipsEmitButDeletes asserts that a
// FindByID failure skips only the emit; SoftDelete still runs (DECISION 2).
func TestDeletePackageByID_FindByIDFailure_SkipsEmitButDeletes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	orgID := uuid.New()
	packID := uuid.New()

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, assert.AnError)
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	svc := &UseCase{
		packageRepo: mockPackRepo,
		Streaming:   mockEmitter,
	}

	err := svc.DeletePackageByID(context.Background(), packID, orgID)
	require.NoError(t, err)

	assert.Empty(t, mockEmitter.Events(), "no event must be emitted when the package cannot be resolved")
}
