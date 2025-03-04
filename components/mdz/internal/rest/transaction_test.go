package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

func TestTransaction_Create(t *testing.T) {
	// Given
	orgID := "org_123"
	ledgerID := "ldg_123"
	expectedTransaction := mmodel.Transaction{
		ID: "txn_123",
	}

	roundTripFunc := func(req *http.Request) *http.Response {
		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		assert.Equal(
			t,
			"http://example.com/v1/organizations/org_123/ledgers/ldg_123/transactions",
			req.URL.String(),
		)

		respBody, _ := json.Marshal(expectedTransaction)
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		}
	}

	clientMock := mockutil.NewMockHTTPClient(roundTripFunc)
	f := &factory.Factory{
		Env: &environment.Env{
			URLAPITransaction: "http://example.com",
		},
		Token:      "test-token",
		HTTPClient: clientMock,
	}

	// When
	repo := NewTransaction(f)
	transaction, err := repo.Create(orgID, ledgerID, mmodel.CreateTransactionInput{})

	// Then
	assert.NoError(t, err)
	assert.Equal(t, expectedTransaction.ID, transaction.ID)
}

func TestTransaction_Get(t *testing.T) {
	// Given
	orgID := "org_123"
	ledgerID := "ldg_123"
	expectedTransactions := mmodel.Transactions{
		Data: []*mmodel.Transaction{
			{ID: "txn_123"},
			{ID: "txn_456"},
		},
	}

	roundTripFunc := func(req *http.Request) *http.Response {
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		assert.Contains(
			t,
			req.URL.String(),
			"http://example.com/v1/organizations/org_123/ledgers/ldg_123/transactions",
		)

		respBody, _ := json.Marshal(expectedTransactions)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		}
	}

	clientMock := mockutil.NewMockHTTPClient(roundTripFunc)
	f := &factory.Factory{
		Env: &environment.Env{
			URLAPITransaction: "http://example.com",
		},
		Token:      "test-token",
		HTTPClient: clientMock,
	}

	// When
	repo := NewTransaction(f)
	transactions, err := repo.Get(orgID, ledgerID, 10, 1, "asc", "", "")

	// Then
	assert.NoError(t, err)
	assert.Equal(t, 2, len(transactions.Data))
	assert.Equal(t, "txn_123", transactions.Data[0].ID)
	assert.Equal(t, "txn_456", transactions.Data[1].ID)
}

func TestTransaction_GetByID(t *testing.T) {
	// Given
	orgID := "org_123"
	ledgerID := "ldg_123"
	transactionID := "txn_123"
	expectedTransaction := mmodel.Transaction{
		ID: transactionID,
	}

	roundTripFunc := func(req *http.Request) *http.Response {
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		assert.Equal(
			t,
			"http://example.com/v1/organizations/org_123/ledgers/ldg_123/transactions/txn_123",
			req.URL.String(),
		)

		respBody, _ := json.Marshal(expectedTransaction)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		}
	}

	clientMock := mockutil.NewMockHTTPClient(roundTripFunc)
	f := &factory.Factory{
		Env: &environment.Env{
			URLAPITransaction: "http://example.com",
		},
		Token:      "test-token",
		HTTPClient: clientMock,
	}

	// When
	repo := NewTransaction(f)
	transaction, err := repo.GetByID(orgID, ledgerID, transactionID)

	// Then
	assert.NoError(t, err)
	assert.Equal(t, transactionID, transaction.ID)
}

func TestTransaction_Update(t *testing.T) {
	// Given
	orgID := "org_123"
	ledgerID := "ldg_123"
	transactionID := "txn_123"
	expectedTransaction := mmodel.Transaction{
		ID: transactionID,
	}

	roundTripFunc := func(req *http.Request) *http.Response {
		assert.Equal(t, http.MethodPatch, req.Method)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		assert.Equal(
			t,
			"http://example.com/v1/organizations/org_123/ledgers/ldg_123/transactions/txn_123",
			req.URL.String(),
		)

		respBody, _ := json.Marshal(expectedTransaction)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		}
	}

	clientMock := mockutil.NewMockHTTPClient(roundTripFunc)
	f := &factory.Factory{
		Env: &environment.Env{
			URLAPITransaction: "http://example.com",
		},
		Token:      "test-token",
		HTTPClient: clientMock,
	}

	// When
	repo := NewTransaction(f)
	transaction, err := repo.Update(orgID, ledgerID, transactionID, mmodel.UpdateTransactionInput{})

	// Then
	assert.NoError(t, err)
	assert.Equal(t, transactionID, transaction.ID)
}

func TestTransaction_Delete(t *testing.T) {
	// Given
	orgID := "org_123"
	ledgerID := "ldg_123"
	transactionID := "txn_123"

	roundTripFunc := func(req *http.Request) *http.Response {
		assert.Equal(t, http.MethodDelete, req.Method)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		assert.Equal(
			t,
			"http://example.com/v1/organizations/org_123/ledgers/ldg_123/transactions/txn_123",
			req.URL.String(),
		)

		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(bytes.NewReader([]byte{})),
		}
	}

	clientMock := mockutil.NewMockHTTPClient(roundTripFunc)
	f := &factory.Factory{
		Env: &environment.Env{
			URLAPITransaction: "http://example.com",
		},
		Token:      "test-token",
		HTTPClient: clientMock,
	}

	// When
	repo := NewTransaction(f)
	err := repo.Delete(orgID, ledgerID, transactionID)

	// Then
	assert.NoError(t, err)
}
