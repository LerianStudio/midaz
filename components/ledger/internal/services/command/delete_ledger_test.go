// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeleteLedgerByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:             mockLedgerRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name        string
		setupMocks  func()
		expectedErr error
	}{
		{
			name: "success - ledger deleted",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityLedger, ledgerID.String()).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "success - metadata soft delete failure does not fail the delete",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityLedger, ledgerID.String()).
					Return(errors.New("mongo unavailable")).
					Times(3)
			},
			expectedErr: nil,
		},
		{
			name: "failure - ledger not found",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityLedger, ledgerID.String()).
					Times(0)
			},
			expectedErr: errors.New("The provided ledger ID does not exist in our records. Please verify the ledger ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(errors.New("failed to delete ledger")).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityLedger, ledgerID.String()).
					Times(0)
			},
			expectedErr: errors.New("failed to delete ledger"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteLedgerByID(ctx, organizationID, ledgerID)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteLedgerByID_MetadataSoftDeleteFailureStillEmits verifies that a
// metadata soft delete failing on every attempt neither fails the request nor
// suppresses the deleted event.
func TestDeleteLedgerByID_MetadataSoftDeleteFailureStillEmits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockLedgerRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID).
		Return(nil).
		Times(1)

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Delete(gomock.Any(), constant.EntityLedger, ledgerID.String()).
		Return(errors.New("mongo unavailable")).
		Times(fastMetadataDeleteRetryPolicy().Attempts)

	mockEmitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		LedgerRepo:             mockLedgerRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
		Streaming:              mockEmitter,
	}

	err := uc.DeleteLedgerByID(context.Background(), organizationID, ledgerID)
	require.NoError(t, err, "a metadata soft delete failure must not fail the delete")

	require.Len(t, mockEmitter.Events(), 1)
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "ledger", "deleted")
}
