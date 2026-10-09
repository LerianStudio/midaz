// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicTransactionBatchRequestIdentity(t *testing.T) {
	t.Parallel()

	rawSource := []byte(`{"amount":"100"}`)
	rawFingerprint := sha256HexOf(rawSource)
	keyedFingerprint := strings.Repeat("c", 64)
	batchFingerprint := strings.Repeat("b", 64)

	for _, tt := range []struct {
		name            string
		input           CreateAtomicTransactionBatchV2Input
		wantFingerprint string
		wantKey         string
		wantLegacy      string
	}{
		{
			name: "a keyed request stores its keyed fingerprint and accepts the raw one",
			input: CreateAtomicTransactionBatchV2Input{
				CanonicalRequest: rawSource, KeyedRequestFingerprint: keyedFingerprint, IdempotencyKey: " client-key ",
			},
			wantFingerprint: keyedFingerprint,
			wantKey:         "client-key",
			wantLegacy:      rawFingerprint,
		},
		{
			name: "a request without a key ignores the keyed fingerprint",
			input: CreateAtomicTransactionBatchV2Input{
				CanonicalRequest: rawSource, KeyedRequestFingerprint: keyedFingerprint,
			},
			wantFingerprint: rawFingerprint,
			wantKey:         rawFingerprint,
		},
		{
			name: "a blank key counts as no key",
			input: CreateAtomicTransactionBatchV2Input{
				CanonicalRequest: rawSource, KeyedRequestFingerprint: keyedFingerprint, IdempotencyKey: "  ",
			},
			wantFingerprint: rawFingerprint,
			wantKey:         rawFingerprint,
		},
		{
			name: "a keyed batch keeps its own fingerprint",
			input: CreateAtomicTransactionBatchV2Input{
				CanonicalRequest: rawSource, RequestFingerprint: batchFingerprint, IdempotencyKey: "batch-key",
			},
			wantFingerprint: batchFingerprint,
			wantKey:         "batch-key",
		},
		{
			name: "a batch without a key is named by its own fingerprint",
			input: CreateAtomicTransactionBatchV2Input{
				CanonicalRequest: rawSource, RequestFingerprint: batchFingerprint,
			},
			wantFingerprint: batchFingerprint,
			wantKey:         batchFingerprint,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fingerprint, effectiveKey, legacy, err := atomicTransactionBatchRequestIdentity(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFingerprint, fingerprint)
			assert.Equal(t, tt.wantKey, effectiveKey)
			assert.Equal(t, tt.wantLegacy, legacy)
		})
	}
}

func TestClaimAtomicTransactionBatch_KeyedCrossLedgerRequestClaimsItsKeyedFingerprint(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("01995190-0000-7000-8000-000000000011")
	ledgerID := uuid.MustParse("01995190-0000-7000-8000-000000000012")
	rawSource := []byte(`{"amount":"100"}`)
	keyedFingerprint := strings.Repeat("c", 64)

	repository := &atomicTransactionBatchClaimRepositoryFake{}
	uc := &UseCase{AtomicTransactionBatchIdempotencyRepo: repository}
	run := &atomicTransactionBatchRun{
		batchID:                    uuid.MustParse("01995190-0000-7000-8000-000000000013"),
		coordinationOrganizationID: organizationID,
		coordinationLedgerID:       ledgerID,
	}

	replay, err := uc.claimAtomicTransactionBatch(context.Background(), CreateAtomicTransactionBatchV2Input{
		CanonicalRequest:        rawSource,
		KeyedRequestFingerprint: keyedFingerprint,
		IdempotencyKey:          "client-key",
	}, run)
	require.NoError(t, err)
	assert.Nil(t, replay)

	assert.Equal(t, "client-key", repository.effectiveKey)
	assert.Equal(t, keyedFingerprint, repository.claim.RequestFingerprint)
	assert.Equal(t, sha256HexOf(rawSource), repository.legacyFingerprint)
	assert.Equal(t, keyedFingerprint, run.idempotencyFingerprint, "later transitions must carry the stored fingerprint")
}

func sha256HexOf(source []byte) string {
	digest := sha256.Sum256(source)

	return hex.EncodeToString(digest[:])
}
