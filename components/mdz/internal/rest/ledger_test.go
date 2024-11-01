package rest

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_ledger_Create(t *testing.T) {
	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"

	name := "Romaguera and Sons"
	code := ptr.StringPtr("ACTIVE")
	description := ptr.StringPtr("Teste Ledger")
	bitcoin := ptr.StringPtr("1iR2KqpxRFjLsPUpWmpADMC7piRNsMAAjq")
	bool := ptr.BoolPtr(false)
	chave := ptr.StringPtr("metadata_chave")
	double := ptr.Float64Ptr(10.5)
	int := ptr.IntPtr(1)

	input := model.LedgerInput{
		Name: name,
		Status: &model.LedgerStatus{
			Code:        code,
			Description: description,
		},
		Metadata: &model.LedgerMetadata{
			Bitcoin: bitcoin,
			Boolean: bool,
			Chave:   chave,
			Double:  double,
			Int:     int,
		},
	}

	expectedResult := &model.LedgerCreate{
		ID:             ledgerID,
		Name:           name,
		OrganizationID: organizationID,
		Status: model.LedgerStatus{
			Code:        code,
			Description: description,
		},
		Metadata: model.LedgerMetadata{
			Bitcoin: bitcoin,
			Boolean: bool,
			Chave:   chave,
			Double:  double,
			Int:     int,
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers", URIAPILedger, organizationID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/ledger_response.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	ledServ := NewLedger(factory)

	result, err := ledServ.Create(organizationID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.OrganizationID, organizationID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.Metadata.Bitcoin, result.Metadata.Bitcoin)
	assert.Equal(t, expectedResult.Metadata.Boolean, result.Metadata.Boolean)
	assert.Equal(t, expectedResult.Metadata.Chave, result.Metadata.Chave)
	assert.Equal(t, expectedResult.Metadata.Double, result.Metadata.Double)
	assert.Equal(t, expectedResult.Metadata.Int, result.Metadata.Int)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST http://127.0.0.1:3000/v1/organizations/0192e250-ed9d-7e5c-a614-9b294151b572/ledgers"])
}
