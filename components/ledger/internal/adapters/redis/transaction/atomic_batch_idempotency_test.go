// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type atomicBatchClaimEvalClient struct {
	redisclient.UniversalClient
	result       any
	err          error
	getValues    map[string]string
	getErrors    map[string]error
	capturedLua  string
	capturedKeys []string
	capturedArgs []any
}

func (client *atomicBatchClaimEvalClient) Get(ctx context.Context, key string) *redisclient.StringCmd {
	cmd := redisclient.NewStringCmd(ctx)
	if err, found := client.getErrors[key]; found {
		cmd.SetErr(err)
	} else if value, found := client.getValues[key]; found {
		cmd.SetVal(value)
	} else {
		cmd.SetErr(redisclient.Nil)
	}

	return cmd
}

func (client *atomicBatchClaimEvalClient) EvalSha(ctx context.Context, _ string, _ []string, _ ...any) *redisclient.Cmd {
	cmd := redisclient.NewCmd(ctx)
	cmd.SetErr(noscriptErr{})

	return cmd
}

func (client *atomicBatchClaimEvalClient) Eval(ctx context.Context, script string, keys []string, args ...any) *redisclient.Cmd {
	client.capturedLua = script
	client.capturedKeys = append([]string(nil), keys...)
	client.capturedArgs = append([]any(nil), args...)

	cmd := redisclient.NewCmd(ctx)
	if client.err != nil {
		cmd.SetErr(client.err)
	} else {
		cmd.SetVal(client.result)
	}

	return cmd
}

func TestClaimAtomicTransactionBatch_ClassifiesAtomicScriptOutcomes(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	claim := atomicBatchIdempotencyClaim("a")
	complete := atomicBatchIdempotencyComplete("a")
	inProgress := claim
	different := atomicBatchIdempotencyComplete("b")

	tests := []struct {
		name        string
		outcome     AtomicTransactionBatchClaimOutcome
		stored      AtomicTransactionBatchIdempotencyRecord
		wantError   bool
		wantBatchID uuid.UUID
	}{
		{
			name:        "first claim",
			outcome:     AtomicTransactionBatchClaimed,
			stored:      claim,
			wantBatchID: claim.BatchID,
		},
		{
			name:        "completed replay",
			outcome:     AtomicTransactionBatchReplayed,
			stored:      complete,
			wantBatchID: complete.BatchID,
		},
		{
			name:        "in-progress conflict",
			outcome:     AtomicTransactionBatchInProgress,
			stored:      inProgress,
			wantError:   true,
			wantBatchID: inProgress.BatchID,
		},
		{
			name:        "changed-fingerprint conflict",
			outcome:     AtomicTransactionBatchFingerprintConflict,
			stored:      different,
			wantError:   true,
			wantBatchID: different.BatchID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			storedJSON, err := json.Marshal(tt.stored)
			require.NoError(t, err)
			client := &atomicBatchClaimEvalClient{result: []any{string(tt.outcome), string(storedJSON)}}
			repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

			result, err := repository.ClaimAtomicTransactionBatch(
				context.Background(), organizationID, ledgerID, "client-key", claim,
			)
			if tt.wantError {
				var conflict pkg.EntityConflictError
				require.ErrorAs(t, err, &conflict)
				assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
			} else {
				require.NoError(t, err)
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.outcome, result.Outcome)
			assert.Equal(t, tt.wantBatchID, result.Record.BatchID)
			assert.Equal(t, utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "client-key"), result.InternalKey)
			assert.Equal(t, claimAtomicTransactionBatchLua, client.capturedLua)
			require.Len(t, client.capturedKeys, 1)
			assert.NotContains(t, client.capturedKeys[0], "client-key")
			require.Len(t, client.capturedArgs, 2, "claim script must not receive or set a nonterminal TTL")
			assert.Equal(t, claim.RequestFingerprint, client.capturedArgs[1])
		})
	}
}

