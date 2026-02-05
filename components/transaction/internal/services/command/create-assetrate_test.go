// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateOrUpdateAssetRate(t *testing.T) {
	t.Parallel()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	tests := []struct {
		name           string
		input          *assetrate.CreateAssetRateInput
		setupMocks     func(ctrl *gomock.Controller, uc *UseCase)
		expectedError  bool
		validateResult func(t *testing.T, result *assetrate.AssetRate, err error)
	}{
		{
			name: "from_code_validation_error",
			input: &assetrate.CreateAssetRateInput{
				From:  "usd", // lowercase - invalid
				To:    "BRL",
				Rate:  100,
				Scale: 2,
				TTL:   libPointers.Int(3600),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				// No mocks needed - validation fails before any repo call
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
			},
		},
		{
			name: "from_code_with_numbers_error",
			input: &assetrate.CreateAssetRateInput{
				From:  "USD1", // contains number - invalid
				To:    "BRL",
				Rate:  100,
				Scale: 2,
				TTL:   libPointers.Int(3600),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				// No mocks needed - validation fails before any repo call
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
			},
		},
		{
			name: "to_code_validation_error",
			input: &assetrate.CreateAssetRateInput{
				From:  "USD",
				To:    "brl", // lowercase - invalid
				Rate:  100,
				Scale: 2,
				TTL:   libPointers.Int(3600),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				// No mocks needed - validation fails before any repo call
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
			},
		},
		{
			name: "find_by_currency_pair_error",
			input: &assetrate.CreateAssetRateInput{
				From:  "USD",
				To:    "BRL",
				Rate:  100,
				Scale: 2,
				TTL:   libPointers.Int(3600),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "USD", "BRL").
					Return(nil, errors.New("database connection error")).
					Times(1)
				uc.AssetRateRepo = mockAssetRateRepo
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "database connection error")
			},
		},
		{
			name: "update_existing_rate",
			input: &assetrate.CreateAssetRateInput{
				From:     "USD",
				To:       "BRL",
				Rate:     550,
				Scale:    2,
				Source:   libPointers.String("Central Bank"),
				TTL:      libPointers.Int(7200),
				Metadata: map[string]any{"updated": true},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				existingID := libCommons.GenerateUUIDv7().String()
				existingRate := &assetrate.AssetRate{
					ID:             existingID,
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					ExternalID:     libCommons.GenerateUUIDv7().String(),
					From:           "USD",
					To:             "BRL",
					Rate:           500,
					Scale:          libPointers.Float64(2),
					Source:         libPointers.String("Old Source"),
					TTL:            3600,
				}

				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "USD", "BRL").
					Return(existingRate, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, uuid.MustParse(existingID), gomock.Any()).
					DoAndReturn(func(ctx context.Context, oID, lID, id uuid.UUID, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						// Verify the updated fields
						assert.Equal(t, float64(550), ar.Rate)
						assert.Equal(t, float64(2), *ar.Scale)
						assert.Equal(t, "Central Bank", *ar.Source)
						assert.Equal(t, 7200, ar.TTL)
						return ar, nil
					}).
					Times(1)

				mockMetadataRepo := mongodb.NewMockRepository(ctrl)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "AssetRate", existingID).
					Return(nil, nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), "AssetRate", existingID, gomock.Any()).
					Return(nil).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
				uc.MetadataRepo = mockMetadataRepo
			},
			expectedError: false,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, float64(550), result.Rate)
				assert.Equal(t, "Central Bank", *result.Source)
				assert.Equal(t, 7200, result.TTL)
			},
		},
		{
			name: "update_existing_rate_with_new_external_id",
			input: &assetrate.CreateAssetRateInput{
				From:       "EUR",
				To:         "USD",
				Rate:       110,
				Scale:      2,
				TTL:        libPointers.Int(3600),
				ExternalID: libPointers.String(libCommons.GenerateUUIDv7().String()),
				// No Metadata - so UpdateMetadata skips FindByEntity, only calls Update
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				existingID := libCommons.GenerateUUIDv7().String()
				existingRate := &assetrate.AssetRate{
					ID:             existingID,
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					ExternalID:     libCommons.GenerateUUIDv7().String(),
					From:           "EUR",
					To:             "USD",
					Rate:           100,
					Scale:          libPointers.Float64(2),
					TTL:            1800,
				}

				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "EUR", "USD").
					Return(existingRate, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, uuid.MustParse(existingID), gomock.Any()).
					DoAndReturn(func(ctx context.Context, oID, lID, id uuid.UUID, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						return ar, nil
					}).
					Times(1)

				mockMetadataRepo := mongodb.NewMockRepository(ctrl)
				// When metadata is nil, UpdateMetadata skips FindByEntity and only calls Update
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), "AssetRate", existingID, gomock.Any()).
					Return(nil).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
				uc.MetadataRepo = mockMetadataRepo
			},
			expectedError: false,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, float64(110), result.Rate)
			},
		},
		{
			name: "update_repository_error",
			input: &assetrate.CreateAssetRateInput{
				From:  "USD",
				To:    "EUR",
				Rate:  95,
				Scale: 2,
				TTL:   libPointers.Int(3600),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				existingID := libCommons.GenerateUUIDv7().String()
				existingRate := &assetrate.AssetRate{
					ID:             existingID,
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					From:           "USD",
					To:             "EUR",
					Rate:           90,
					TTL:            1800,
				}

				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "USD", "EUR").
					Return(existingRate, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, uuid.MustParse(existingID), gomock.Any()).
					Return(nil, errors.New("update failed")).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "update failed")
			},
		},
		{
			name: "update_metadata_error",
			input: &assetrate.CreateAssetRateInput{
				From:     "GBP",
				To:       "USD",
				Rate:     130,
				Scale:    2,
				TTL:      libPointers.Int(3600),
				Metadata: map[string]any{"key": "value"},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				existingID := libCommons.GenerateUUIDv7().String()
				existingRate := &assetrate.AssetRate{
					ID:             existingID,
					OrganizationID: orgID.String(),
					LedgerID:       ledgerID.String(),
					From:           "GBP",
					To:             "USD",
					Rate:           125,
					TTL:            1800,
				}

				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "GBP", "USD").
					Return(existingRate, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Update(gomock.Any(), orgID, ledgerID, uuid.MustParse(existingID), gomock.Any()).
					DoAndReturn(func(ctx context.Context, oID, lID, id uuid.UUID, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						return ar, nil
					}).
					Times(1)

				mockMetadataRepo := mongodb.NewMockRepository(ctrl)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), "AssetRate", existingID).
					Return(nil, errors.New("metadata lookup failed")).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
				uc.MetadataRepo = mockMetadataRepo
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
			},
		},
		{
			name: "create_new_rate_without_metadata",
			input: &assetrate.CreateAssetRateInput{
				From:  "JPY",
				To:    "USD",
				Rate:  7,
				Scale: 4,
				TTL:   libPointers.Int(1800),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "JPY", "USD").
					Return(nil, nil). // No existing rate
					Times(1)

				mockAssetRateRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						assert.Equal(t, orgID.String(), ar.OrganizationID)
						assert.Equal(t, ledgerID.String(), ar.LedgerID)
						assert.Equal(t, "JPY", ar.From)
						assert.Equal(t, "USD", ar.To)
						assert.Equal(t, float64(7), ar.Rate)
						assert.Equal(t, float64(4), *ar.Scale)
						assert.Equal(t, 1800, ar.TTL)
						assert.NotEmpty(t, ar.ID)
						assert.NotEmpty(t, ar.ExternalID) // Auto-generated
						return ar, nil
					}).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
			},
			expectedError: false,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "JPY", result.From)
				assert.Equal(t, "USD", result.To)
				assert.Equal(t, float64(7), result.Rate)
				assert.NotEmpty(t, result.ID)
				assert.NotEmpty(t, result.ExternalID)
			},
		},
		{
			name: "create_new_rate_with_external_id",
			input: &assetrate.CreateAssetRateInput{
				From:       "CHF",
				To:         "EUR",
				Rate:       108,
				Scale:      2,
				TTL:        libPointers.Int(3600),
				ExternalID: libPointers.String("custom-external-id-123"),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "CHF", "EUR").
					Return(nil, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						assert.Equal(t, "custom-external-id-123", ar.ExternalID)
						return ar, nil
					}).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
			},
			expectedError: false,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "custom-external-id-123", result.ExternalID)
			},
		},
		{
			name: "create_new_rate_with_metadata",
			input: &assetrate.CreateAssetRateInput{
				From:     "AUD",
				To:       "NZD",
				Rate:     107,
				Scale:    2,
				TTL:      libPointers.Int(3600),
				Metadata: map[string]any{"source": "forex", "priority": "high"},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "AUD", "NZD").
					Return(nil, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						return ar, nil
					}).
					Times(1)

				mockMetadataRepo := mongodb.NewMockRepository(ctrl)
				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "AssetRate", gomock.Any()).
					DoAndReturn(func(ctx context.Context, entityName string, meta *mongodb.Metadata) error {
						assert.Equal(t, "AssetRate", entityName)
						assert.Equal(t, "forex", meta.Data["source"])
						assert.Equal(t, "high", meta.Data["priority"])
						return nil
					}).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
				uc.MetadataRepo = mockMetadataRepo
			},
			expectedError: false,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "AUD", result.From)
				assert.Equal(t, "NZD", result.To)
				assert.Equal(t, "forex", result.Metadata["source"])
				assert.Equal(t, "high", result.Metadata["priority"])
			},
		},
		{
			name: "create_repository_error",
			input: &assetrate.CreateAssetRateInput{
				From:  "CAD",
				To:    "USD",
				Rate:  75,
				Scale: 2,
				TTL:   libPointers.Int(3600),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "CAD", "USD").
					Return(nil, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("create failed: duplicate key")).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "create failed")
			},
		},
		{
			name: "create_metadata_error",
			input: &assetrate.CreateAssetRateInput{
				From:     "MXN",
				To:       "USD",
				Rate:     5,
				Scale:    2,
				TTL:      libPointers.Int(3600),
				Metadata: map[string]any{"region": "latam"},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
				mockAssetRateRepo.EXPECT().
					FindByCurrencyPair(gomock.Any(), orgID, ledgerID, "MXN", "USD").
					Return(nil, nil).
					Times(1)

				mockAssetRateRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, ar *assetrate.AssetRate) (*assetrate.AssetRate, error) {
						return ar, nil
					}).
					Times(1)

				mockMetadataRepo := mongodb.NewMockRepository(ctrl)
				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), "AssetRate", gomock.Any()).
					Return(errors.New("mongodb connection error")).
					Times(1)

				uc.AssetRateRepo = mockAssetRateRepo
				uc.MetadataRepo = mockMetadataRepo
			},
			expectedError: true,
			validateResult: func(t *testing.T, result *assetrate.AssetRate, err error) {
				assert.Nil(t, result)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "mongodb connection error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			uc := &UseCase{}
			tt.setupMocks(ctrl, uc)

			result, err := uc.CreateOrUpdateAssetRate(context.Background(), orgID, ledgerID, tt.input)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			tt.validateResult(t, result, err)
		})
	}
}
