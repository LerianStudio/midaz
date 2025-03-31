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

func Test_transaction_Create(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	transactionID := "01933f96-ed04-7c57-be5b-c091388830f8"
	parentTransactionID := "01933f94-67b1-794c-bb13-6b75aed7591a"

	description := "Test Transaction"
	template := "transfer"
	amount := int64(1000)
	amountScale := int64(2)
	assetCode := "USD"
	chartOfAccountsGroupName := "group1"
	source := []string{"account1", "account2"}
	destination := []string{"account3", "account4"}
	statusCode := "COMPLETED"
	statusDescription := ptr.StringPtr("Transaction completed successfully")

	metadata := map[string]any{
		"reference":   "INV-001",
		"category":    "payment",
		"isRecurring": true,
	}

	input := mmodel.CreateTransactionInput{
		Description:              description,
		Template:                 template,
		Amount:                   &amount,
		AmountScale:              &amountScale,
		AssetCode:                assetCode,
		ChartOfAccountsGroupName: chartOfAccountsGroupName,
		Source:                   source,
		Destination:              destination,
		ParentTransactionID:      &parentTransactionID,
		Status: &mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Transaction{
		ID:                       transactionID,
		Description:              description,
		Template:                 template,
		Amount:                   &amount,
		AmountScale:              &amountScale,
		AssetCode:                assetCode,
		ChartOfAccountsGroupName: chartOfAccountsGroupName,
		Source:                   source,
		Destination:              destination,
		ParentTransactionID:      &parentTransactionID,
		Status: &mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		CreatedAt: time.Date(2024, 11, 18, 14, 04, 35, 972791193, time.UTC),
		UpdatedAt: time.Date(2024, 11, 18, 14, 04, 35, 972794057, time.UTC),
		Metadata:  metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions",
		URIAPITransaction, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/transaction_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	transaction := NewTransaction(factory)

	result, err := transaction.Create(organizationID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Description, result.Description)
	assert.Equal(t, expectedResult.Template, result.Template)
	assert.Equal(t, *expectedResult.Amount, *result.Amount)
	assert.Equal(t, *expectedResult.AmountScale, *result.AmountScale)
	assert.Equal(t, expectedResult.AssetCode, result.AssetCode)
	assert.Equal(t, expectedResult.ChartOfAccountsGroupName, result.ChartOfAccountsGroupName)
	assert.Equal(t, expectedResult.Source, result.Source)
	assert.Equal(t, expectedResult.Destination, result.Destination)
	assert.Equal(t, *expectedResult.ParentTransactionID, *result.ParentTransactionID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, *expectedResult.Status.Description, *result.Status.Description)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST "+uri])
}

func Test_transaction_CreateDSL(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	transactionID := "01933f96-ed04-7c57-be5b-c091388830f8"

	dslContent := `
	transaction {
		description = "Test Transaction"
		template = "transfer"
		source = ["account1", "account2"]
		destination = ["account3", "account4"]
		amount = 1000
		amountScale = 2
		assetCode = "USD"
		metadata = {
			reference = "INV-001"
			category = "payment"
			isRecurring = true
		}
	}
	`

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/dsl",
		URIAPITransaction, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/transaction_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	transaction := NewTransaction(factory)

	result, err := transaction.CreateDSL(organizationID, ledgerID, dslContent)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, transactionID, result.ID)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST "+uri])
}

func Test_transaction_Get(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	limit := 10
	page := 1
	sortOrder := "desc"
	startDate := "2024-01-01"
	endDate := "2024-12-31"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions",
		URIAPITransaction, organizationID, ledgerID)

	// Use BuildPaginatedURL to ensure the URL is constructed correctly
	uri, err := BuildPaginatedURL(baseURL, limit, page, sortOrder, startDate, endDate)
	assert.NoError(t, err)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/transaction_response_get.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	transaction := NewTransaction(factory)

	result, err := transaction.Get(organizationID, ledgerID, limit, page, sortOrder, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Items))
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f8", result.Items[0].ID)
	assert.NotNil(t, result.Pagination.NextCursor)
	assert.Equal(t, "next_page_token", *result.Pagination.NextCursor)
	assert.NotNil(t, result.Pagination.PrevCursor)
	assert.Equal(t, "prev_page_token", *result.Pagination.PrevCursor)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_transaction_GetByID(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	transactionID := "01933f96-ed04-7c57-be5b-c091388830f8"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s",
		URIAPITransaction, organizationID, ledgerID, transactionID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/transaction_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	transaction := NewTransaction(factory)

	result, err := transaction.GetByID(organizationID, ledgerID, transactionID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, transactionID, result.ID)
	assert.Equal(t, "Test Transaction", result.Description)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_transaction_Revert(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	transactionID := "01933f96-ed04-7c57-be5b-c091388830f8"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s/revert",
		URIAPITransaction, organizationID, ledgerID, transactionID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/transaction_response_revert.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	transaction := NewTransaction(factory)

	result, err := transaction.Revert(organizationID, ledgerID, transactionID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, transactionID, result.ID)
	assert.Equal(t, "Revert: Test Transaction", result.Description)
	assert.Equal(t, "revert", result.Template)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST "+uri])
}
