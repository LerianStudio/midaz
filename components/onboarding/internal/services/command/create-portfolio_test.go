package command

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"
	"time"
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
					Return(nil, errors.New("failed to create portfolio")).
					Times(1)
			},
			expectedErr:   errors.New("failed to create portfolio"),
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
					Return(errors.New("failed to create metadata")).
					Times(1)
			},
			expectedErr:   errors.New("failed to create metadata"),
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
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
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
	type fields struct {
		OrganizationRepo organization.Repository
		LedgerRepo       ledger.Repository
		SegmentRepo      segment.Repository
		PortfolioRepo    portfolio.Repository
		AccountRepo      account.Repository
		AssetRepo        asset.Repository
		MetadataRepo     mongodb.Repository
		RabbitMQRepo     rabbitmq.ProducerRepository
		RedisRepo        redis.RedisRepository
	}
	type args struct {
		ctx            context.Context
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		cpi            *mmodel.CreatePortfolioInput
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *mmodel.Portfolio
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &UseCase{
				OrganizationRepo: tt.fields.OrganizationRepo,
				LedgerRepo:       tt.fields.LedgerRepo,
				SegmentRepo:      tt.fields.SegmentRepo,
				PortfolioRepo:    tt.fields.PortfolioRepo,
				AccountRepo:      tt.fields.AccountRepo,
				AssetRepo:        tt.fields.AssetRepo,
				MetadataRepo:     tt.fields.MetadataRepo,
				RabbitMQRepo:     tt.fields.RabbitMQRepo,
				RedisRepo:        tt.fields.RedisRepo,
			}
			got, err := uc.CreatePortfolio(tt.args.ctx, tt.args.organizationID, tt.args.ledgerID, tt.args.cpi)
			if (err != nil) != tt.wantErr {
				t.Errorf("UseCase.CreatePortfolio() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UseCase.CreatePortfolio() = %v, want %v", got, tt.want)
			}
		})
	}
}
