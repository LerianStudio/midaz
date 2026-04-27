// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package crmhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds a crmhttp.Client pointing at the given test server. The
// circuit breaker is real (shared with production) because we want the HTTP→error
// mapping exercised end-to-end; the breaker is permissive enough that a handful of
// failures per test won't trip it.
func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	logger := &libLog.NopLogger{}

	mgr, err := libCircuitBreaker.NewManager(logger)
	require.NoError(t, err)

	c, err := NewClient(baseURL, mgr, logger)
	require.NoError(t, err)

	return c
}

func TestClient_GetHolder_Success(t *testing.T) {
	holderID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer token-xyz", r.Header.Get("Authorization"))
		assert.Contains(t, r.URL.Path, holderID.String())

		_ = json.NewEncoder(w).Encode(mmodel.Holder{ID: &holderID})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	got, err := c.GetHolder(context.Background(), "org", holderID, "token-xyz")
	require.NoError(t, err)
	require.NotNil(t, got.ID)
	assert.Equal(t, holderID, *got.ID)
}

func TestClient_GetHolder_404_MapsToHolderNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	_, err := c.GetHolder(context.Background(), "org", uuid.New(), "token")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrHolderNotFound), "expected ErrHolderNotFound, got %v", err)
}

func TestClient_GetHolder_5xx_MapsToTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	_, err := c.GetHolder(context.Background(), "org", uuid.New(), "token")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrCRMTransient), "expected ErrCRMTransient, got %v", err)
}

func TestClient_GetHolder_ConnectionRefused_MapsToTransient(t *testing.T) {
	// Create and immediately close → any subsequent call gets a connection error.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	c := newTestClient(t, srv.URL)

	_, err := c.GetHolder(context.Background(), "org", uuid.New(), "token")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrCRMTransient), "expected ErrCRMTransient, got %v", err)
}

func TestClient_GetHolder_400_MapsToBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		_ = json.NewEncoder(w).Encode(mmodel.Error{Code: "CRM-0015", Message: "bad"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	_, err := c.GetHolder(context.Background(), "org", uuid.New(), "token")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrCRMBadRequest), "expected ErrCRMBadRequest, got %v", err)
}

func TestClient_CreateAccountAlias_Success_SendsIdempotencyKey(t *testing.T) {
	aliasID := uuid.New()
	holderID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "my-key", r.Header.Get("Idempotency-Key"))
		assert.Contains(t, r.URL.Path, holderID.String()+"/aliases")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, _ := io.ReadAll(r.Body)
		assert.True(t, strings.Contains(string(body), "ledgerId"), "body carries alias payload")

		w.WriteHeader(http.StatusCreated)

		_ = json.NewEncoder(w).Encode(mmodel.Alias{ID: &aliasID})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	input := &mmodel.CreateAliasInput{LedgerID: uuid.NewString(), AccountID: uuid.NewString()}

	got, err := c.CreateAccountAlias(context.Background(), "org", holderID, input, "my-key", "tok")
	require.NoError(t, err)
	require.NotNil(t, got.ID)
	assert.Equal(t, aliasID, *got.ID)
}

func TestClient_CreateAccountAlias_409_CRMConflict_MapsToIdempotencyKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)

		_ = json.NewEncoder(w).Encode(mmodel.Error{Code: constant.ErrIdempotencyKey.Error(), Message: "dup"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	_, err := c.CreateAccountAlias(context.Background(), "org", uuid.New(),
		&mmodel.CreateAliasInput{LedgerID: "l", AccountID: "a"}, "k", "tok")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrIdempotencyKey), "expected ErrIdempotencyKey mapping, got %v", err)
}

func TestClient_CreateAccountAlias_409_AliasHolderConflict_MapsSpecifically(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)

		_ = json.NewEncoder(w).Encode(mmodel.Error{Code: constant.ErrAccountAlreadyAssociated.Error(), Message: "dup"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	_, err := c.CreateAccountAlias(context.Background(), "org", uuid.New(),
		&mmodel.CreateAliasInput{LedgerID: "l", AccountID: "a"}, "k", "tok")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrAliasHolderConflict), "expected alias-holder conflict, got %v", err)
}

func TestClient_CreateAccountAlias_409_Generic_MapsToCRMConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)

		_ = json.NewEncoder(w).Encode(mmodel.Error{Code: "UNKNOWN", Message: "dup"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	_, err := c.CreateAccountAlias(context.Background(), "org", uuid.New(),
		&mmodel.CreateAliasInput{LedgerID: "l", AccountID: "a"}, "k", "tok")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrCRMConflict), "expected generic CRM conflict, got %v", err)
}

func TestClient_GetAliasByAccount_404_ReturnsAliasNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "ledger_id=")
		assert.Contains(t, r.URL.RawQuery, "account_id=")

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	alias, err := c.GetAliasByAccount(context.Background(), "org", "led", "acc", "tok")
	require.Error(t, err)
	assert.Nil(t, alias)
	assert.True(t, errors.Is(err, constant.ErrAliasNotFound), "expected ErrAliasNotFound, got %v", err)
}

func TestClient_GetAliasByAccount_Success(t *testing.T) {
	aliasID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(mmodel.Alias{ID: &aliasID})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	got, err := c.GetAliasByAccount(context.Background(), "org", "led", "acc", "tok")
	require.NoError(t, err)
	require.NotNil(t, got.ID)
	assert.Equal(t, aliasID, *got.ID)
}

func TestClient_CloseAlias_204_NoContent_Success(t *testing.T) {
	holderID := uuid.New()
	aliasID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, holderID.String())
		assert.Contains(t, r.URL.Path, aliasID.String())
		assert.Contains(t, r.URL.Path, "/close")
		assert.Equal(t, "close-key", r.Header.Get("Idempotency-Key"))

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	err := c.CloseAlias(context.Background(), "org", holderID, aliasID, "close-key", "tok")
	assert.NoError(t, err)
}
