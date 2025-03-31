package repository_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestAssetInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAsset := repository.NewMockAsset(ctrl)

	// Test interface compliance by using the mock
	var _ repository.Asset = mockAsset
}

func TestAsset_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAsset := repository.NewMockAsset(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		input          mmodel.CreateAssetInput
		mockSetup      func()
		expectedResult *mmodel.Asset
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreateAssetInput{
				Name: "Brazilian Real",
				Type: "currency",
				Code: "BRL",
			},
			mockSetup: func() {
				expectedAsset := &mmodel.Asset{
					ID:   "asset123",
					Name: "Brazilian Real",
					Type: "currency",
					Code: "BRL",
				}
				mockAsset.EXPECT().
					Create("org123", "ledger123", gomock.Any()).
					Return(expectedAsset, nil)
			},
			expectedResult: &mmodel.Asset{
				ID:   "asset123",
				Name: "Brazilian Real",
				Type: "currency",
				Code: "BRL",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreateAssetInput{
				Name: "Brazilian Real",
				Type: "currency",
				Code: "BRL",
			},
			mockSetup: func() {
				mockAsset.EXPECT().
					Create("org123", "ledger123", gomock.Any()).
					Return(nil, errors.New("creation failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("creation failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockAsset.Create(tc.organizationID, tc.ledgerID, tc.input)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestAsset_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAsset := repository.NewMockAsset(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		limit          int
		page           int
		sortOrder      string
		startDate      string
		endDate        string
		mockSetup      func()
		expectedResult *mmodel.Assets
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			limit:          10,
			page:           1,
			sortOrder:      "asc",
			startDate:      "2025-01-01",
			endDate:        "2025-03-30",
			mockSetup: func() {
				expectedAssets := &mmodel.Assets{
					Items: []mmodel.Asset{
						{
							ID:   "asset123",
							Name: "Brazilian Real",
							Type: "currency",
							Code: "BRL",
						},
					},
					Page:  1,
					Limit: 10,
				}
				mockAsset.EXPECT().
					Get("org123", "ledger123", 10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(expectedAssets, nil)
			},
			expectedResult: &mmodel.Assets{
				Items: []mmodel.Asset{
					{
						ID:   "asset123",
						Name: "Brazilian Real",
						Type: "currency",
						Code: "BRL",
					},
				},
				Page:  1,
				Limit: 10,
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			limit:          10,
			page:           1,
			sortOrder:      "asc",
			startDate:      "2025-01-01",
			endDate:        "2025-03-30",
			mockSetup: func() {
				mockAsset.EXPECT().
					Get("org123", "ledger123", 10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockAsset.Get(
				tc.organizationID,
				tc.ledgerID,
				tc.limit,
				tc.page,
				tc.sortOrder,
				tc.startDate,
				tc.endDate,
			)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestAsset_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAsset := repository.NewMockAsset(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		assetID        string
		mockSetup      func()
		expectedResult *mmodel.Asset
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			assetID:        "asset123",
			mockSetup: func() {
				expectedAsset := &mmodel.Asset{
					ID:   "asset123",
					Name: "Brazilian Real",
					Type: "currency",
					Code: "BRL",
				}
				mockAsset.EXPECT().
					GetByID("org123", "ledger123", "asset123").
					Return(expectedAsset, nil)
			},
			expectedResult: &mmodel.Asset{
				ID:   "asset123",
				Name: "Brazilian Real",
				Type: "currency",
				Code: "BRL",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			assetID:        "asset123",
			mockSetup: func() {
				mockAsset.EXPECT().
					GetByID("org123", "ledger123", "asset123").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockAsset.GetByID(tc.organizationID, tc.ledgerID, tc.assetID)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestAsset_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAsset := repository.NewMockAsset(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		assetID        string
		input          mmodel.UpdateAssetInput
		mockSetup      func()
		expectedResult *mmodel.Asset
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			assetID:        "asset123",
			input: mmodel.UpdateAssetInput{
				Name: "Updated Asset",
			},
			mockSetup: func() {
				expectedAsset := &mmodel.Asset{
					ID:   "asset123",
					Name: "Updated Asset",
					Type: "currency",
					Code: "BRL",
				}
				mockAsset.EXPECT().
					Update("org123", "ledger123", "asset123", gomock.Any()).
					Return(expectedAsset, nil)
			},
			expectedResult: &mmodel.Asset{
				ID:   "asset123",
				Name: "Updated Asset",
				Type: "currency",
				Code: "BRL",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			assetID:        "asset123",
			input: mmodel.UpdateAssetInput{
				Name: "Updated Asset",
			},
			mockSetup: func() {
				mockAsset.EXPECT().
					Update("org123", "ledger123", "asset123", gomock.Any()).
					Return(nil, errors.New("update failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("update failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockAsset.Update(tc.organizationID, tc.ledgerID, tc.assetID, tc.input)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestAsset_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAsset := repository.NewMockAsset(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		assetID        string
		mockSetup      func()
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			assetID:        "asset123",
			mockSetup: func() {
				mockAsset.EXPECT().
					Delete("org123", "ledger123", "asset123").
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			assetID:        "asset123",
			mockSetup: func() {
				mockAsset.EXPECT().
					Delete("org123", "ledger123", "asset123").
					Return(errors.New("deletion failed"))
			},
			expectedError: errors.New("deletion failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			err := mockAsset.Delete(tc.organizationID, tc.ledgerID, tc.assetID)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
