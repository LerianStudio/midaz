// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIdempotencyRepo is an in-memory IdempotencyRepo with SetNX semantics.
// setErr/getErr inject failures for the error-path assertions.
type fakeIdempotencyRepo struct {
	store  map[string]string
	setErr error
	getErr error
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

const (
	testIdempotencyKey  = "idempotency:crm:holder:org-1:key-1"
	testIdempotencyHash = "hash-1"
	testIdempotencyTTL  = 300 * time.Second
)

func TestCreateOrCheckCRMIdempotency_FreshClaim(t *testing.T) {
	uc := &UseCase{Idempotency: newFakeIdempotencyRepo()}

	res, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)

	require.NoError(t, err)
	assert.Nil(t, res.Replay)
}

func TestCreateOrCheckCRMIdempotency_ReplayHit(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	uc := &UseCase{Idempotency: repo}

	// First call claims the slot.
	first, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	require.Nil(t, first.Replay)

	// Store the created entity value, then retry with the same key.
	uc.SetCRMIdempotencyValue(context.Background(), testIdempotencyKey, `{"id":"abc"}`, testIdempotencyTTL)

	second, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	require.NotNil(t, second.Replay)
	assert.Equal(t, `{"id":"abc"}`, *second.Replay)
}

func TestCreateOrCheckCRMIdempotency_InFlight(t *testing.T) {
	repo := newFakeIdempotencyRepo()
	uc := &UseCase{Idempotency: repo}

	// First call claims the slot but stores no value yet (in-flight).
	first, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
	require.NoError(t, err)
	require.Nil(t, first.Replay)

	// Second concurrent call sees a claimed-but-empty slot.
	second, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)
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

	res, err := uc.CreateOrCheckCRMIdempotency(context.Background(), testIdempotencyKey, testIdempotencyHash, testIdempotencyTTL)

	require.NoError(t, err)
	assert.Nil(t, res.Replay)
}

func TestSetCRMIdempotencyValue_DisabledNoOp(t *testing.T) {
	uc := &UseCase{Idempotency: nil}

	// Must not panic and must not error.
	uc.SetCRMIdempotencyValue(context.Background(), testIdempotencyKey, `{"id":"abc"}`, testIdempotencyTTL)
}

func TestCRMIdempotencyKeyBuilders(t *testing.T) {
	assert.Equal(t, "idempotency:crm:holder:org-1:key-1", HolderIdempotencyKey("org-1", "key-1"))
	assert.Equal(t, "idempotency:crm:instrument:org-1:holder-1:key-1", InstrumentIdempotencyKey("org-1", "holder-1", "key-1"))
}
