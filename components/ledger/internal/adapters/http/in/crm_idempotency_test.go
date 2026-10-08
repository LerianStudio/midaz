// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
)

// newTestFieldEncryptor is the production legacy-mode field encryptor
// (KMS_VENDOR=none) that seals CRM idempotency slots.
func newTestFieldEncryptor(t *testing.T) encryption.FieldEncryptor {
	t.Helper()

	metrics := encryption.NewProtectionMetrics(nil)
	resolver := encryption.NewProtectionStateResolver(nil, metrics)

	return encryption.NewFieldEncryptorAdapter(encryption.NewEncryptionService(resolver, nil, nil, testutils.SetupCrypto(t), metrics))
}

// postCRMCreate sends a CRM create to the test app with the given request
// headers and returns the status, the X-Idempotency-Replayed header and the
// decoded body.
func postCRMCreate(t *testing.T, app *fiber.App, path string, headers map[string]string, body string) (int, string, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

	return resp.StatusCode, resp.Header.Get("X-Idempotency-Replayed"), got
}

// withIdempotencyKey returns the request headers carrying X-Idempotency: key.
func withIdempotencyKey(key string) map[string]string {
	return map[string]string{"X-Idempotency": key}
}

// fakeCRMIdempotencyRepo is an in-memory IdempotencyRepo with SetNX semantics,
// shared by the CRM handler tests whose flows claim an idempotency slot. One
// instance is shared across the requests of a single replay test so the second
// request sees the first one's claim. claimErr and delErr inject claim and
// release failures.
type fakeCRMIdempotencyRepo struct {
	store    map[string]string
	claimErr error
	delErr   error
}

func newFakeCRMIdempotencyRepo() *fakeCRMIdempotencyRepo {
	return &fakeCRMIdempotencyRepo{store: make(map[string]string)}
}

func (f *fakeCRMIdempotencyRepo) SetNX(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	if f.claimErr != nil {
		return false, f.claimErr
	}

	if _, ok := f.store[key]; ok {
		return false, nil
	}

	f.store[key] = value

	return true, nil
}

func (f *fakeCRMIdempotencyRepo) Get(_ context.Context, key string) (string, error) {
	value, ok := f.store[key]
	if !ok {
		return "", redis.Nil
	}

	return value, nil
}

func (f *fakeCRMIdempotencyRepo) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.store[key] = value

	return nil
}

func (f *fakeCRMIdempotencyRepo) Del(_ context.Context, key string) error {
	if f.delErr != nil {
		return f.delErr
	}

	delete(f.store, key)

	return nil
}
