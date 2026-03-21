// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUpdateOperation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationID := uuid.New()

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	tests := []struct {
		name           string
		input          *operation.UpdateOperationInput
		setupMocks     func()
		expectedErr    error
		expectedResult *operation.Operation
		checkError     func(t *testing.T, err error)
	}{
		{
			name: "operation update with metadata",
			input: &operation.UpdateOperationInput{
				Description: "Updated operation description",
				Metadata: map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
			setupMocks: func() {
				expectedOperation := &operation.Operation{
					ID:             operationID.String(),
					LedgerID:       ledgerID.String(),
					OrganizationID: organizationID.String(),
					TransactionID:  transactionID.String(),
					Description:    "Updated operation description",
					UpdatedAt:      time.Now(),
				}

				mockOperationRepo.EXPECT().
					Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, &operation.Operation{
						Description: "Updated operation description",
					}).
					Return(expectedOperation, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Operation", operationID.String()).
					Return(nil, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), "Operation", operationID.String(), map[string]any{
						"key1": "value1",
						"key2": "value2",
					}).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
			expectedResult: &operation.Operation{
				ID:             operationID.String(),
				LedgerID:       ledgerID.String(),
				OrganizationID: organizationID.String(),
				TransactionID:  transactionID.String(),
				Description:    "Updated operation description",
				Metadata: map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
		{
			name: "operation not found",
			input: &operation.UpdateOperationInput{
				Description: "Updated operation description",
			},
			setupMocks: func() {
				mockOperationRepo.EXPECT().
					Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, &operation.Operation{
						Description: "Updated operation description",
					}).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: nil,
			checkError: func(t *testing.T, err error) {
				var entityNotFoundError pkg.EntityNotFoundError
				assert.True(t, errors.As(err, &entityNotFoundError), "expected EntityNotFoundError")
				assert.Equal(t, "Operation", entityNotFoundError.EntityType)
			},
		},
		{
			name: "repository error",
			input: &operation.UpdateOperationInput{
				Description: "Updated operation description",
			},
			setupMocks: func() {
				mockOperationRepo.EXPECT().
					Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, &operation.Operation{
						Description: "Updated operation description",
					}).
					Return(nil, errors.New("database connection error")).
					Times(1)
			},
			expectedErr: errors.New("database connection error"),
		},
		{
			name: "metadata update error",
			input: &operation.UpdateOperationInput{
				Description: "Updated operation description",
				Metadata: map[string]any{
					"key1": "value1",
				},
			},
			setupMocks: func() {
				expectedOperation := &operation.Operation{
					ID:             operationID.String(),
					LedgerID:       ledgerID.String(),
					OrganizationID: organizationID.String(),
					TransactionID:  transactionID.String(),
					Description:    "Updated operation description",
					UpdatedAt:      time.Now(),
				}

				mockOperationRepo.EXPECT().
					Update(gomock.Any(), organizationID, ledgerID, transactionID, operationID, &operation.Operation{
						Description: "Updated operation description",
					}).
					Return(expectedOperation, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "Operation", operationID.String()).
					Return(nil, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), "Operation", operationID.String(), map[string]any{
						"key1": "value1",
					}).
					Return(errors.New("mongodb connection error")).
					Times(1)
			},
			expectedErr: errors.New("mongodb connection error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.UpdateOperation(context.Background(), organizationID, ledgerID, transactionID, operationID, tt.input)

			if tt.checkError != nil {
				assert.Error(t, err)
				tt.checkError(t, err)
				assert.Nil(t, result)
				return
			}

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.ID, result.ID)
				assert.Equal(t, tt.expectedResult.Description, result.Description)
				assert.Equal(t, tt.expectedResult.Metadata, result.Metadata)
			}
		})
	}
}
