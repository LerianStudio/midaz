package rest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_product_Create(t *testing.T) {
	productID := "0193271b-877f-7c98-a5a6-43b664d68982"
	ledgerID := "01932715-9f93-7432-90c3-4352bcfe464d"
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"

	name := "Product Refined Cotton Chair"
	code := "ACTIVE"
	description := ptr.StringPtr("Teste Product")

	metadata := map[string]any{
		"bitcoin": "3g9ofZcD7KRWL44BWdNa3PyM4PfzgqDG5P",
		"chave":   "metadata_chave",
		"boolean": true,
	}

	input := mmodel.CreateProductInput{
		Name: name,
		Status: mmodel.Status{
			Code:        code,
			Description: description,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Product{
		ID:             productID,
		Name:           name,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status: mmodel.Status{
			Code:        code,
			Description: description,
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/products",
		URIAPILedger, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/product_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	productServ := NewProduct(factory)

	result, err := productServ.Create(organizationID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST "+uri])
}

func Test_product_Get(t *testing.T) {
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"
	ledgerID := "01931b04-c2d1-7a41-83ac-c5d6d8a3c22c"

	limit := 2
	page := 1

	expectedResult := mmodel.Products{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Product{
			{
				ID:   "01932727-1b5a-7540-98c0-6521ffe78ce6",
				Name: "Product Licensed Concrete Hat",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste Product"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 11, 18, 51, 33, 15793, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 11, 18, 51, 33, 15793, time.UTC),
				DeletedAt:      nil,
			},
			{
				ID:   "0193271b-877f-7c98-a5a6-43b664d68982",
				Name: "Toy30 Portfolio",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste Product"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 11, 18, 51, 27, 447406, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 11, 18, 51, 27, 447406, time.UTC),
				DeletedAt:      nil,
				Metadata: map[string]any{
					"bitcoin": "3g9ofZcD7KRWL44BWdNa3PyM4PfzgqDG5P",
					"chave":   "metadata_chave",
					"boolean": true,
				},
			},
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/products?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/product_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	product := NewProduct(factory)

	result, err := product.Get(organizationID, ledgerID, limit, page)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].Metadata, v.Metadata)
	}

	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}
