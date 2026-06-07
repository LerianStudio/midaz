// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

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

// D7 posture (c): key NOT configured -> skip verification, return nil (deployments
// without HMAC stay functional).
func TestVerifyHMACOrReject_NoKey_Skips(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: nil,
	}

	err := uc.verifyHMACOrReject(context.Background(), []byte("data"), "some-hmac")
	require.NoError(t, err, "no key configured must skip verification and proceed")
}

func TestVerifyHMACOrReject_EmptyKey_Skips(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: []byte{},
	}

	err := uc.verifyHMACOrReject(context.Background(), []byte("data"), "some-hmac")
	require.NoError(t, err, "empty key configured must skip verification and proceed")
}

// D7 posture (b): signature ABSENT but key CONFIGURED -> REJECT (a producer that
// should sign didn't).
func TestVerifyHMACOrReject_KeyConfiguredSignatureAbsent_Rejects(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: []byte("test-hmac-secret-key"),
	}

	err := uc.verifyHMACOrReject(context.Background(), []byte("data"), "")
	require.Error(t, err, "key configured but signature absent must reject")
	assert.True(t, pkg.IsBusinessError(err), "rejection must be a business-typed (non-retryable) error")

	var unauthorized pkg.UnauthorizedError
	require.ErrorAs(t, err, &unauthorized)
	assert.Equal(t, constant.ErrCodeInvalidMessageSignature.Error(), unauthorized.Code)
}

// D7 posture (a): mismatch (key configured, signature present, verification fails)
// -> REJECT with a permanent (non-retryable) error.
func TestVerifyHMACOrReject_Mismatch_Rejects(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: []byte("test-hmac-secret-key"),
	}

	err := uc.verifyHMACOrReject(context.Background(), []byte("test data"), "deadbeefdeadbeef")
	require.Error(t, err, "key configured, signature present but invalid must reject")
	assert.True(t, pkg.IsBusinessError(err), "rejection must be a business-typed (non-retryable) error")

	var unauthorized pkg.UnauthorizedError
	require.ErrorAs(t, err, &unauthorized)
	assert.Equal(t, constant.ErrCodeInvalidMessageSignature.Error(), unauthorized.Code)
}

// Valid signature with key configured -> proceed.
func TestVerifyHMACOrReject_ValidSignature_Proceeds(t *testing.T) {
	t.Parallel()

	key := []byte("test-hmac-secret-key")
	data := []byte(`{"accounts":[{"id":1}]}`)
	validHMAC := computeHMACForTest(t, data, key)

	uc := &UseCase{
		Logger:          log.NewNop(),
		Tracer:          noop.NewTracerProvider().Tracer("test"),
		ExternalHMACKey: key,
	}

	err := uc.verifyHMACOrReject(context.Background(), data, validHMAC)
	require.NoError(t, err, "valid signature with key configured must proceed")
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
