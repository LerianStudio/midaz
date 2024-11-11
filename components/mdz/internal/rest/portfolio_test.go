package rest

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_portfolio_Create(t *testing.T) {
	portfolioID := "01931b44-6e33-791a-bfad-27992fa15984"
	ledgerID := "01931b04-c2d1-7a41-83ac-c5d6d8a3c22c"
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"

	name := "Leslie_Spencer Portfolio"
	code := "ACTIVE"
	description := ptr.StringPtr("Teste Portfolio")
	allowSending := ptr.BoolPtr(true)
	allowReceiving := ptr.BoolPtr(false)

	metadata := map[string]any{
		"bitcoin": "3o5onPR55kL6ajk14dGL5Q1fEhAnvY",
		"chave":   "metadata_chave",
		"boolean": true,
	}

	input := mmodel.CreatePortfolioInput{
		Name: name,
		Status: mmodel.StatusAllow{
			Code:           code,
			Description:    description,
			AllowSending:   allowSending,
			AllowReceiving: allowReceiving,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Portfolio{
		ID:             portfolioID,
		Name:           name,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status: mmodel.StatusAllow{
			Code:           code,
			Description:    description,
			AllowSending:   allowSending,
			AllowReceiving: allowReceiving,
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/portfolios",
		URIAPILedger, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/portfolio_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	portfolioServ := NewPortfolio(factory)

	result, err := portfolioServ.Create(organizationID, ledgerID, input)

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
