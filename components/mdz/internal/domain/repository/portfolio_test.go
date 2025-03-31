package repository_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestPortfolioInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolio := repository.NewMockPortfolio(ctrl)

	// Test interface compliance by using the mock
	var _ repository.Portfolio = mockPortfolio
}

func TestPortfolio_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolio := repository.NewMockPortfolio(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		input          mmodel.CreatePortfolioInput
		mockSetup      func()
		expectedResult *mmodel.Portfolio
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreatePortfolioInput{
				Name:     "Test Portfolio",
				EntityID: "entity123",
			},
			mockSetup: func() {
				expectedPortfolio := &mmodel.Portfolio{
					ID:             "portfolio123",
					Name:           "Test Portfolio",
					EntityID:       "entity123",
					OrganizationID: "org123",
					LedgerID:       "ledger123",
				}
				mockPortfolio.EXPECT().
					Create("org123", "ledger123", gomock.Any()).
					Return(expectedPortfolio, nil)
			},
			expectedResult: &mmodel.Portfolio{
				ID:             "portfolio123",
				Name:           "Test Portfolio",
				EntityID:       "entity123",
				OrganizationID: "org123",
				LedgerID:       "ledger123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreatePortfolioInput{
				Name:     "Test Portfolio",
				EntityID: "entity123",
			},
			mockSetup: func() {
				mockPortfolio.EXPECT().
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

			result, err := mockPortfolio.Create(tc.organizationID, tc.ledgerID, tc.input)

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

func TestPortfolio_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolio := repository.NewMockPortfolio(ctrl)

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
		expectedResult *mmodel.Portfolios
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
				expectedPortfolios := &mmodel.Portfolios{
					Items: []mmodel.Portfolio{
						{
							ID:             "portfolio123",
							Name:           "Test Portfolio",
							EntityID:       "entity123",
							OrganizationID: "org123",
							LedgerID:       "ledger123",
						},
					},
					Page:  1,
					Limit: 10,
				}
				mockPortfolio.EXPECT().
					Get("org123", "ledger123", 10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(expectedPortfolios, nil)
			},
			expectedResult: &mmodel.Portfolios{
				Items: []mmodel.Portfolio{
					{
						ID:             "portfolio123",
						Name:           "Test Portfolio",
						EntityID:       "entity123",
						OrganizationID: "org123",
						LedgerID:       "ledger123",
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
				mockPortfolio.EXPECT().
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

			result, err := mockPortfolio.Get(
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

func TestPortfolio_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolio := repository.NewMockPortfolio(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		mockSetup      func()
		expectedResult *mmodel.Portfolio
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			mockSetup: func() {
				expectedPortfolio := &mmodel.Portfolio{
					ID:             "portfolio123",
					Name:           "Test Portfolio",
					EntityID:       "entity123",
					OrganizationID: "org123",
					LedgerID:       "ledger123",
				}
				mockPortfolio.EXPECT().
					GetByID("org123", "ledger123", "portfolio123").
					Return(expectedPortfolio, nil)
			},
			expectedResult: &mmodel.Portfolio{
				ID:             "portfolio123",
				Name:           "Test Portfolio",
				EntityID:       "entity123",
				OrganizationID: "org123",
				LedgerID:       "ledger123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			mockSetup: func() {
				mockPortfolio.EXPECT().
					GetByID("org123", "ledger123", "portfolio123").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockPortfolio.GetByID(tc.organizationID, tc.ledgerID, tc.portfolioID)

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

func TestPortfolio_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolio := repository.NewMockPortfolio(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		input          mmodel.UpdatePortfolioInput
		mockSetup      func()
		expectedResult *mmodel.Portfolio
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			input: mmodel.UpdatePortfolioInput{
				Name: "Updated Portfolio",
			},
			mockSetup: func() {
				expectedPortfolio := &mmodel.Portfolio{
					ID:             "portfolio123",
					Name:           "Updated Portfolio",
					EntityID:       "entity123",
					OrganizationID: "org123",
					LedgerID:       "ledger123",
				}
				mockPortfolio.EXPECT().
					Update("org123", "ledger123", "portfolio123", gomock.Any()).
					Return(expectedPortfolio, nil)
			},
			expectedResult: &mmodel.Portfolio{
				ID:             "portfolio123",
				Name:           "Updated Portfolio",
				EntityID:       "entity123",
				OrganizationID: "org123",
				LedgerID:       "ledger123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			input: mmodel.UpdatePortfolioInput{
				Name: "Updated Portfolio",
			},
			mockSetup: func() {
				mockPortfolio.EXPECT().
					Update("org123", "ledger123", "portfolio123", gomock.Any()).
					Return(nil, errors.New("update failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("update failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockPortfolio.Update(tc.organizationID, tc.ledgerID, tc.portfolioID, tc.input)

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

func TestPortfolio_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPortfolio := repository.NewMockPortfolio(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		mockSetup      func()
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			mockSetup: func() {
				mockPortfolio.EXPECT().
					Delete("org123", "ledger123", "portfolio123").
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			mockSetup: func() {
				mockPortfolio.EXPECT().
					Delete("org123", "ledger123", "portfolio123").
					Return(errors.New("deletion failed"))
			},
			expectedError: errors.New("deletion failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			err := mockPortfolio.Delete(tc.organizationID, tc.ledgerID, tc.portfolioID)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
