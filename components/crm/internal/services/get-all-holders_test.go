// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllHolders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := holder.NewMockRepository(ctrl)

	holderID1 := libCommons.GenerateUUIDv7()
	name1 := "John Smith"
	document1 := "90217469051"

	holderID2 := libCommons.GenerateUUIDv7()
	name2 := "Alice Johnson"
	document2 := "12345678901"

	holderID3 := libCommons.GenerateUUIDv7()
	name3 := "Bob Martin"
	document3 := "98765432109"
	externalID3 := "G4K7N8M"

	uc := &UseCase{
		HolderRepo: mockRepo,
	}

	query := http.QueryHeader{Limit: 10, Page: 1}
	queryWithDocument := http.QueryHeader{Limit: 10, Page: 1, Document: &document1}
	queryWithExternalId := http.QueryHeader{Limit: 10, Page: 1, ExternalID: &externalID3}

	testCases := []struct {
		name           string
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Holder
	}{
		{
			name:   "Success get all holders",
			filter: query,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), query, false).
					Return([]*mmodel.Holder{
						{ID: &holderID1, Name: &name1, Document: &document1},
						{ID: &holderID2, Name: &name2, Document: &document2},
						{ID: &holderID3, Name: &name3, Document: &document3},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Holder{
				{ID: &holderID1, Name: &name1, Document: &document1},
				{ID: &holderID2, Name: &name2, Document: &document2},
				{ID: &holderID3, Name: &name3, Document: &document3},
			},
		},
		{
			name:   "Success get all holders with document filter",
			filter: queryWithDocument,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), queryWithDocument, false).
					Return([]*mmodel.Holder{
						{ID: &holderID1, Name: &name1, Document: &document1},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Holder{
				{ID: &holderID1, Name: &name1, Document: &document1},
			},
		},
		{
			name:   "Success get all holders with external id filter",
			filter: queryWithExternalId,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), queryWithExternalId, false).
					Return([]*mmodel.Holder{
						{ID: &holderID3, Name: &name3, Document: &document3, ExternalID: &externalID3},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Holder{
				{ID: &holderID3, Name: &name3, Document: &document3, ExternalID: &externalID3},
			},
		},
		{
			name:   "Success returning empty array when no holders found",
			filter: query,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), query, gomock.Any()).
					Return([]*mmodel.Holder{}, nil)
			},
			expectErr:      false,
			expectedResult: []*mmodel.Holder{},
		},
		{
			name:   "Error when repository fails to find all holders",
			filter: query,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), query, false).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			holders, err := uc.GetAllHolders(ctx, uuid.New().String(), testCase.filter, false)

			if testCase.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, testCase.expectedResult, holders)
			}
		})
	}
}
