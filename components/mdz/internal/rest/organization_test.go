package rest

import (
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func TestCreateOrganization(t *testing.T) {
	metadata := map[string]any{
		"bitcoin": "3fozQHVceartTg14kwF4PkfgUU4JhsUX",
		"chave":   "metadata_chave",
		"boolean": true,
	}

	inp := mmodel.CreateOrganizationInput{
		LegalName:       "Corwin LLC",
		DoingBusinessAs: ptr.StringPtr("The ledger.io"),
		LegalDocument:   "48784548000104",
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: ptr.StringPtr("Teste Ledger"),
		},
		Address: mmodel.Address{
			Line1:   "Avenida Paulista, 1234",
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: "04696040",
			City:    "Ozellaport",
			State:   "MU",
			Country: "LV",
		},
		Metadata: metadata,
	}

	expectedResult := mmodel.Organization{
		ID:              "1a259e90-8f28-491d-8c09-c047293b1a0f",
		LegalName:       "Corwin LLC",
		DoingBusinessAs: ptr.StringPtr("The ledger.io"),
		LegalDocument:   "48784548000104",
		Address: mmodel.Address{
			Line1:   "Avenida Paulista, 1234",
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: "04696040",
			City:    "Ozellaport",
			State:   "MU",
			Country: "LV",
		},
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: ptr.StringPtr("Teste Ledger"),
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)

	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	httpmock.RegisterResponder(http.MethodPost, URIAPILedger+"/v1/organizations",
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/organization_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	org := NewOrganization(factory)

	result, err := org.Create(inp)

	assert.NoError(t, err)

	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.LegalName, result.LegalName)
	assert.Equal(t, expectedResult.DoingBusinessAs, result.DoingBusinessAs)
	assert.Equal(t, expectedResult.LegalDocument, result.LegalDocument)
	assert.Equal(t, expectedResult.Address.Line1, result.Address.Line1)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST http://127.0.0.1:3000/v1/organizations"])
}

// \1 performs an operation
func TestOrganizationGetByID(t *testing.T) {
	organizationID := "0192c559-62f4-738b-9be5-262b71f6375a"
	URIAPILedger := "http://127.0.0.1:3000"

	metadata := map[string]any{
		"bitcoin": "1R4DvodZi68SxKbvNeQGCkaPj25Ryumy",
		"chave":   "max",
	}

	expectedResult := &mmodel.Organization{
		ID:              organizationID,
		LegalName:       "Koelpin - Marquardt",
		DoingBusinessAs: ptr.StringPtr("The ledger.io"),
		LegalDocument:   "48784548000104",
		Address: mmodel.Address{
			Line1: "Avenida Paulista, 1234",
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)

	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder(http.MethodGet, URIAPILedger+"/v1/organizations/"+organizationID,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/organization_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	org := NewOrganization(factory)

	result, err := org.GetByID("0192c559-62f4-738b-9be5-262b71f6375a")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.LegalName, result.LegalName)
	assert.Equal(t, expectedResult.DoingBusinessAs, result.DoingBusinessAs)
	assert.Equal(t, expectedResult.LegalDocument, result.LegalDocument)
	assert.Equal(t, expectedResult.Address.Line1, result.Address.Line1)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://127.0.0.1:3000/v1/organizations/0192c559-62f4-738b-9be5-262b71f6375a"])
}

// \1 performs an operation
func TestOrganizationUpdate(t *testing.T) {
	inp := mmodel.UpdateOrganizationInput{
		LegalName:       "Corwin LLC",
		DoingBusinessAs: "The ledger.io",
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: ptr.StringPtr("Teste Ledger"),
		},
		Address: mmodel.Address{
			Line1:   "Avenida Paulista, 1234",
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: "04696040",
			City:    "Ozellaport",
			State:   "MU",
			Country: "LV",
		},
	}

	expectedResult := &mmodel.Organization{
		ID:              "1a259e90-8f28-491d-8c09-c047293b1a0f",
		LegalName:       "Corwin LLC",
		DoingBusinessAs: ptr.StringPtr("The ledger.io"),
		LegalDocument:   "48784548000104",
		Address: mmodel.Address{
			Line1:   "Avenida Paulista, 1234",
			Line2:   ptr.StringPtr("CJ 203"),
			ZipCode: "04696040",
			City:    "Ozellaport",
			State:   "MU",
			Country: "LV",
		},
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: ptr.StringPtr("Teste Ledger"),
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)

	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"
	organizationID := "1a259e90-8f28-491d-8c09-c047293b1a0f"

	httpmock.RegisterResponder(http.MethodPatch, URIAPILedger+"/v1/organizations/"+organizationID,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/organization_response_update.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	org := NewOrganization(factory)

	result, err := org.Update(organizationID, inp)

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

// \1 performs an operation
func Test_organization_Delete(t *testing.T) {
	client := &http.Client{}
	httpmock.ActivateNonDefault(client)

	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"
	organizationID := "1a259e90-8f28-491d-8c09-c047293b1a0f"

	httpmock.RegisterResponder(http.MethodDelete, URIAPILedger+"/v1/organizations/"+organizationID,
		httpmock.NewStringResponder(http.StatusNoContent, ""))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	org := NewOrganization(factory)

	err := org.Delete(organizationID)

	assert.NoError(t, err)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["DELETE http://127.0.0.1:3000/v1/organizations/1a259e90-8f28-491d-8c09-c047293b1a0f"])
}

// \1 performs an operation
func Test_organization_Get(t *testing.T) {
	organizationID := "0192c559-62f4-738b-9be5-262b71f6375a"
	URIAPILedger := "http://127.0.0.1:3000"

	limit := 5
	page := 1

	metadata := map[string]any{
		"bitcoin": "1R4DvodZi68SxKbvNeQGCkaPj25Ryumy",
		"chave":   "max",
	}

	item := mmodel.Organization{
		ID:              organizationID,
		LegalName:       "Koelpin - Marquardt",
		DoingBusinessAs: ptr.StringPtr("The ledger.io"),
		LegalDocument:   "48784548000104",
		Address: mmodel.Address{
			Line1: "Avenida Paulista, 1234",
		},
		Metadata: metadata,
	}

	expectedResult := mmodel.Organizations{
		Items: []mmodel.Organization{
			item,
		},
		Limit: limit,
		Page:  page,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)

	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder(http.MethodGet, URIAPILedger+"/v1/organizations?limit=5&page=1",
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/organization_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	org := NewOrganization(factory)

	result, err := org.Get(limit, page, "", "", "")
	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].Metadata, v.Metadata)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()

	assert.Equal(t, 1, info["GET http://127.0.0.1:3000/v1/organizations?limit=5&page=1"])
}
