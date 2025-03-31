package repository_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestLedgerInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedger := repository.NewMockLedger(ctrl)
	
	// Test interface compliance by using the mock
	var _ repository.Ledger = mockLedger
}

func TestLedger_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedger := repository.NewMockLedger(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		input          mmodel.CreateLedgerInput
		mockSetup      func()
		expectedResult *mmodel.Ledger
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			input: mmodel.CreateLedgerInput{
				Name: "Test Ledger",
			},
			mockSetup: func() {
				expectedLedger := &mmodel.Ledger{
					ID:             "ledger123",
					Name:           "Test Ledger",
					OrganizationID: "org123",
				}
				mockLedger.EXPECT().
					Create("org123", gomock.Any()).
					Return(expectedLedger, nil)
			},
			expectedResult: &mmodel.Ledger{
				ID:             "ledger123",
				Name:           "Test Ledger",
				OrganizationID: "org123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			input: mmodel.CreateLedgerInput{
				Name: "Test Ledger",
			},
			mockSetup: func() {
				mockLedger.EXPECT().
					Create("org123", gomock.Any()).
					Return(nil, errors.New("creation failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("creation failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockLedger.Create(tc.organizationID, tc.input)
			
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

func TestLedger_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedger := repository.NewMockLedger(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		limit          int
		page           int
		sortOrder      string
		startDate      string
		endDate        string
		mockSetup      func()
		expectedResult *mmodel.Ledgers
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			limit:          10,
			page:           1,
			sortOrder:      "asc",
			startDate:      "2025-01-01",
			endDate:        "2025-03-30",
			mockSetup: func() {
				expectedLedgers := &mmodel.Ledgers{
					Items: []mmodel.Ledger{
						{
							ID:             "ledger123",
							Name:           "Test Ledger",
							OrganizationID: "org123",
						},
					},
					Page:  1,
					Limit: 10,
				}
				mockLedger.EXPECT().
					Get("org123", 10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(expectedLedgers, nil)
			},
			expectedResult: &mmodel.Ledgers{
				Items: []mmodel.Ledger{
					{
						ID:             "ledger123",
						Name:           "Test Ledger",
						OrganizationID: "org123",
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
			limit:          10,
			page:           1,
			sortOrder:      "asc",
			startDate:      "2025-01-01",
			endDate:        "2025-03-30",
			mockSetup: func() {
				mockLedger.EXPECT().
					Get("org123", 10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockLedger.Get(
				tc.organizationID, 
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

func TestLedger_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedger := repository.NewMockLedger(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		mockSetup      func()
		expectedResult *mmodel.Ledger
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			mockSetup: func() {
				expectedLedger := &mmodel.Ledger{
					ID:             "ledger123",
					Name:           "Test Ledger",
					OrganizationID: "org123",
				}
				mockLedger.EXPECT().
					GetByID("org123", "ledger123").
					Return(expectedLedger, nil)
			},
			expectedResult: &mmodel.Ledger{
				ID:             "ledger123",
				Name:           "Test Ledger",
				OrganizationID: "org123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			mockSetup: func() {
				mockLedger.EXPECT().
					GetByID("org123", "ledger123").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockLedger.GetByID(tc.organizationID, tc.ledgerID)
			
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

func TestLedger_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedger := repository.NewMockLedger(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		input          mmodel.UpdateLedgerInput
		mockSetup      func()
		expectedResult *mmodel.Ledger
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.UpdateLedgerInput{
				Name: "Updated Ledger",
			},
			mockSetup: func() {
				expectedLedger := &mmodel.Ledger{
					ID:             "ledger123",
					Name:           "Updated Ledger",
					OrganizationID: "org123",
				}
				mockLedger.EXPECT().
					Update("org123", "ledger123", gomock.Any()).
					Return(expectedLedger, nil)
			},
			expectedResult: &mmodel.Ledger{
				ID:             "ledger123",
				Name:           "Updated Ledger",
				OrganizationID: "org123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.UpdateLedgerInput{
				Name: "Updated Ledger",
			},
			mockSetup: func() {
				mockLedger.EXPECT().
					Update("org123", "ledger123", gomock.Any()).
					Return(nil, errors.New("update failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("update failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockLedger.Update(tc.organizationID, tc.ledgerID, tc.input)
			
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

func TestLedger_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedger := repository.NewMockLedger(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		mockSetup      func()
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			mockSetup: func() {
				mockLedger.EXPECT().
					Delete("org123", "ledger123").
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			mockSetup: func() {
				mockLedger.EXPECT().
					Delete("org123", "ledger123").
					Return(errors.New("deletion failed"))
			},
			expectedError: errors.New("deletion failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			err := mockLedger.Delete(tc.organizationID, tc.ledgerID)
			
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
