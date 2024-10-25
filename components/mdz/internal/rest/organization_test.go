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
	tests := []struct {
		name           string
		input          model.Organization
		mockResponse   string
		mockStatusCode int
		expectError    bool
		expectedResult *model.OrganizationCreate
	}{
		{
			name: "success",
			input: model.Organization{
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
			},
			mockResponse: `{
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
			}`,
			mockStatusCode: 201,
			expectError:    false,
			expectedResult: &model.OrganizationCreate{
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
			},
		},
		{
			name: "invalid input",
			input: model.Organization{
				LegalName:     "",
				LegalDocument: "",
			},
			mockResponse:   `{"error": "invalid input"}`,
			mockStatusCode: 400,
			expectError:    true,
			expectedResult: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			httpmock.RegisterResponder("POST", "http://127.0.0.1:3000/v1/organizations",
				httpmock.NewStringResponder(tc.mockStatusCode, tc.mockResponse))

			factory := &factory.Factory{
				HTTPClient: &http.Client{},
				Env: &environment.Env{
					URLAPILedger: "http://127.0.0.1:3000",
				},
			}

			orgService := NewOrganization(factory)

			result, err := orgService.Create(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.expectedResult.ID, result.ID)
				assert.Equal(t, tc.expectedResult.LegalName, result.LegalName)
				assert.Equal(t, tc.expectedResult.DoingBusinessAs, result.DoingBusinessAs)
				assert.Equal(t, tc.expectedResult.LegalDocument, result.LegalDocument)
				assert.Equal(t, tc.expectedResult.Address.Line1, result.Address.Line1)
			}

			info := httpmock.GetCallCountInfo()
			assert.Equal(t, 1, info["POST http://127.0.0.1:3000/v1/organizations"])
		})
	}
}
