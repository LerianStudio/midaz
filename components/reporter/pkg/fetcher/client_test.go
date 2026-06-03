// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Constructor tests ---

func TestNewFetcherClient_Defaults(t *testing.T) {
	t.Parallel()

	client := NewFetcherClient("http://localhost:4007")

	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:4007", client.baseURL)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, fetcherGlobalHTTPTimeout, client.httpClient.Timeout)
	assert.Nil(t, client.m2mProvider)
	assert.Nil(t, client.cbExecutor)
}

func TestNewFetcherClient_WithOptions(t *testing.T) {
	t.Parallel()

	provider := &stubM2MProvider{token: "test-token"}
	customHTTP := &http.Client{Timeout: 5 * time.Second}

	client := NewFetcherClient(
		"http://fetcher:4007",
		WithM2MTokenProvider(provider),
		WithHTTPClient(customHTTP),
	)

	assert.NotNil(t, client)
	assert.Equal(t, "http://fetcher:4007", client.baseURL)
	assert.Equal(t, customHTTP, client.httpClient)
	assert.NotNil(t, client.m2mProvider)
}

// --- GetConnectionSchema tests ---

func TestFetcherClient_GetConnectionSchema_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/management/connections/ds-1/schema", r.URL.Path)

		resp := ConnectionSchemaResponse{
			ID: "ds-1",
			Tables: []SchemaTableResponse{
				{
					Name: "accounts",
					Fields: []SchemaFieldResponse{
						{Name: "id", Type: "uuid"},
						{Name: "name", Type: "varchar"},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	schema, err := client.GetConnectionSchema(context.Background(), "ds-1")
	require.NoError(t, err)
	assert.Equal(t, "ds-1", schema.ID)
	require.Len(t, schema.Tables, 1)
	assert.Equal(t, "accounts", schema.Tables[0].Name)
	require.Len(t, schema.Tables[0].Fields, 2)
}

func TestFetcherClient_GetConnectionSchema_NotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Code: "NOT_FOUND", Message: "connection not found"})
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.GetConnectionSchema(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// --- ValidateSchema tests ---

func TestFetcherClient_ValidateSchema_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/management/connections/validate-schema", r.URL.Path)

		var reqBody ValidateSchemaRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		assert.Equal(t, map[string]map[string][]string{
			"ds-1": {"users": {"id", "name"}},
		}, reqBody.MappedFields)

		resp := ValidateSchemaResponse{
			Status:  "success",
			Message: "All tables and fields validated successfully.",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	result, err := client.ValidateSchema(context.Background(), map[string]map[string][]string{
		"ds-1": {"users": {"id", "name"}},
	})
	require.NoError(t, err)
	assert.True(t, result.IsSuccess())
	assert.Empty(t, result.Errors)
}

func TestFetcherClient_ValidateSchema_WithWarnings(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ValidateSchemaResponse{
			Status:  "failure",
			Message: "Schema validation failed.",
			Errors: []SchemaValidationError{
				{Type: "FIELD_NOT_FOUND", DataSourceID: "ds-1", Table: "users", Field: "nonexistent_field"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	result, err := client.ValidateSchema(context.Background(), map[string]map[string][]string{
		"ds-1": {"users": {"nonexistent_field"}},
	})
	require.NoError(t, err)
	assert.False(t, result.IsSuccess())
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "FIELD_NOT_FOUND", result.Errors[0].Type)
}

func TestFetcherClient_ValidateSchema_422ParsedAsResult(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate fetcher returning 422 with structured validation errors
		errResp := map[string]any{
			"title":   "Schema validation failed",
			"code":    "FET-1060",
			"message": "Schema validation found inconsistencies.",
			"errors": []map[string]string{
				{"type": "TABLE_NOT_FOUND", "dataSourceId": "midaz_transaction", "table": "transaction_abc123"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(errResp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	result, err := client.ValidateSchema(context.Background(), map[string]map[string][]string{
		"midaz_transaction": {"transaction_abc123": {"id"}},
	})
	require.NoError(t, err, "422 with structured errors should not return an error")
	require.NotNil(t, result)
	assert.False(t, result.IsSuccess())
	assert.Equal(t, "failure", result.Status)
	assert.Equal(t, "Schema validation found inconsistencies.", result.Message)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "TABLE_NOT_FOUND", result.Errors[0].Type)
	assert.Equal(t, "midaz_transaction", result.Errors[0].DataSourceID)
	assert.Equal(t, "transaction_abc123", result.Errors[0].Table)
}

func TestFetcherClient_ValidateSchema_422UnparsableFallsBackToError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	result, err := client.ValidateSchema(context.Background(), map[string]map[string][]string{
		"ds-1": {"users": {"id"}},
	})
	require.Error(t, err, "422 with unparsable body should return an error")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "422")
}

func TestFetcherClient_ValidateSchema_500RemainsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal failure"}`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	result, err := client.ValidateSchema(context.Background(), map[string]map[string][]string{
		"ds-1": {"users": {"id"}},
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "500")
}

// --- stubM2MProvider for testing ---

type stubM2MProvider struct {
	token string
	err   error
}

func (s *stubM2MProvider) GetToken(ctx context.Context) (string, error) {
	return s.token, s.err
}
