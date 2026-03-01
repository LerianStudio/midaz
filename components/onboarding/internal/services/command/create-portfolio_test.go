// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

var (
	errPortCreate           = errors.New("failed to create portfolio")
	errPortMetadataCreation = errors.New("failed to create metadata")
)

//nolint:funlen
func TestCreatePortfolio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mocks
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		PortfolioRepo: mockPortfolioRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name          string
		input         *mmodel.CreatePortfolioInput
		mockSetup     func()
		expectedErr   error
		expectedPortf *mmodel.Portfolio
	}{
		{
			name: "success - portfolio created",
			input: &mmodel.CreatePortfolioInput{
				Name:     "Test Portfolio",
				EntityID: "entity-123",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: libPointers.String("Active portfolio"),
				},
				Metadata: map[string]any{
					"key1": "value1",
				},
			},
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Portfolio{
						ID:             uuid.New().String(),
						EntityID:       "entity-123",
						LedgerID:       ledgerID.String(),
						OrganizationID: organizationID.String(),
						Name:           "Test Portfolio",
						Status: mmodel.Status{
							Code:        "ACTIVE",
							Description: libPointers.String("Active portfolio"),
						},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "Portfolio", gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
			expectedPortf: &mmodel.Portfolio{
				Name: "Test Portfolio",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
		},
		{
			name: "failure - repository error",
			input: &mmodel.CreatePortfolioInput{
				Name:     "Test Portfolio",
				EntityID: "entity-123",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errPortCreate).
					Times(1)
			},
			expectedErr:   errPortCreate,
			expectedPortf: nil,
		},
		{
			name: "failure - metadata creation error",
			input: &mmodel.CreatePortfolioInput{
				Name:     "Test Portfolio",
				EntityID: "entity-123",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: map[string]any{
					"key1": "value1",
				},
			},
			mockSetup: func() {
				mockPortfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Portfolio{
						ID:             uuid.New().String(),
						EntityID:       "entity-123",
						LedgerID:       ledgerID.String(),
						OrganizationID: organizationID.String(),
						Name:           "Test Portfolio",
						Status: mmodel.Status{
							Code: "ACTIVE",
						},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "Portfolio", gomock.Any()).
					Return(errPortMetadataCreation).
					Times(1)
			},
			expectedErr:   errPortMetadataCreation,
			expectedPortf: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configura os mocks
			tt.mockSetup()

			// Executa a função
			result, err := uc.CreatePortfolio(ctx, organizationID, ledgerID, tt.input)

			// Validações
			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedPortf.Name, result.Name)
				assert.Equal(t, tt.expectedPortf.Status.Code, result.Status.Code)
			}
		})
	}
}

// TestCreatePortfolioSuccess is responsible to test CreatePortfolio with success.
func TestCreatePortfolioSuccess(t *testing.T) {
	p := &mmodel.Portfolio{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		EntityID:       libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		PortfolioRepo: portfolio.NewMockRepository(gomock.NewController(t)),
	}

	mockRepo, ok := uc.PortfolioRepo.(*portfolio.MockRepository)
	require.True(t, ok, "expected PortfolioRepo to be *portfolio.MockRepository")

	mockRepo.
		EXPECT().
		Create(gomock.Any(), p).
		Return(p, nil).
		Times(1)
	res, err := uc.PortfolioRepo.Create(context.TODO(), p)

	assert.Equal(t, p, res)
	require.NoError(t, err)
}

// TestCreatePortfolioError is responsible to test CreatePortfolio with error.
func TestCreatePortfolioError(t *testing.T) {
	errMSG := "failed to create portfolio"
	p := &mmodel.Portfolio{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		EntityID:       libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		PortfolioRepo: portfolio.NewMockRepository(gomock.NewController(t)),
	}

	mockRepo2, ok := uc.PortfolioRepo.(*portfolio.MockRepository)
	require.True(t, ok, "expected PortfolioRepo to be *portfolio.MockRepository")

	mockRepo2.
		EXPECT().
		Create(gomock.Any(), p).
		Return(nil, errPortCreate).
		Times(1)
	res, err := uc.PortfolioRepo.Create(context.TODO(), p)

	require.Error(t, err)
	require.ErrorContains(t, err, errMSG)
	assert.Nil(t, res)
}
