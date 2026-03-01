// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

func TestGetAllAssetRatesByAssetCode(t *testing.T) { //nolint:funlen
	t.Parallel()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	fromAssetCode := "USD"
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Run("returns_asset_rates_with_metadata", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := UseCase{AssetRateRepo: mockAssetRateRepo, MetadataRepo: mockMetadataRepo}

		assetRateID := libCommons.GenerateUUIDv7().String()

		assetRates := []*assetrate.AssetRate{
			{
				ID:             assetRateID,
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				ExternalID:     libCommons.GenerateUUIDv7().String(),
				From:           fromAssetCode,
				To:             filter.ToAssetCodes[0],
				Rate:           100,
				Scale:          libPointers.Float64(2),
				Source:         libPointers.String("External System"),
				TTL:            3600,
			},
		}

		metadata := []*mongodb.Metadata{
			{
				EntityID:   assetRateID,
				EntityName: "AssetRate",
				Data:       map[string]any{"key": "value"},
			},
		}

		mockAssetRateRepo.
			EXPECT().
			FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination()).
			Return(assetRates, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "AssetRate", filter).
			Return(metadata, nil).
			Times(1)

		result, cur, err := uc.GetAllAssetRatesByAssetCode(context.TODO(), orgID, ledgerID, fromAssetCode, filter)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, mockCur, cur)
		assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
	})

	t.Run("returns_asset_rates_without_metadata", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := UseCase{AssetRateRepo: mockAssetRateRepo, MetadataRepo: mockMetadataRepo}

		assetRateID := libCommons.GenerateUUIDv7().String()

		assetRates := []*assetrate.AssetRate{
			{
				ID:             assetRateID,
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				From:           fromAssetCode,
				To:             filter.ToAssetCodes[0],
				Rate:           150,
			},
		}

		mockAssetRateRepo.
			EXPECT().
			FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination()).
			Return(assetRates, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "AssetRate", filter).
			Return([]*mongodb.Metadata{}, nil).
			Times(1)

		result, cur, err := uc.GetAllAssetRatesByAssetCode(context.TODO(), orgID, ledgerID, fromAssetCode, filter)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, mockCur, cur)
		assert.Nil(t, result[0].Metadata)
	})

	t.Run("returns_empty_when_no_asset_rates", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		uc := UseCase{AssetRateRepo: mockAssetRateRepo}

		mockAssetRateRepo.
			EXPECT().
			FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, nil).
			Times(1)

		result, cur, err := uc.GetAllAssetRatesByAssetCode(context.TODO(), orgID, ledgerID, fromAssetCode, filter)

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	t.Run("error_invalid_asset_codes", func(t *testing.T) {
		t.Parallel()

		uc := UseCase{}

		cases := []struct {
			name            string
			fromAssetCode   string
			toAssetCodes    []string
			expectedErrCode string
		}{
			{
				name:            "invalid_from_asset_code_non_letter",
				fromAssetCode:   "123USD",
				toAssetCodes:    []string{"BRL"},
				expectedErrCode: "0033", // ErrInvalidCodeFormat: non-letter characters
			},
			{
				name:            "invalid_from_asset_code_lowercase",
				fromAssetCode:   "usd",
				toAssetCodes:    []string{"BRL"},
				expectedErrCode: "0004", // ErrCodeUppercaseRequirement: lowercase letters
			},
			{
				name:            "invalid_to_asset_code_non_letter",
				fromAssetCode:   "USD",
				toAssetCodes:    []string{"123BRL"},
				expectedErrCode: "0033", // ErrInvalidCodeFormat: non-letter characters
			},
			{
				name:            "invalid_to_asset_code_lowercase",
				fromAssetCode:   "USD",
				toAssetCodes:    []string{"brl"},
				expectedErrCode: "0004", // ErrCodeUppercaseRequirement: lowercase letters
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				testFilter := http.QueryHeader{
					Limit:        10,
					Page:         1,
					SortOrder:    "asc",
					StartDate:    time.Now().AddDate(0, -1, 0),
					EndDate:      time.Now(),
					ToAssetCodes: tc.toAssetCodes,
				}

				result, cur, err := uc.GetAllAssetRatesByAssetCode(context.TODO(), orgID, ledgerID, tc.fromAssetCode, testFilter)

				require.Error(t, err)
				assert.Nil(t, result)
				assert.Equal(t, libHTTP.CursorPagination{}, cur)
				assert.Contains(t, err.Error(), tc.expectedErrCode)
			})
		}
	})

	t.Run("error_asset_rate_repo_failure", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		uc := UseCase{AssetRateRepo: mockAssetRateRepo}

		mockAssetRateRepo.
			EXPECT().
			FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		result, cur, err := uc.GetAllAssetRatesByAssetCode(context.TODO(), orgID, ledgerID, fromAssetCode, filter)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound)
	})

	t.Run("error_metadata_repo_failure", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockAssetRateRepo := assetrate.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		uc := UseCase{AssetRateRepo: mockAssetRateRepo, MetadataRepo: mockMetadataRepo}

		assetRateID := libCommons.GenerateUUIDv7().String()

		assetRates := []*assetrate.AssetRate{
			{
				ID:             assetRateID,
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				From:           fromAssetCode,
				To:             filter.ToAssetCodes[0],
				Rate:           100,
			},
		}

		mockAssetRateRepo.
			EXPECT().
			FindAllByAssetCodes(gomock.Any(), orgID, ledgerID, fromAssetCode, filter.ToAssetCodes, filter.ToCursorPagination()).
			Return(assetRates, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), "AssetRate", filter).
			Return(nil, services.ErrDatabaseItemNotFound).
			Times(1)

		result, cur, err := uc.GetAllAssetRatesByAssetCode(context.TODO(), orgID, ledgerID, fromAssetCode, filter)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound)
	})
}
