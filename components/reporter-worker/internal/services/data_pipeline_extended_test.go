// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestDownloadExtractedData_NilStorage(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:             log.NewNop(),
		Tracer:             noop.NewTracerProvider().Tracer("test"),
		FetcherDataStorage: nil,
	}

	_, err := uc.downloadExtractedData(context.Background(), "/data/test.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetcher data storage client is not configured")
}

func TestDownloadExtractedData_DownloadError(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
		FetcherDataStorage: &mockFetcherDataDownloader{
			downloadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("S3 connection refused")
			},
		},
	}

	_, err := uc.downloadExtractedData(context.Background(), "/data/test.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download extracted data")
}

func TestDownloadExtractedData_Success(t *testing.T) {
	t.Parallel()

	expected := []byte(`{"db":{"table":[{"id":"1"}]}}`)

	uc := &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
		FetcherDataStorage: &mockFetcherDataDownloader{
			downloadFunc: func(_ context.Context, path string) ([]byte, error) {
				assert.Equal(t, "/data/test.json", path)
				return expected, nil
			},
		},
	}

	data, err := uc.downloadExtractedData(context.Background(), "/data/test.json")
	require.NoError(t, err)
	assert.Equal(t, expected, data)
}

func TestDecryptExtractedData_NoKey_ReturnsRawData(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		StorageDecryptKey: nil,
	}

	raw := []byte(`{"unencrypted":"data"}`)
	result, err := uc.decryptExtractedData(context.Background(), raw)
	require.NoError(t, err)
	assert.Equal(t, raw, result, "with no decrypt key, data should be returned as-is")
}

func TestDecryptExtractedData_EmptyKey_ReturnsRawData(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		StorageDecryptKey: []byte{},
	}

	raw := []byte(`{"unencrypted":"data"}`)
	result, err := uc.decryptExtractedData(context.Background(), raw)
	require.NoError(t, err)
	assert.Equal(t, raw, result)
}

func TestDecryptExtractedData_InvalidCiphertext(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		StorageDecryptKey: []byte("0123456789abcdef0123456789abcdef"),
	}

	_, err := uc.decryptExtractedData(context.Background(), []byte("not-valid-ciphertext"))
	require.Error(t, err)
	// decryptExtractedData now propagates the inner typed error directly (no double-wrapping).
	// The inner error from decryptFetcherData describes the base64 decode failure.
	assert.Contains(t, err.Error(), "base64 decode encrypted data")
}

func TestAuditHMAC_EmptyReceivedHMAC_Skips(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: []byte("some-key"),
	}

	// Should not panic and should skip verification
	require.NotPanics(t, func() {
		uc.auditHMAC(context.Background(), []byte("data"), "")
	})
}

func TestAuditHMAC_NoKey_Skips(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: nil,
	}

	require.NotPanics(t, func() {
		uc.auditHMAC(context.Background(), []byte("data"), "some-hmac")
	})
}

func TestAuditHMAC_EmptyKey_Skips(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: []byte{},
	}

	require.NotPanics(t, func() {
		uc.auditHMAC(context.Background(), []byte("data"), "some-hmac")
	})
}

func TestAuditHMAC_WithKeyAndHMAC_DoesNotPanic(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: []byte("test-hmac-key-for-verification"),
	}

	require.NotPanics(t, func() {
		uc.auditHMAC(context.Background(), []byte("test data"), "invalid-hmac-value")
	})
}

func TestParseExtractedData_ValidJSON(t *testing.T) {
	t.Parallel()

	input := map[string]map[string][]map[string]any{
		"db": {
			"users": {
				{"id": "1", "name": "Alice"},
				{"id": "2", "name": "Bob"},
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	result, err := parseExtractedData(data)
	require.NoError(t, err)
	assert.Len(t, result["db"]["users"], 2)
	assert.Equal(t, "Alice", result["db"]["users"][0]["name"])
}

func TestParseExtractedData_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := parseExtractedData([]byte(`{invalid`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal extracted data")
}

func TestParseExtractedData_EmptyJSON(t *testing.T) {
	t.Parallel()

	result, err := parseExtractedData([]byte(`{}`))
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestConvertResultSchemaNotation_ConvertsSchemaKeys(t *testing.T) {
	t.Parallel()

	input := map[string]map[string][]map[string]any{
		"db1": {
			"public.users": {
				{"id": "1"},
			},
			"public.orders": {
				{"id": "2"},
			},
		},
	}

	result := convertResultSchemaNotation(input)

	assert.NotNil(t, result["db1"])
	assert.Contains(t, result["db1"], "public__users")
	assert.Contains(t, result["db1"], "public__orders")
	assert.NotContains(t, result["db1"], "public.users")
	assert.NotContains(t, result["db1"], "public.orders")
}

func TestConvertResultSchemaNotation_NoSchemaPrefix(t *testing.T) {
	t.Parallel()

	input := map[string]map[string][]map[string]any{
		"db1": {
			"users": {
				{"id": "1"},
			},
		},
	}

	result := convertResultSchemaNotation(input)

	assert.Contains(t, result["db1"], "users")
}

func TestConvertResultSchemaNotation_EmptyResult(t *testing.T) {
	t.Parallel()

	result := convertResultSchemaNotation(map[string]map[string][]map[string]any{})
	assert.Empty(t, result)
}

func TestConvertResultSchemaNotation_MultipleDBs(t *testing.T) {
	t.Parallel()

	input := map[string]map[string][]map[string]any{
		"db1": {
			"schema1.table1": {{"id": "1"}},
		},
		"db2": {
			"schema2.table2": {{"id": "2"}},
		},
	}

	result := convertResultSchemaNotation(input)

	assert.Contains(t, result["db1"], "schema1__table1")
	assert.Contains(t, result["db2"], "schema2__table2")
}
