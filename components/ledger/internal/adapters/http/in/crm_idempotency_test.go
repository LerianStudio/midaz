// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

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

// fakeCRMIdempotencyRepo is an in-memory IdempotencyRepo with SetNX semantics,
// shared by the CRM handler tests whose flows claim an idempotency slot. One
// instance is shared across the requests of a single replay test so the second
// request sees the first one's claim. delErr injects a release failure.
type fakeCRMIdempotencyRepo struct {
	store  map[string]string
	delErr error
}

func newFakeCRMIdempotencyRepo() *fakeCRMIdempotencyRepo {
	return &fakeCRMIdempotencyRepo{store: make(map[string]string)}
}

func (f *fakeCRMIdempotencyRepo) SetNX(_ context.Context, key, value string, _ time.Duration) (bool, error) {
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
