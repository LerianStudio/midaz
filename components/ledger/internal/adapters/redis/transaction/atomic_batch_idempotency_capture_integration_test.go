//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegrationAtomicTransactionBatchInitialResponseCaptureCAS(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newAtomicBatchRecoveryAckFixture(t, container.Client)
	firstID := fixture.record.TransactionIDs[0]
	secondID := fixture.record.TransactionIDs[1]
	first := json.RawMessage(`{"id":"first","status":"CREATED"}`)
	second := json.RawMessage(`{"id":"second","status":"PENDING"}`)

	result, err := fixture.repository.CaptureAtomicTransactionBatchInitialResponse(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.OwnerToken,
		secondID,
		second,
	)
	require.NoError(t, err)
	require.Equal(t, AtomicTransactionBatchInitialResponseCaptured, result.Outcome)
	require.Equal(t, AtomicTransactionBatchIdempotencyFormatVersion, result.Record.FormatVersion)
	require.Len(t, result.Record.InitialResponses, 1)

	result, err = fixture.repository.CaptureAtomicTransactionBatchInitialResponse(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.OwnerToken,
		firstID,
		first,
	)
	require.NoError(t, err)
	require.Equal(t, AtomicTransactionBatchInitialResponseCaptured, result.Outcome)
	require.Len(t, result.Record.InitialResponses, 2)

	duplicate, err := fixture.repository.CaptureAtomicTransactionBatchInitialResponse(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.OwnerToken,
		secondID,
		second,
	)
	require.NoError(t, err)
	assert.Equal(t, AtomicTransactionBatchInitialResponseAlreadyCaptured, duplicate.Outcome)

	conflict, err := fixture.repository.CaptureAtomicTransactionBatchInitialResponse(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.OwnerToken,
		secondID,
		json.RawMessage(`{"id":"second","status":"COMMITTED"}`),
	)
	require.Error(t, err)
	require.NotNil(t, conflict)
	assert.Equal(t, AtomicTransactionBatchInitialResponseConflict, conflict.Outcome)

	storedRaw, err := container.Client.Get(fixture.ctx, fixture.recordKey).Bytes()
	require.NoError(t, err)
	var stored AtomicTransactionBatchIdempotencyRecord
	require.NoError(t, json.Unmarshal(storedRaw, &stored))
	assert.Equal(t, AtomicTransactionBatchIdempotencyFormatVersion, stored.FormatVersion)
	assert.Equal(t, base64.StdEncoding.EncodeToString(first), stored.InitialResponses[firstID.String()])
	assert.Equal(t, base64.StdEncoding.EncodeToString(second), stored.InitialResponses[secondID.String()])
	assert.NotContains(t, string(storedRaw), `"order"`)

	// A capture failure must not remove recovery evidence. ACK is deliberately
	// separate and can only run after this immutable source is accepted.
	assert.True(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[0]).Val())
	assert.True(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[1]).Val())
}
