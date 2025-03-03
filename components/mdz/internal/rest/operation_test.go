package rest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_operation_Get(t *testing.T) {
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"

	limit := 2
	page := 1

	expectedResult := mmodel.Operations{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Operation{
			{
				ID:             "01932167-d43f-8g8c-d2ge-f7h848e64c94",
				TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
				AccountID:      "01932159-f4bd-7e0a-971e-52cc6e528312",
				AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
				Amount:         "500.00",
				Type:           "DEBIT",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Status: mmodel.OperationStatus{
					Code:        "COMPLETED",
					Description: ptr.StringPtr("Operation completed successfully"),
				},
				CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
			{
				ID:             "01932168-e54g-9h9d-e3hf-g8i959f75d05",
				TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
				AccountID:      "01932160-g5ce-7f1b-982f-63dd7f639423",
				AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
				Amount:         "500.00",
				Type:           "CREDIT",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Status: mmodel.OperationStatus{
					Code:        "COMPLETED",
					Description: ptr.StringPtr("Operation completed successfully"),
				},
				CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/operation_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.Get(organizationID, ledgerID, limit, page, "", "", "")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].TransactionID, v.TransactionID)
		assert.Equal(t, expectedResult.Items[i].AccountID, v.AccountID)
		assert.Equal(t, expectedResult.Items[i].AssetID, v.AssetID)
		assert.Equal(t, expectedResult.Items[i].Amount, v.Amount)
		assert.Equal(t, expectedResult.Items[i].Type, v.Type)
		assert.Equal(t, expectedResult.Items[i].Status.Code, v.Status.Code)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_operation_GetByID(t *testing.T) {
	operationID := "01932167-d43f-8g8c-d2ge-f7h848e64c94"
	transactionID := "01932161-h6df-8g2c-b83g-74ee8g7405f4"
	accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
	assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	amount := "500.00"
	operationType := "DEBIT"

	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &mmodel.Operation{
		ID:             operationID,
		TransactionID:  transactionID,
		AccountID:      accountID,
		AssetID:        assetID,
		Amount:         amount,
		Type:           operationType,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status: mmodel.OperationStatus{
			Code:        "COMPLETED",
			Description: ptr.StringPtr("Operation completed successfully"),
		},
		CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations/%s",
		URIAPILedger, organizationID, ledgerID, operationID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/operation_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.GetByID(organizationID, ledgerID, operationID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.TransactionID, result.TransactionID)
	assert.Equal(t, expectedResult.AccountID, result.AccountID)
	assert.Equal(t, expectedResult.AssetID, result.AssetID)
	assert.Equal(t, expectedResult.Amount, result.Amount)
	assert.Equal(t, expectedResult.Type, result.Type)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, *expectedResult.Status.Description, *result.Status.Description)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_operation_GetByAccount(t *testing.T) {
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"

	limit := 2
	page := 1

	expectedResult := mmodel.Operations{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Operation{
			{
				ID:             "01932167-d43f-8g8c-d2ge-f7h848e64c94",
				TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
				AccountID:      accountID,
				AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
				Amount:         "500.00",
				Type:           "DEBIT",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Status: mmodel.OperationStatus{
					Code:        "COMPLETED",
					Description: ptr.StringPtr("Operation completed successfully"),
				},
				CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
			{
				ID:             "01932169-f65h-0i0e-f4ig-h9j060g86e16",
				TransactionID:  "01932162-i7eg-9h3d-c94h-85ff9h8516g5",
				AccountID:      accountID,
				AssetID:        "01930365-4d46-7a09-a503-b932714f85af",
				Amount:         "200.50",
				Type:           "CREDIT",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Status: mmodel.OperationStatus{
					Code:        "COMPLETED",
					Description: ptr.StringPtr("Operation completed successfully"),
				},
				CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, accountID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/operation_response_by_account.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.GetByAccount(organizationID, ledgerID, accountID, limit, page, "", "", "")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].TransactionID, v.TransactionID)
		assert.Equal(t, expectedResult.Items[i].AccountID, v.AccountID)
		assert.Equal(t, expectedResult.Items[i].AssetID, v.AssetID)
		assert.Equal(t, expectedResult.Items[i].Amount, v.Amount)
		assert.Equal(t, expectedResult.Items[i].Type, v.Type)
		assert.Equal(t, expectedResult.Items[i].Status.Code, v.Status.Code)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_operation_GetByAccountAndID(t *testing.T) {
	operationID := "01932167-d43f-8g8c-d2ge-f7h848e64c94"
	transactionID := "01932161-h6df-8g2c-b83g-74ee8g7405f4"
	accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
	assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	amount := "500.00"
	operationType := "DEBIT"

	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &mmodel.Operation{
		ID:             operationID,
		TransactionID:  transactionID,
		AccountID:      accountID,
		AssetID:        assetID,
		Amount:         amount,
		Type:           operationType,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status: mmodel.OperationStatus{
			Code:        "COMPLETED",
			Description: ptr.StringPtr("Operation completed successfully"),
		},
		CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/operations/%s",
		URIAPILedger, organizationID, ledgerID, accountID, operationID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/operation_response_get_by_account_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.GetByAccountAndID(organizationID, ledgerID, accountID, operationID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.TransactionID, result.TransactionID)
	assert.Equal(t, expectedResult.AccountID, result.AccountID)
	assert.Equal(t, expectedResult.AssetID, result.AssetID)
	assert.Equal(t, expectedResult.Amount, result.Amount)
	assert.Equal(t, expectedResult.Type, result.Type)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, *expectedResult.Status.Description, *result.Status.Description)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_operation_Update(t *testing.T) {
	operationID := "01932167-d43f-8g8c-d2ge-f7h848e64c94"
	transactionID := "01932161-h6df-8g2c-b83g-74ee8g7405f4"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	
	statusCode := "CANCELED"
	statusDescription := "Operation was canceled"

	inp := mmodel.UpdateOperationInput{
		Status: mmodel.OperationStatus{
			Code:        statusCode,
			Description: ptr.StringPtr(statusDescription),
		},
	}

	expectedResult := &mmodel.Operation{
		ID:            operationID,
		TransactionID: transactionID,
		Status: mmodel.OperationStatus{
			Code:        statusCode,
			Description: ptr.StringPtr(statusDescription),
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s/operations/%s",
		URIAPILedger, organizationID, ledgerID, transactionID, operationID)

	httpmock.RegisterResponder(http.MethodPatch, uri,
		mockutil.MockResponseFromFile(http.StatusOK,
			"./.fixtures/operation_response_update.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.Update(organizationID, ledgerID, transactionID, operationID, inp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.TransactionID, result.TransactionID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, *expectedResult.Status.Description, *result.Status.Description)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["PATCH "+uri])
}