package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllAliases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAliasRepo := alias.NewMockRepository(ctrl)

	holderID := libCommons.GenerateUUIDv7()

	id1 := libCommons.GenerateUUIDv7()
	id2 := libCommons.GenerateUUIDv7()
	accountId := libCommons.GenerateUUIDv7().String()
	ledgerId := libCommons.GenerateUUIDv7().String()
	document := "98765432109"
	account := "123450"
	iban := "US12345678901234567810"
	branch := "0001"

	uc := &UseCase{
		AliasRepo: mockAliasRepo,
	}

	query := http.QueryHeader{Limit: 10, Page: 1}
	queryWithDocument := http.QueryHeader{Limit: 10, Page: 1, Document: &document}
	queryWithAccountId := http.QueryHeader{Limit: 10, Page: 1, AccountID: &accountId}
	queryWithLedgerId := http.QueryHeader{Limit: 10, Page: 1, LedgerID: &ledgerId}
	queryWithbankingDetailsAccount := http.QueryHeader{Limit: 10, Page: 1, BankingDetailsAccount: &account}
	queryWithbankingDetailsIban := http.QueryHeader{Limit: 10, Page: 1, ExternalID: &iban}
	queryWithbankingDetailsBranch := http.QueryHeader{Limit: 10, Page: 1, ExternalID: &branch}

	testCases := []struct {
		name           string
		holderId       uuid.UUID
		filter         http.QueryHeader
		mockSetup      func()
		expectedErr    error
		expectedResult []*mmodel.Alias
	}{
		{
			name:     "Success get all aliases",
			holderId: holderID,
			filter:   query,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{
						{ID: &id1},
						{ID: &id2},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: []*mmodel.Alias{
				{ID: &id1},
				{ID: &id2},
			},
		},
		{
			name:     "Success get all aliases with filter by document",
			holderId: holderID,
			filter:   queryWithDocument,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{
						{ID: &id1, Document: &document},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: []*mmodel.Alias{
				{ID: &id1, Document: &document},
			},
		},
		{
			name:     "Success get all aliases with filter by accountID",
			holderId: holderID,
			filter:   queryWithAccountId,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{
						{ID: &id1, AccountID: &accountId},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: []*mmodel.Alias{
				{ID: &id1, AccountID: &accountId},
			},
		},
		{
			name:     "Success get all aliases with filter by ledgerID",
			holderId: holderID,
			filter:   queryWithLedgerId,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{
						{ID: &id1, LedgerID: &ledgerId},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: []*mmodel.Alias{
				{ID: &id1, LedgerID: &ledgerId},
			},
		},
		{
			name:     "Success get all aliases with filter by banking details account",
			holderId: holderID,
			filter:   queryWithbankingDetailsAccount,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{
						{ID: &id1, BankingDetails: &mmodel.BankingDetails{Account: &account}},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: []*mmodel.Alias{
				{ID: &id1, BankingDetails: &mmodel.BankingDetails{Account: &account}},
			},
		},
		{
			name:     "Success get all aliases with filter by banking details iban",
			holderId: holderID,
			filter:   queryWithbankingDetailsIban,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{
						{ID: &id1, BankingDetails: &mmodel.BankingDetails{IBAN: &iban}},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: []*mmodel.Alias{
				{ID: &id1, BankingDetails: &mmodel.BankingDetails{IBAN: &iban}},
			},
		},
		{
			name:     "Success get all aliases with filter by banking details branch",
			holderId: holderID,
			filter:   queryWithbankingDetailsBranch,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{
						{ID: &id1, BankingDetails: &mmodel.BankingDetails{Branch: &branch}},
					}, nil)
			},
			expectedErr: nil,
			expectedResult: []*mmodel.Alias{
				{ID: &id1, BankingDetails: &mmodel.BankingDetails{Branch: &branch}},
			},
		},
		{
			name:     "Success returning empty array when no aliases found",
			holderId: holderID,
			filter:   query,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.Alias{}, nil)
			},
			expectedErr:    nil,
			expectedResult: []*mmodel.Alias{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			accounts, err := uc.GetAllAliases(ctx, uuid.New().String(), testCase.holderId, query, false)

			if testCase.expectedErr != nil {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, testCase.expectedResult, accounts)
			}
		})
	}
}