func TestClaimAtomicTransactionBatch_NamespacesHashedKeyByTenant(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	claim := atomicBatchIdempotencyClaim("a")
	payload, err := json.Marshal(claim)
	require.NoError(t, err)
	client := &atomicBatchClaimEvalClient{result: []any{string(AtomicTransactionBatchClaimed), string(payload)}}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	_, err = repository.ClaimAtomicTransactionBatch(ctx, organizationID, ledgerID, "raw-client-key", claim)
	require.NoError(t, err)

	wantInternal := utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "raw-client-key")
	assert.Equal(t, []string{"tenant:tenant-a:" + wantInternal}, client.capturedKeys)
	assert.NotContains(t, client.capturedKeys[0], "raw-client-key")
}

func TestAtomicTransactionBatchIdempotencyRecordValidation(t *testing.T) {
	t.Parallel()

	claim := atomicBatchIdempotencyClaim("a")
	prepared := claim
	prepared.State = AtomicTransactionBatchStatePrepared
	prepared.TransactionIDs = atomicBatchTransactionIDs()
	applied := prepared
	applied.State = AtomicTransactionBatchStateApplied
	applied.ExecutionID = uuidPointer(uuid.MustParse("00000000-0000-0000-0000-000000000020"))
	complete := applied
	complete.State = AtomicTransactionBatchStateComplete
	complete.InitialResponses = atomicBatchInitialResponses(complete.TransactionIDs)
	complete.Response = json.RawMessage(`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[]}`)

	for _, record := range []AtomicTransactionBatchIdempotencyRecord{claim, prepared, applied, complete} {
		require.NoError(t, validateAtomicTransactionBatchIdempotencyRecord(record), record.State)
	}

	legacyApplied := applied
	legacyApplied.FormatVersion = AtomicTransactionBatchLegacyFormatVersion
	require.NoError(t, validateAtomicTransactionBatchIdempotencyRecord(legacyApplied), "legacy applied fixture remains readable")
	legacyApplied.InitialResponses = atomicBatchInitialResponses(legacyApplied.TransactionIDs)
	assert.ErrorContains(t, validateAtomicTransactionBatchIdempotencyRecord(legacyApplied), "legacy record cannot contain")

	invalidVersion := claim
	invalidVersion.FormatVersion++
	assert.Error(t, validateAtomicTransactionBatchIdempotencyRecord(invalidVersion))

	invalidClaim := claim
	invalidClaim.TransactionIDs = atomicBatchTransactionIDs()
	assert.Error(t, validateAtomicTransactionBatchIdempotencyRecord(invalidClaim))

	invalidPrepared := prepared
	invalidPrepared.ExecutionID = uuidPointer(uuid.New())
	assert.Error(t, validateAtomicTransactionBatchIdempotencyRecord(invalidPrepared))

	invalidComplete := complete
	invalidComplete.Response = json.RawMessage(`null`)
	assert.Error(t, validateAtomicTransactionBatchIdempotencyRecord(invalidComplete))

	repeated := prepared
	repeated.TransactionIDs = append([]uuid.UUID(nil), prepared.TransactionIDs...)
	repeated.TransactionIDs[1] = repeated.TransactionIDs[0]
	assert.Error(t, validateAtomicTransactionBatchIdempotencyRecord(repeated))

	partial := applied
	partial.InitialResponses = atomicBatchInitialResponses(partial.TransactionIDs[:1])
	require.NoError(t, validateAtomicTransactionBatchIdempotencyRecord(partial), "applied records allow partial arrival")

	oversized := applied
	oversized.InitialResponses = map[string]string{
		oversized.TransactionIDs[0].String(): base64.StdEncoding.EncodeToString([]byte(`{"payload":"` + strings.Repeat("x", atomicTransactionBatchInitialResponsesMaxBytes) + `"}`)),
	}
	assert.ErrorContains(t, validateAtomicTransactionBatchIdempotencyRecord(oversized), "exceed byte budget")
}

