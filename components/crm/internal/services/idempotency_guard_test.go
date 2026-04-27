// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/idempotency"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type probeResponse struct {
	Value string `json:"value"`
}

func TestExecuteIdempotent_EmptyKey_BypassesGuardAndRunsFn(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := idempotency.NewMockRepository(ctrl)
	// No Find/Store expectations: the guard must be a pure pass-through.

	calls := 0
	fn := func(_ context.Context) (*probeResponse, error) {
		calls++
		return &probeResponse{Value: "ran"}, nil
	}

	got, err := ExecuteIdempotent(context.Background(), repo, "", "hash-doesnt-matter", 0, fn)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "ran", got.Value)
	assert.Equal(t, 1, calls, "fn must run exactly once when guard is bypassed")
}

func TestExecuteIdempotent_NewKey_RunsFnAndStoresResult(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := idempotency.NewMockRepository(ctrl)

	// First call: no existing record, so Find returns (nil, nil).
	repo.EXPECT().
		Find(gomock.Any(), gomock.Any(), "idem-key-42").
		Return(nil, nil).
		Times(1)

	// Store must be invoked exactly once with the hash and serialized result.
	repo.EXPECT().
		Store(gomock.Any(), gomock.AssignableToTypeOf(&idempotency.Record{})).
		DoAndReturn(func(_ context.Context, rec *idempotency.Record) error {
			assert.Equal(t, "idem-key-42", rec.IdempotencyKey)
			assert.Equal(t, "request-hash-xyz", rec.RequestHash)

			var decoded probeResponse
			require.NoError(t, json.Unmarshal(rec.ResponseDocument, &decoded))
			assert.Equal(t, "fresh", decoded.Value)

			return nil
		}).
		Times(1)

	calls := 0
	fn := func(_ context.Context) (*probeResponse, error) {
		calls++
		return &probeResponse{Value: "fresh"}, nil
	}

	got, err := ExecuteIdempotent(context.Background(), repo, "idem-key-42", "request-hash-xyz", 0, fn)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "fresh", got.Value)
	assert.Equal(t, 1, calls, "fn must run exactly once on a fresh key")
}

func TestExecuteIdempotent_SameKeySameHash_ReturnsCachedResponse(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := idempotency.NewMockRepository(ctrl)

	cachedDoc, err := json.Marshal(probeResponse{Value: "cached"})
	require.NoError(t, err)

	repo.EXPECT().
		Find(gomock.Any(), gomock.Any(), "idem-key-1").
		Return(&idempotency.Record{
			TenantID:         "tenant-a",
			IdempotencyKey:   "idem-key-1",
			RequestHash:      "matching-hash",
			ResponseDocument: cachedDoc,
		}, nil).
		Times(1)

	// Store MUST NOT be called on a cache hit.

	calls := 0
	fn := func(_ context.Context) (*probeResponse, error) {
		calls++
		return &probeResponse{Value: "should-not-run"}, nil
	}

	got, err := ExecuteIdempotent(context.Background(), repo, "idem-key-1", "matching-hash", 0, fn)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "cached", got.Value)
	assert.Equal(t, 0, calls, "fn must NOT run on a cache hit")
}

func TestExecuteIdempotent_SameKeyDifferentHash_ReturnsConflict(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := idempotency.NewMockRepository(ctrl)

	repo.EXPECT().
		Find(gomock.Any(), gomock.Any(), "idem-key-collision").
		Return(&idempotency.Record{
			TenantID:         "tenant-a",
			IdempotencyKey:   "idem-key-collision",
			RequestHash:      "original-hash",
			ResponseDocument: []byte(`{}`),
		}, nil).
		Times(1)

	calls := 0
	fn := func(_ context.Context) (*probeResponse, error) {
		calls++
		return nil, nil
	}

	got, err := ExecuteIdempotent(context.Background(), repo, "idem-key-collision", "different-hash", 0, fn)

	require.Error(t, err)
	require.Nil(t, got)
	assert.Equal(t, 0, calls, "fn must NOT run when the hash conflicts")

	// The returned error must be the ErrIdempotencyKey business error (0084).
	conflictErr, ok := err.(pkg.EntityConflictError)
	require.True(t, ok, "expected EntityConflictError for ErrIdempotencyKey, got %T", err)
	assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflictErr.Code)
}

func TestExecuteIdempotent_FnError_DoesNotStore(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := idempotency.NewMockRepository(ctrl)

	repo.EXPECT().
		Find(gomock.Any(), gomock.Any(), "idem-key-errs").
		Return(nil, nil).
		Times(1)

	// Store MUST NOT be called when fn returns an error — we want retries
	// with the same key to succeed on a transient failure.

	boom := errors.New("downstream exploded")
	fn := func(_ context.Context) (*probeResponse, error) {
		return nil, boom
	}

	got, err := ExecuteIdempotent(context.Background(), repo, "idem-key-errs", "h", 0, fn)

	require.ErrorIs(t, err, boom)
	require.Nil(t, got)
}
