package repository_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestSegmentInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegment := repository.NewMockSegment(ctrl)
	
	// Test interface compliance by using the mock
	var _ repository.Segment = mockSegment
}

func TestSegment_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegment := repository.NewMockSegment(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		input          mmodel.CreateSegmentInput
		mockSetup      func()
		expectedResult *mmodel.Segment
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreateSegmentInput{
				Name: "Test Segment",
			},
			mockSetup: func() {
				expectedSegment := &mmodel.Segment{
					ID:             "segment123",
					Name:           "Test Segment",
					OrganizationID: "org123",
					LedgerID:       "ledger123",
				}
				mockSegment.EXPECT().
					Create("org123", "ledger123", gomock.Any()).
					Return(expectedSegment, nil)
			},
			expectedResult: &mmodel.Segment{
				ID:             "segment123",
				Name:           "Test Segment",
				OrganizationID: "org123",
				LedgerID:       "ledger123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			input: mmodel.CreateSegmentInput{
				Name: "Test Segment",
			},
			mockSetup: func() {
				mockSegment.EXPECT().
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
			
			result, err := mockSegment.Create(tc.organizationID, tc.ledgerID, tc.input)
			
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

func TestSegment_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegment := repository.NewMockSegment(ctrl)
	
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
		expectedResult *mmodel.Segments
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
				expectedSegments := &mmodel.Segments{
					Items: []mmodel.Segment{
						{
							ID:             "segment123",
							Name:           "Test Segment",
							OrganizationID: "org123",
							LedgerID:       "ledger123",
						},
					},
					Page:  1,
					Limit: 10,
				}
				mockSegment.EXPECT().
					Get("org123", "ledger123", 10, 1, "asc", "2025-01-01", "2025-03-30").
					Return(expectedSegments, nil)
			},
			expectedResult: &mmodel.Segments{
				Items: []mmodel.Segment{
					{
						ID:             "segment123",
						Name:           "Test Segment",
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
				mockSegment.EXPECT().
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
			
			result, err := mockSegment.Get(
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

func TestSegment_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegment := repository.NewMockSegment(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		segmentID      string
		mockSetup      func()
		expectedResult *mmodel.Segment
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			segmentID:      "segment123",
			mockSetup: func() {
				expectedSegment := &mmodel.Segment{
					ID:             "segment123",
					Name:           "Test Segment",
					OrganizationID: "org123",
					LedgerID:       "ledger123",
				}
				mockSegment.EXPECT().
					GetByID("org123", "ledger123", "segment123").
					Return(expectedSegment, nil)
			},
			expectedResult: &mmodel.Segment{
				ID:             "segment123",
				Name:           "Test Segment",
				OrganizationID: "org123",
				LedgerID:       "ledger123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			segmentID:      "segment123",
			mockSetup: func() {
				mockSegment.EXPECT().
					GetByID("org123", "ledger123", "segment123").
					Return(nil, errors.New("retrieval failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("retrieval failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockSegment.GetByID(tc.organizationID, tc.ledgerID, tc.segmentID)
			
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

func TestSegment_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegment := repository.NewMockSegment(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		segmentID      string
		input          mmodel.UpdateSegmentInput
		mockSetup      func()
		expectedResult *mmodel.Segment
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			segmentID:      "segment123",
			input: mmodel.UpdateSegmentInput{
				Name: "Updated Segment",
			},
			mockSetup: func() {
				expectedSegment := &mmodel.Segment{
					ID:             "segment123",
					Name:           "Updated Segment",
					OrganizationID: "org123",
					LedgerID:       "ledger123",
				}
				mockSegment.EXPECT().
					Update("org123", "ledger123", "segment123", gomock.Any()).
					Return(expectedSegment, nil)
			},
			expectedResult: &mmodel.Segment{
				ID:             "segment123",
				Name:           "Updated Segment",
				OrganizationID: "org123",
				LedgerID:       "ledger123",
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			segmentID:      "segment123",
			input: mmodel.UpdateSegmentInput{
				Name: "Updated Segment",
			},
			mockSetup: func() {
				mockSegment.EXPECT().
					Update("org123", "ledger123", "segment123", gomock.Any()).
					Return(nil, errors.New("update failed"))
			},
			expectedResult: nil,
			expectedError:  errors.New("update failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			result, err := mockSegment.Update(tc.organizationID, tc.ledgerID, tc.segmentID, tc.input)
			
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

func TestSegment_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegment := repository.NewMockSegment(ctrl)
	
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		segmentID      string
		mockSetup      func()
		expectedError  error
	}{
		{
			name:           "success",
			organizationID: "org123",
			ledgerID:       "ledger123",
			segmentID:      "segment123",
			mockSetup: func() {
				mockSegment.EXPECT().
					Delete("org123", "ledger123", "segment123").
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			segmentID:      "segment123",
			mockSetup: func() {
				mockSegment.EXPECT().
					Delete("org123", "ledger123", "segment123").
					Return(errors.New("deletion failed"))
			},
			expectedError: errors.New("deletion failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			
			err := mockSegment.Delete(tc.organizationID, tc.ledgerID, tc.segmentID)
			
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