func TestClaimAtomicTransactionBatch_PropagatesRedisAndReplyErrors(t *testing.T) {
	t.Parallel()

	claim := atomicBatchIdempotencyClaim("a")
	redisErr := errors.New("redis unavailable")
	client := &atomicBatchClaimEvalClient{err: redisErr}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	result, err := repository.ClaimAtomicTransactionBatch(
		context.Background(), uuid.New(), uuid.New(), "key", claim,
	)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, redisErr)

	client.err = nil
	client.result = []any{"unknown", `{}`}
	result, err = repository.ClaimAtomicTransactionBatch(
		context.Background(), uuid.New(), uuid.New(), "key", claim,
	)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "unknown atomic transaction batch claim outcome")
}

func TestClaimAtomicTransactionBatch_RejectsInvalidInputBeforeRedis(t *testing.T) {
	t.Parallel()

	client := &atomicBatchClaimEvalClient{}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
	claim := atomicBatchIdempotencyClaim("a")

	for _, input := range []struct {
		name               string
		organizationID     uuid.UUID
		ledgerID           uuid.UUID
		effectiveKey       string
		mutate             func(*AtomicTransactionBatchIdempotencyRecord)
		wantErrorSubstring string
	}{
		{
			name:               "missing organization",
			ledgerID:           uuid.New(),
			effectiveKey:       "key",
			wantErrorSubstring: "scope is required",
		},
		{
			name:               "missing ledger",
			organizationID:     uuid.New(),
			effectiveKey:       "key",
			wantErrorSubstring: "scope is required",
		},
		{
			name:               "missing effective key",
			organizationID:     uuid.New(),
			ledgerID:           uuid.New(),
			wantErrorSubstring: "effective idempotency key is required",
		},
		{
			name:           "non-claimed candidate",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			effectiveKey:   "key",
			mutate: func(record *AtomicTransactionBatchIdempotencyRecord) {
				record.State = AtomicTransactionBatchStatePrepared
			},
			wantErrorSubstring: "must start in claimed state",
		},
	} {
		t.Run(input.name, func(t *testing.T) {
			candidate := claim
			if input.mutate != nil {
				input.mutate(&candidate)
			}

			result, err := repository.ClaimAtomicTransactionBatch(
				context.Background(), input.organizationID, input.ledgerID, input.effectiveKey, candidate,
			)
			assert.Nil(t, result)
			assert.ErrorContains(t, err, input.wantErrorSubstring)
			assert.Empty(t, client.capturedLua)
		})
	}
}

func atomicBatchIdempotencyClaim(fingerprintCharacter string) AtomicTransactionBatchIdempotencyRecord {
	return AtomicTransactionBatchIdempotencyRecord{
		FormatVersion:      AtomicTransactionBatchIdempotencyFormatVersion,
		State:              AtomicTransactionBatchStateClaimed,
		RequestFingerprint: strings.Repeat(fingerprintCharacter, 64),
		OwnerToken:         "owner-token",
		BatchID:            uuid.MustParse("00000000-0000-0000-0000-000000000010"),
	}
}

func atomicBatchIdempotencyComplete(fingerprintCharacter string) AtomicTransactionBatchIdempotencyRecord {
	record := atomicBatchIdempotencyClaim(fingerprintCharacter)
	record.FormatVersion = AtomicTransactionBatchLegacyFormatVersion
	record.State = AtomicTransactionBatchStateComplete
	record.ExecutionID = uuidPointer(uuid.MustParse("00000000-0000-0000-0000-000000000020"))
	record.TransactionIDs = atomicBatchTransactionIDs()
	record.Response = json.RawMessage(`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[]}`)

	return record
}

func atomicBatchTransactionIDs() []uuid.UUID {
	return []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000030"),
		uuid.MustParse("00000000-0000-0000-0000-000000000031"),
	}
}

func atomicBatchInitialResponses(transactionIDs []uuid.UUID) map[string]string {
	responses := make(map[string]string, len(transactionIDs))
	for index, transactionID := range transactionIDs {
		responses[transactionID.String()] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`{"id":"initial-%d"}`, index)))
	}

	return responses
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	return &value
}
