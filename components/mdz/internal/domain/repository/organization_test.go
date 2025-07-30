package repository_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestOrganizationInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganization := repository.NewMockOrganization(ctrl)

	// Test interface compliance by using the mock
	var _ repository.Organization = mockOrganization
}

func TestOrganization_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganization := repository.NewMockOrganization(ctrl)

	testCases := []struct {
		name           string
		input          mmodel.CreateOrganizationInput
		mockSetup      func()
		expectedResult *mmodel.Organization
		expectedError  error
	}{
		{
			name: "success",
			input: mmodel.CreateOrganizationInput{
				LegalName:     "Lerian Studio",
				LegalDocument: "00000000000000",
			},
			mockSetup: func() {
				expectedOrg := &mmodel.Organization{
					ID:            "org123",
					LegalName:     "Lerian Studio",
					LegalDocument: "00000000000000",
				}
				mockOrganization.EXPECT().
					Create(gomock.Any()).
					Return(expectedOrg, nil)
			},
			expectedResult: &mmodel.Organization{
				ID:            "org123",
				LegalName:     "Lerian Studio",
				LegalDocument: "00000000000000",
			},
			expectedError: nil,
		},
		{
			name: "error",
			input: mmodel.CreateOrganizationInput{
				LegalName:     "Lerian Studio",
				LegalDocument: "00000000000000",
			},
			mockSetup: func() {
				mockOrganization.EXPECT().
					Create(gomock.Any()).
					Return(nil, errors.New("creation failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("creation failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockOrganization.Create(tc.input)

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

func TestOrganization_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganization := repository.NewMockOrganization(ctrl)

	testCases := []struct {
		name           string
		limit          int
		page           int
		sortOrder      string
		startDate      string
		endDate        string
		mockSetup      func()
		expectedResult *mmodel.Organizations
		expectedError  error
	}{
		{
			name:      "success",
			limit:     10,
			page:      1,
			sortOrder: "asc",
			startDate: "2025-01-01",
			endDate:   "2025-03-30",
			mockSetup: func() {
				expectedOrgs := &mmodel.Organizations{
					Items: []mmodel.Organization{
						{
							ID:            "org123",
							LegalName:     "Lerian Studio",
							LegalDocument: "00000000000000",
						},
					},
					Page:  1,
					Limit: 10,
				}
				mockOrganization.EXPECT().
					Get(10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(expectedOrgs, nil)
			},
			expectedResult: &mmodel.Organizations{
				Items: []mmodel.Organization{
					{
						ID:            "org123",
						LegalName:     "Lerian Studio",
						LegalDocument: "00000000000000",
					},
				},
				Page:  1,
				Limit: 10,
			},
			expectedError: nil,
		},
		{
			name:      "error",
			limit:     10,
			page:      1,
			sortOrder: "asc",
			startDate: "2025-01-01",
			endDate:   "2025-03-30",
			mockSetup: func() {
				mockOrganization.EXPECT().
					Get(10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockOrganization.Get(
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

func TestOrganization_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganization := repository.NewMockOrganization(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		mockSetup      func()
		expectedResult *mmodel.Organization
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			mockSetup: func() {
				expectedOrg := &mmodel.Organization{
					ID:            "org123",
					LegalName:     "Lerian Studio",
					LegalDocument: "00000000000000",
				}
				mockOrganization.EXPECT().
					GetByID("org123").
					Return(expectedOrg, nil)
			},
			expectedResult: &mmodel.Organization{
				ID:            "org123",
				LegalName:     "Lerian Studio",
				LegalDocument: "00000000000000",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			mockSetup: func() {
				mockOrganization.EXPECT().
					GetByID("org123").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockOrganization.GetByID(tc.organizationID)

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

func TestOrganization_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganization := repository.NewMockOrganization(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		input          mmodel.UpdateOrganizationInput
		mockSetup      func()
		expectedResult *mmodel.Organization
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			input: mmodel.UpdateOrganizationInput{
				LegalName: "Updated Lerian Studio",
			},
			mockSetup: func() {
				expectedOrg := &mmodel.Organization{
					ID:            "org123",
					LegalName:     "Updated Lerian Studio",
					LegalDocument: "00000000000000",
				}
				mockOrganization.EXPECT().
					Update("org123", gomock.Any()).
					Return(expectedOrg, nil)
			},
			expectedResult: &mmodel.Organization{
				ID:            "org123",
				LegalName:     "Updated Lerian Studio",
				LegalDocument: "00000000000000",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			input: mmodel.UpdateOrganizationInput{
				LegalName: "Updated Lerian Studio",
			},
			mockSetup: func() {
				mockOrganization.EXPECT().
					Update("org123", gomock.Any()).
					Return(nil, errors.New("update failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("update failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			result, err := mockOrganization.Update(tc.organizationID, tc.input)

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

func TestOrganization_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganization := repository.NewMockOrganization(ctrl)

	testCases := []struct {
		name           string
		organizationID string
		mockSetup      func()
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			mockSetup: func() {
				mockOrganization.EXPECT().
					Delete("org123").
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			mockSetup: func() {
				mockOrganization.EXPECT().
					Delete("org123").
					Return(errors.New("deletion failed"))
			},
			expectedError: errors.New("deletion failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			err := mockOrganization.Delete(tc.organizationID)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
