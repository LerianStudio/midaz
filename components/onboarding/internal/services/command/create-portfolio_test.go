package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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
		name              string
		input             *mmodel.CreatePortfolioInput
		mockSetup         func()
		expectInternalErr bool
		expectedPortf     *mmodel.Portfolio
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
			expectInternalErr: false,
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
					Return(nil, errors.New("failed to create portfolio")).
					Times(1)
			},
			expectInternalErr: true,
			expectedPortf:     nil,
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
					Return(errors.New("failed to create metadata")).
					Times(1)
			},
			expectInternalErr: true,
			expectedPortf:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configura os mocks
			tt.mockSetup()

			// Executa a função
			result, err := uc.CreatePortfolio(ctx, organizationID, ledgerID, tt.input)

			// Validações
			if tt.expectInternalErr {
				assert.Error(t, err)
				var internalErr pkg.InternalServerError
				assert.True(t, errors.As(err, &internalErr), "expected InternalServerError type")
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedPortf.Name, result.Name)
				assert.Equal(t, tt.expectedPortf.Status.Code, result.Status.Code)
			}
		})
	}
}

// TestCreatePortfolioSuccess is responsible to test CreatePortfolio with success
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

	uc.PortfolioRepo.(*portfolio.MockRepository).
		EXPECT().
		Create(gomock.Any(), p).
		Return(p, nil).
		Times(1)
	res, err := uc.PortfolioRepo.Create(context.TODO(), p)

	assert.Equal(t, p, res)
	assert.Nil(t, err)
}

// TestCreatePortfolioError is responsible to test CreatePortfolio with error
func TestCreatePortfolioError(t *testing.T) {
	errMSG := "err to create portfolio on database"
	p := &mmodel.Portfolio{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		EntityID:       libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
	}

	uc := UseCase{
		PortfolioRepo: portfolio.NewMockRepository(gomock.NewController(t)),
	}

	uc.PortfolioRepo.(*portfolio.MockRepository).
		EXPECT().
		Create(gomock.Any(), p).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.PortfolioRepo.Create(context.TODO(), p)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}

func TestUseCase_CreatePortfolio(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name      string
		input     *mmodel.CreatePortfolioInput
		setupMock func(ctrl *gomock.Controller) (*portfolio.MockRepository, *mongodb.MockRepository)
		validate  func(t *testing.T, result *mmodel.Portfolio, err error)
	}{
		{
			name: "default status when status is empty",
			input: &mmodel.CreatePortfolioInput{
				Name:     "Empty Status Portfolio",
				EntityID: "entity-empty-status",
				Status:   mmodel.Status{}, // Empty status - no metadata
			},
			setupMock: func(ctrl *gomock.Controller) (*portfolio.MockRepository, *mongodb.MockRepository) {
				mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
				mockMetadataRepo := mongodb.NewMockRepository(ctrl)

				mockPortfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, p *mmodel.Portfolio) (*mmodel.Portfolio, error) {
						// Verify the status was defaulted to ACTIVE
						assert.Equal(t, "ACTIVE", p.Status.Code)
						return p, nil
					}).
					Times(1)

				// No metadata in input, so MetadataRepo.Create is not called

				return mockPortfolioRepo, mockMetadataRepo
			},
			validate: func(t *testing.T, result *mmodel.Portfolio, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "ACTIVE", result.Status.Code)
			},
		},
		{
			name: "preserves custom status code",
			input: &mmodel.CreatePortfolioInput{
				Name:     "Custom Status Portfolio",
				EntityID: "entity-custom-status",
				Status: mmodel.Status{
					Code:        "PENDING",
					Description: libPointers.String("Pending approval"),
				},
				// No metadata
			},
			setupMock: func(ctrl *gomock.Controller) (*portfolio.MockRepository, *mongodb.MockRepository) {
				mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
				mockMetadataRepo := mongodb.NewMockRepository(ctrl)

				mockPortfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, p *mmodel.Portfolio) (*mmodel.Portfolio, error) {
						// Verify the custom status is preserved
						assert.Equal(t, "PENDING", p.Status.Code)
						assert.Equal(t, "Pending approval", *p.Status.Description)
						return p, nil
					}).
					Times(1)

				// No metadata in input, so MetadataRepo.Create is not called

				return mockPortfolioRepo, mockMetadataRepo
			},
			validate: func(t *testing.T, result *mmodel.Portfolio, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "PENDING", result.Status.Code)
				assert.Equal(t, "Pending approval", *result.Status.Description)
			},
		},
		{
			name: "assigns organization and ledger IDs",
			input: &mmodel.CreatePortfolioInput{
				Name:     "ID Assignment Portfolio",
				EntityID: "entity-id-assignment",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				// No metadata
			},
			setupMock: func(ctrl *gomock.Controller) (*portfolio.MockRepository, *mongodb.MockRepository) {
				mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
				mockMetadataRepo := mongodb.NewMockRepository(ctrl)

				mockPortfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, p *mmodel.Portfolio) (*mmodel.Portfolio, error) {
						// Verify organization and ledger IDs are properly assigned
						assert.Equal(t, organizationID.String(), p.OrganizationID)
						assert.Equal(t, ledgerID.String(), p.LedgerID)
						assert.Equal(t, "entity-id-assignment", p.EntityID)
						assert.Equal(t, "ID Assignment Portfolio", p.Name)
						return p, nil
					}).
					Times(1)

				// No metadata in input, so MetadataRepo.Create is not called

				return mockPortfolioRepo, mockMetadataRepo
			},
			validate: func(t *testing.T, result *mmodel.Portfolio, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, organizationID.String(), result.OrganizationID)
				assert.Equal(t, ledgerID.String(), result.LedgerID)
			},
		},
		{
			name: "with metadata",
			input: &mmodel.CreatePortfolioInput{
				Name:     "Metadata Portfolio",
				EntityID: "entity-metadata",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: map[string]any{
					"department": "finance",
					"region":     "us-east",
					"priority":   1,
				},
			},
			setupMock: func(ctrl *gomock.Controller) (*portfolio.MockRepository, *mongodb.MockRepository) {
				mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
				mockMetadataRepo := mongodb.NewMockRepository(ctrl)

				mockPortfolioRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, p *mmodel.Portfolio) (*mmodel.Portfolio, error) {
						return p, nil
					}).
					Times(1)

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "Portfolio", gomock.Any()).
					DoAndReturn(func(ctx context.Context, entityName string, meta *mongodb.Metadata) error {
						// Verify metadata is properly passed
						assert.NotEmpty(t, meta.EntityID)
						assert.Equal(t, "Portfolio", meta.EntityName)
						assert.Equal(t, "finance", meta.Data["department"])
						assert.Equal(t, "us-east", meta.Data["region"])
						assert.Equal(t, 1, meta.Data["priority"])
						return nil
					}).
					Times(1)

				return mockPortfolioRepo, mockMetadataRepo
			},
			validate: func(t *testing.T, result *mmodel.Portfolio, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				// Verify metadata is returned in the result
				assert.NotNil(t, result.Metadata)
				assert.Equal(t, "finance", result.Metadata["department"])
				assert.Equal(t, "us-east", result.Metadata["region"])
				assert.Equal(t, 1, result.Metadata["priority"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPortfolioRepo, mockMetadataRepo := tt.setupMock(ctrl)

			uc := &UseCase{
				PortfolioRepo: mockPortfolioRepo,
				MetadataRepo:  mockMetadataRepo,
			}

			result, err := uc.CreatePortfolio(ctx, organizationID, ledgerID, tt.input)
			tt.validate(t, result, err)
		})
	}
}
