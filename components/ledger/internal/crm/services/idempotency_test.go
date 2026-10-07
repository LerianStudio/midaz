// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIdempotencyRepo is an in-memory IdempotencyRepo with SetNX semantics.
// setErr/getErr/delErr inject failures for the error-path assertions.
type fakeIdempotencyRepo struct {
	store  map[string]string
	setErr error
	getErr error
	delErr error
}

func newFakeIdempotencyRepo() *fakeIdempotencyRepo {
	return &fakeIdempotencyRepo{store: make(map[string]string)}
}

func (f *fakeIdempotencyRepo) SetNX(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	if f.setErr != nil {
		return false, f.setErr
	}

	if _, ok := f.store[key]; ok {
		return false, nil
	}

	f.store[key] = value

	return true, nil
}

func (f *fakeIdempotencyRepo) Get(_ context.Context, key string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}

	value, ok := f.store[key]
	if !ok {
		return "", redis.Nil
	}

	return value, nil
}

func (f *fakeIdempotencyRepo) Set(_ context.Context, key, value string, _ time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}

	f.store[key] = value

	return nil
}

func (f *fakeIdempotencyRepo) Del(_ context.Context, key string) error {
	if f.delErr != nil {
		return f.delErr
	}

	delete(f.store, key)

	return nil
}

// newTestReplayEncryptor is the production legacy-mode field encryptor (KMS_VENDOR=none).
func newTestReplayEncryptor(t *testing.T) encryption.FieldEncryptor {
	t.Helper()

	metrics := encryption.NewProtectionMetrics(nil)
	resolver := encryption.NewProtectionStateResolver(nil, metrics)

	return encryption.NewFieldEncryptorAdapter(encryption.NewEncryptionService(resolver, nil, nil, testutils.SetupCrypto(t), metrics))
}

const (
	testIdempotencyOrg  = "org-1"
	testIdempotencyKey  = "idempotency:crm:holder:org-1:key-1"
	testIdempotencyHash = "hash-1"
	testIdempotencyTTL  = 300 * time.Second
)

func TestCreateOrCheckCRMIdempotency_FreshClaim(t *testing.T) {
	uc := &UseCase{Idempotency: newFakeIdempotencyRepo()}

	res, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)

	require.NoError(t, err)
	assert.Nil(t, res.Replay)
}

func TestCreateOrCheckCRMIdempotency_ReplayHit(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	uc := &UseCase{Idempotency: repo, Encryptor: newTestReplayEncryptor(t)}
	entity := `{"id":"abc","name":"Maria Silva","document":"91315026015"}`

	// First call claims the slot.
	first, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	require.Nil(t, first.Replay)

	// Store the created entity value, then retry with the same key.
	uc.SetCRMIdempotencyValue(context.Background(), testIdempotencyOrg, testIdempotencyKey, entity, testIdempotencyTTL)

	stored := repo.store[testIdempotencyKey]
	require.NotEmpty(t, stored)
	assert.NotContains(t, stored, "Maria Silva", "the cached entity must not hold personal data in clear")
	assert.NotContains(t, stored, "91315026015", "the cached entity must not hold personal data in clear")

	second, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	require.NotNil(t, second.Replay)
	assert.Equal(t, entity, *second.Replay)
}

func TestCreateOrCheckCRMIdempotency_UnreadableReplayFails(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	repo.store[testIdempotencyKey] = `{"id":"abc"}`
	uc := &UseCase{Idempotency: repo, Encryptor: newTestReplayEncryptor(t)}

	res, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)

	require.Error(t, err)
	assert.False(t, pkg.IsBusinessError(err))
	assert.Nil(t, res.Replay)
}

func TestCreateOrCheckCRMIdempotency_InFlight(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	uc := &UseCase{Idempotency: repo}

	// First call claims the slot but stores no value yet (in-flight).
	first, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	require.Nil(t, first.Replay)

	// Second concurrent call sees a claimed-but-empty slot.
	second, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.Error(t, err)
	assert.Nil(t, second.Replay)
	assert.True(t, pkg.IsBusinessError(err))

	// ValidateBusinessError returns a typed struct carrying the sentinel's code,
	// not the sentinel itself, so assert on the code.
	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
}

func TestCreateOrCheckCRMIdempotency_DisabledPassthrough(t *testing.T) {
	uc := &UseCase{Idempotency: nil}

	res, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)

	require.NoError(t, err)
	assert.Nil(t, res.Replay)
}

func TestSetCRMIdempotencyValue_DisabledNoOp(t *testing.T) {
	uc := &UseCase{Idempotency: nil}

	// Must not panic and must not error.
	uc.SetCRMIdempotencyValue(context.Background(), testIdempotencyOrg, testIdempotencyKey, `{"id":"abc"}`, testIdempotencyTTL)
}

func TestReleaseCRMIdempotency_DisabledNoOp(t *testing.T) {
	uc := &UseCase{Idempotency: nil}

	// Must not panic.
	uc.ReleaseCRMIdempotency(context.Background(), testIdempotencyKey)
}

func TestReleaseCRMIdempotency_RemovesClaim(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	uc := &UseCase{Idempotency: repo}

	first, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	require.Nil(t, first.Replay)
	require.Contains(t, repo.store, testIdempotencyKey)

	uc.ReleaseCRMIdempotency(context.Background(), testIdempotencyKey)

	assert.NotContains(t, repo.store, testIdempotencyKey)

	// The released slot is claimable again instead of answering in-flight.
	second, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyOrg, testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	assert.Nil(t, second.Replay)
}

func TestReleaseCRMIdempotency_DelFailureSwallowed(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	repo.store[testIdempotencyKey] = ""
	repo.delErr = errors.New("redis unavailable")
	uc := &UseCase{Idempotency: repo}

	// Returns nothing: the failure must not reach the caller.
	uc.ReleaseCRMIdempotency(context.Background(), testIdempotencyKey)

	assert.Contains(t, repo.store, testIdempotencyKey, "a failed release leaves the slot to its TTL")
}

func TestCRMIdempotencyKeyBuilders(t *testing.T) {
	assert.Equal(t, "idempotency:crm:holder:org-1:key-1", HolderIdempotencyKey("org-1", "key-1"))
	assert.Equal(t, "idempotency:crm:instrument:org-1:holder-1:key-1", InstrumentIdempotencyKey("org-1", "holder-1", "key-1"))
}
