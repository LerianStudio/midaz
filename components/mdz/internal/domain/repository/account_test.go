package repository_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestAccountInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccount := repository.NewMockAccount(ctrl)

	// Test interface compliance by using the mock
	var _ repository.Account = mockAccount
}

func TestAccount_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccount := repository.NewMockAccount(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		input          mmodel.CreateAccountInput
		mockSetup      func()
		expectedResult *mmodel.Account
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreateAccountInput{
				Name:      "Test Account",
				AssetCode: "USD",
				Type:      "checking",
			},
			mockSetup: func() {
				expectedAccount := &mmodel.Account{
					ID:        "acc123",
					Name:      "Test Account",
					AssetCode: "USD",
					Type:      "checking",
				}
				mockAccount.EXPECT().
					Create("org123", "ledger123", gomock.Any()).
					Return(expectedAccount, nil)
			},
			expectedResult: &mmodel.Account{
				ID:        "acc123",
				Name:      "Test Account",
				AssetCode: "USD",
				Type:      "checking",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreateAccountInput{
				Name:      "Test Account",
				AssetCode: "USD",
				Type:      "checking",
			},
			mockSetup: func() {
				mockAccount.EXPECT().
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

			result, err := mockAccount.Create(tc.organizationID, tc.ledgerID, tc.input)

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

func TestAccount_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccount := repository.NewMockAccount(ctrl)

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
		expectedResult *mmodel.Accounts
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
				expectedAccounts := &mmodel.Accounts{
					Items: []mmodel.Account{
						{
							ID:        "acc123",
							Name:      "Test Account",
							AssetCode: "USD",
							Type:      "checking",
						},
					},
					Page:  1,
					Limit: 10,
				}
				mockAccount.EXPECT().
					Get("org123", "ledger123", 10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(expectedAccounts, nil)
			},
			expectedResult: &mmodel.Accounts{
				Items: []mmodel.Account{
					{
						ID:        "acc123",
						Name:      "Test Account",
						AssetCode: "USD",
						Type:      "checking",
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
				mockAccount.EXPECT().
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

			result, err := mockAccount.Get(
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

func TestAccount_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccount := repository.NewMockAccount(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		accountID      string
		mockSetup      func()
		expectedResult *mmodel.Account
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			accountID:      "acc123",
			mockSetup: func() {
				expectedAccount := &mmodel.Account{
					ID:        "acc123",
					Name:      "Test Account",
					AssetCode: "USD",
					Type:      "checking",
				}
				mockAccount.EXPECT().
					GetByID("org123", "ledger123", "acc123").
					Return(expectedAccount, nil)
			},
			expectedResult: &mmodel.Account{
				ID:        "acc123",
				Name:      "Test Account",
				AssetCode: "USD",
				Type:      "checking",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			accountID:      "acc123",
			mockSetup: func() {
				mockAccount.EXPECT().
					GetByID("org123", "ledger123", "acc123").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockAccount.GetByID(tc.organizationID, tc.ledgerID, tc.accountID)

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

func TestAccount_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccount := repository.NewMockAccount(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		accountID      string
		input          mmodel.UpdateAccountInput
		mockSetup      func()
		expectedResult *mmodel.Account
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			accountID:      "acc123",
			input: mmodel.UpdateAccountInput{
				Name: "Updated Account",
			},
			mockSetup: func() {
				expectedAccount := &mmodel.Account{
					ID:        "acc123",
					Name:      "Updated Account",
					AssetCode: "USD",
					Type:      "checking",
				}
				mockAccount.EXPECT().
					Update("org123", "ledger123", "acc123", gomock.Any()).
					Return(expectedAccount, nil)
			},
			expectedResult: &mmodel.Account{
				ID:        "acc123",
				Name:      "Updated Account",
				AssetCode: "USD",
				Type:      "checking",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			accountID:      "acc123",
			input: mmodel.UpdateAccountInput{
				Name: "Updated Account",
			},
			mockSetup: func() {
				mockAccount.EXPECT().
					Update("org123", "ledger123", "acc123", gomock.Any()).
					Return(nil, errors.New("update failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("update failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockAccount.Update(tc.organizationID, tc.ledgerID, tc.accountID, tc.input)

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

func TestAccount_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccount := repository.NewMockAccount(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		accountID      string
		mockSetup      func()
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			accountID:      "acc123",
			mockSetup: func() {
				mockAccount.EXPECT().
					Delete("org123", "ledger123", "acc123").
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			accountID:      "acc123",
			mockSetup: func() {
				mockAccount.EXPECT().
					Delete("org123", "ledger123", "acc123").
					Return(errors.New("deletion failed"))
			},
			expectedError: errors.New("deletion failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			err := mockAccount.Delete(tc.organizationID, tc.ledgerID, tc.accountID)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
