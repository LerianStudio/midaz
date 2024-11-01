package rest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

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

func Test_ledger_List(t *testing.T) {
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"

	limit := 5
	page := 1

	expectedResult := model.LedgerList{
		Page:  page,
		Limit: limit,
		Items: []model.LedgerItems{
			{
				ID:             "0192e362-b270-7158-a647-7a59e4e26a27",
				Name:           "Ankunding - Paucek",
				OrganizationID: "0192e250-ed9d-7e5c-a614-9b294151b572",
				Status: &model.LedgerStatus{
					Code:        ptr.StringPtr("ACTIVE"),
					Description: ptr.StringPtr("Teste Ledger"),
				},
				CreatedAt: time.Date(2024, 10, 31, 16, 22, 29, 232078000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 16, 22, 29, 232078000, time.UTC),
				DeletedAt: nil,
				Metadata: &model.LedgerMetadata{
					Bitcoin: ptr.StringPtr("3HH89s3LPALardk1jLt2PcjAJng"),
					Boolean: ptr.BoolPtr(true),
					Chave:   ptr.StringPtr("metadata_chave"),
					Double:  ptr.Float64Ptr(10.5),
					Int:     ptr.IntPtr(1),
				},
			},
			{
				ID:             "0192e258-2c81-7e37-b6ba-a2007495c652",
				Name:           "Zieme - Mante",
				OrganizationID: "0192e250-ed9d-7e5c-a614-9b294151b572",
				Status: &model.LedgerStatus{
					Code:        ptr.StringPtr("ACTIVE"),
					Description: ptr.StringPtr("Teste Ledger"),
				},
				CreatedAt: time.Date(2024, 10, 31, 11, 31, 22, 369928000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 11, 31, 22, 369928000, time.UTC),
				DeletedAt: nil,
				Metadata: &model.LedgerMetadata{
					Bitcoin: ptr.StringPtr("329aaP47xTc8hQxXB92896U2RBXGEt"),
					Boolean: ptr.BoolPtr(true),
					Chave:   ptr.StringPtr("metadata_chave"),
					Double:  ptr.Float64Ptr(10.5),
					Int:     ptr.IntPtr(1),
				},
			},
			{
				ID:             "0192e257-f5c0-7687-8534-303bae7aa4aa",
				Name:           "Lang LLC",
				OrganizationID: "0192e250-ed9d-7e5c-a614-9b294151b572",
				Status: &model.LedgerStatus{
					Code:        ptr.StringPtr("ACTIVE"),
					Description: nil,
				},
				CreatedAt: time.Date(2024, 10, 31, 11, 31, 8, 352409000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 11, 31, 8, 352409000, time.UTC),
				DeletedAt: nil,
			},
			{
				ID:             "0192e251-328d-7390-99f5-5c54980115ed",
				Name:           "Romaguera and Sons",
				OrganizationID: "0192e250-ed9d-7e5c-a614-9b294151b572",
				Status: &model.LedgerStatus{
					Code:        ptr.StringPtr("ACTIVE"),
					Description: ptr.StringPtr("Teste Ledger"),
				},
				CreatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
				DeletedAt: nil,
				Metadata: &model.LedgerMetadata{
					Bitcoin: ptr.StringPtr("1iR2KqpxRFjLsPUpWmpADMC7piRNsMAAjq"),
					Boolean: ptr.BoolPtr(false),
					Chave:   ptr.StringPtr("metadata_chave"),
					Double:  ptr.Float64Ptr(10.5),
					Int:     ptr.IntPtr(1),
				},
			},
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers?limit=%d&page=%d", URIAPILedger, organizationID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/ledger_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	ledServ := NewLedger(factory)

	result, err := ledServ.Get(organizationID, limit, page)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://127.0.0.1:3000/v1/organizations/0192e250-ed9d-7e5c-a614-9b294151b572/ledgers?limit=5&page=1"])
}

func Test_ledger_GetByID(t *testing.T) {
	ledgerID := "0192e362-b270-7158-a647-7a59e4e26a27"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"

	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &model.LedgerItems{
		ID:             ledgerID,
		Name:           "Ankunding - Paucek",
		OrganizationID: organizationID,
		Status: &model.LedgerStatus{
			Code:        ptr.StringPtr("ACTIVE"),
			Description: ptr.StringPtr("Teste Ledger"),
		},
		CreatedAt: time.Date(2024, 10, 31, 16, 22, 29, 232078000, time.UTC),
		UpdatedAt: time.Date(2024, 10, 31, 16, 22, 29, 232078000, time.UTC),
		DeletedAt: nil,
		Metadata: &model.LedgerMetadata{
			Bitcoin: ptr.StringPtr("3HH89s3LPALardk1jLt2PcjAJng"),
			Boolean: ptr.BoolPtr(true),
			Chave:   ptr.StringPtr("metadata_chave"),
			Double:  ptr.Float64Ptr(10.5),
			Int:     ptr.IntPtr(1),
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s",
		URIAPILedger, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/ledger_response_item.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	ledServ := NewLedger(factory)

	result, err := ledServ.GetByID(organizationID, ledgerID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.DeletedAt, result.DeletedAt)
	assert.Equal(t, expectedResult.Metadata.Bitcoin, result.Metadata.Bitcoin)
	assert.Equal(t, expectedResult.Metadata.Boolean, result.Metadata.Boolean)
	assert.Equal(t, expectedResult.Metadata.Chave, result.Metadata.Chave)
	assert.Equal(t, expectedResult.Metadata.Double, result.Metadata.Double)
	assert.Equal(t, expectedResult.Metadata.Int, result.Metadata.Int)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://127.0.0.1:3000/v1/organizations/0192e250-ed9d-7e5c-a614-9b294151b572/ledgers/0192e362-b270-7158-a647-7a59e4e26a27"])
}
