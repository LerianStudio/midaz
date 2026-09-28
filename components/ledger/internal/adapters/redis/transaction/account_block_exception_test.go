// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestBuildAccountBlockExceptionEntry_KeyAndValue locks the two halves of the
// cache contract the transaction script reads back: the key shape (namespaced,
// {transactions}-slotted) and the CamelCase JSON the Lua cjson decode expects.
//
// The casing is asserted on the RAW BYTES, not on a round-trip through the Go
// struct: a struct-tag rename would round-trip cleanly while silently breaking
// the Lua decode, and only a byte-level assertion catches that.
func TestBuildAccountBlockExceptionEntry_KeyAndValue(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	exceptionID := uuid.MustParse("018f2c1e-6a3b-7c4d-8e5f-0a1b2c3d4e5f")

	key, value, err := buildAccountBlockExceptionEntry(context.Background(), orgID, ledgerID,
		AccountBlockException{ID: exceptionID, Alias: "@fraud_account", Amount: "150", TTL: 300 * time.Second})
	require.NoError(t, err)

	assert.Equal(t, utils.AccountBlockExceptionInternalKey(orgID, ledgerID, exceptionID), key,
		"single-tenant context leaves the internal key unprefixed")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(value, &decoded))

	assert.Equal(t, map[string]any{"Alias": "@fraud_account", "Amount": "150"}, decoded,
		"the blob must carry exactly the CamelCase Alias/Amount pair the Lua decode reads")
}

// TestBuildAccountBlockExceptionEntry_AmountIsStoredAsGiven proves the adapter
// does NOT re-normalize the amount: canonicalization is the command's job (it
// owns the decimal parse), so the adapter storing something else would silently
// desynchronize the two sides of the consumption comparison.
func TestBuildAccountBlockExceptionEntry_AmountIsStoredAsGiven(t *testing.T) {
	t.Parallel()

	_, value, err := buildAccountBlockExceptionEntry(context.Background(), uuid.New(), uuid.New(),
		AccountBlockException{ID: uuid.New(), Alias: "@a", Amount: "0.01", TTL: time.Second})
	require.NoError(t, err)

	var decoded struct {
		Amount string `json:"Amount"`
	}

	require.NoError(t, json.Unmarshal(value, &decoded))
	assert.Equal(t, "0.01", decoded.Amount)
}

// TestCreateAccountBlockExceptions_EmptyBatchIsNoOp locks the guard that keeps an
// empty batch from touching Redis at all: the repository has a nil connection
// here, so any client acquisition would panic.
func TestCreateAccountBlockExceptions_EmptyBatchIsNoOp(t *testing.T) {
	t.Parallel()

	repo := &RedisConsumerRepository{}

	require.NoError(t, repo.CreateAccountBlockExceptions(context.Background(),
		uuid.New(), uuid.New(), nil))
	require.NoError(t, repo.CreateAccountBlockExceptions(context.Background(),
		uuid.New(), uuid.New(), []AccountBlockException{}))
}
