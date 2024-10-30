package rest

import (
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestCreateOrganization(t *testing.T) {

	mockResp := `{
		"id": "1a259e90-8f28-491d-8c09-c047293b1a0f",
		"legalName": "Corwin LLC",
		"doingBusinessAs": "The ledger.io",
		"legalDocument": "48784548000104",
		"address": {
			"line1": "Avenida Paulista, 1234",
			"line2": "CJ 203",
			"zipCode": "04696040",
			"city": "Ozellaport",
			"state": "MU",
			"country": "LV"
		},
		"status": {
			"code": "ACTIVE",
			"description": "Teste Ledger"
		},
		"createdAt": "2024-10-22T23:36:35.979331542Z",
		"updatedAt": "2024-10-22T23:36:35.979334336Z",
		"metadata": {
			"bitcoinn": "3fozQHVceartTg14kwF4PkfgUU4JhsUX",
			"boolean": true,
			"chave": "metadata_chave",
			"double": 10.5,
			"int": 1
		}
	}`

	input := model.Organization{
		LegalName:       "Corwin LLC",
		DoingBusinessAs: "The ledger.io",
		LegalDocument:   "48784548000104",
		Status: model.Status{
			Code:        ptr.StringPtr("ACTIVE"),
			Description: "Teste Ledger",
		},
		Address: model.Address{
			Line1:   ptr.StringPtr("Avenida Paulista, 1234"),
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: ptr.StringPtr("04696040"),
			City:    ptr.StringPtr("Ozellaport"),
			State:   ptr.StringPtr("MU"),
			Country: "LV",
		},
		Metadata: &model.Metadata{
			Chave:   ptr.StringPtr("metadata_chave"),
			Bitcoin: ptr.StringPtr("3fozQHVceartTg14kwF4PkfgUU4JhsUX"),
			Boolean: ptr.BoolPtr(true),
			Double:  ptr.Float64Ptr(10.5),
			Int:     ptr.IntPtr(1),
		},
	}

	expectedResult := &model.OrganizationCreate{
		ID:              "1a259e90-8f28-491d-8c09-c047293b1a0f",
		LegalName:       "Corwin LLC",
		DoingBusinessAs: "The ledger.io",
		LegalDocument:   "48784548000104",
		Address: model.Address{
			Line1:   ptr.StringPtr("Avenida Paulista, 1234"),
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: ptr.StringPtr("04696040"),
			City:    ptr.StringPtr("Ozellaport"),
			State:   ptr.StringPtr("MU"),
			Country: "LV",
		},
		Status: model.Status{
			Code:        ptr.StringPtr("ACTIVE"),
			Description: "Teste Ledger",
		},
		Metadata: model.Metadata{
			Chave:   ptr.StringPtr("metadata_chave"),
			Bitcoin: ptr.StringPtr("3fozQHVceartTg14kwF4PkfgUU4JhsUX"),
			Boolean: ptr.BoolPtr(true),
			Double:  ptr.Float64Ptr(10.5),
			Int:     ptr.IntPtr(1),
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	httpmock.RegisterResponder(http.MethodPost, URIAPILedger+"/v1/organizations",
		httpmock.NewStringResponder(http.StatusCreated, mockResp))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	orgService := NewOrganization(factory)

	result, err := orgService.Create(input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.LegalName, result.LegalName)
	assert.Equal(t, expectedResult.DoingBusinessAs, result.DoingBusinessAs)
	assert.Equal(t, expectedResult.LegalDocument, result.LegalDocument)
	assert.Equal(t, expectedResult.Address.Line1, result.Address.Line1)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST http://127.0.0.1:3000/v1/organizations"])
}

func TestOrganizationGetByID(t *testing.T) {

	mockResp := `{
	    "id": "0192c559-62f4-738b-9be5-262b71f6375a",
	    "parentOrganizationId": null,
	    "legalName": "Koelpin - Marquardt",
	    "doingBusinessAs": "The ledger.io",
	    "legalDocument": "48784548000104",
	    "address": {
	        "line1": "Avenida Paulista, 1234",
	        "line2": null,
	        "zipCode": "",
	        "city": "",
	        "state": "",
	        "country": "KG"
	    },
	    "status": {
	        "code": "ACTIVE",
	        "description": "Teste Ledger"
	    },
	    "createdAt": "2024-10-25T20:23:42.580221Z",
	    "updatedAt": "2024-10-25T20:23:42.580221Z",
	    "deletedAt": null,
	    "metadata": {
	        "bitcoinn": "1R4DvodZi68SxKbvNeQGCkaPj25Ryumy",
	        "chave": "max"
	    }
	}`

	organizationID := "0192c559-62f4-738b-9be5-262b71f6375a"
	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &model.OrganizationItem{
		ID:              organizationID,
		LegalName:       "Koelpin - Marquardt",
		DoingBusinessAs: "The ledger.io",
		LegalDocument:   "48784548000104",
		Address: model.Address{
			Line1: ptr.StringPtr("Avenida Paulista, 1234"),
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder(http.MethodGet, URIAPILedger+"/v1/organizations/"+organizationID,
		httpmock.NewStringResponder(http.StatusOK, mockResp))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	orgService := NewOrganization(factory)

	result, err := orgService.GetByID("0192c559-62f4-738b-9be5-262b71f6375a")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.LegalName, result.LegalName)
	assert.Equal(t, expectedResult.DoingBusinessAs, result.DoingBusinessAs)
	assert.Equal(t, expectedResult.LegalDocument, result.LegalDocument)
	assert.Equal(t, expectedResult.Address.Line1, result.Address.Line1)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://127.0.0.1:3000/v1/organizations/0192c559-62f4-738b-9be5-262b71f6375a"])
}

func TestOrganizationUpdate(t *testing.T) {
	mockResp := `{
		"id": "1a259e90-8f28-491d-8c09-c047293b1a0f",
		"legalName": "Corwin LLC",
		"doingBusinessAs": "The ledger.io",
		"legalDocument": "48784548000104",
		"address": {
			"line1": "Avenida Paulista, 1234",
			"line2": "CJ 203",
			"zipCode": "04696040",
			"city": "Ozellaport",
			"state": "MU",
			"country": "LV"
		},
		"status": {
			"code": "ACTIVE",
			"description": "Teste Ledger"
		},
		"createdAt": "2024-10-22T23:36:35.979331542Z",
		"updatedAt": "2024-10-22T23:36:35.979334336Z"
	}`

	input := model.OrganizationUpdate{
		LegalName:       "Corwin LLC",
		DoingBusinessAs: "The ledger.io",
		LegalDocument:   ptr.StringPtr("48784548000104"),
		Status: &model.StatusUpdate{
			Code:        ptr.StringPtr("ACTIVE"),
			Description: ptr.StringPtr("Teste Ledger"),
		},
		Address: model.Address{
			Line1:   ptr.StringPtr("Avenida Paulista, 1234"),
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: ptr.StringPtr("04696040"),
			City:    ptr.StringPtr("Ozellaport"),
			State:   ptr.StringPtr("MU"),
			Country: "LV",
		},
	}

	expectedResult := &model.OrganizationItem{
		ID:              "1a259e90-8f28-491d-8c09-c047293b1a0f",
		LegalName:       "Corwin LLC",
		DoingBusinessAs: "The ledger.io",
		LegalDocument:   "48784548000104",
		Address: model.Address{
			Line1:   ptr.StringPtr("Avenida Paulista, 1234"),
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: ptr.StringPtr("04696040"),
			City:    ptr.StringPtr("Ozellaport"),
			State:   ptr.StringPtr("MU"),
			Country: "LV",
		},
		Status: model.Status{
			Code:        ptr.StringPtr("ACTIVE"),
			Description: "Teste Ledger",
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"
	organizationID := "1a259e90-8f28-491d-8c09-c047293b1a0f"

	httpmock.RegisterResponder(http.MethodPatch, URIAPILedger+"/v1/organizations/"+organizationID,
		httpmock.NewStringResponder(http.StatusOK, mockResp))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	orgService := NewOrganization(factory)

	result, err := orgService.Update(organizationID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.LegalName, result.LegalName)
	assert.Equal(t, expectedResult.DoingBusinessAs, result.DoingBusinessAs)
	assert.Equal(t, expectedResult.LegalDocument, result.LegalDocument)
	assert.Equal(t, expectedResult.Address.Line1, result.Address.Line1)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["PATCH http://127.0.0.1:3000/v1/organizations/1a259e90-8f28-491d-8c09-c047293b1a0f"])
}
