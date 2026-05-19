// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeleteAccountTypeStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the account-type.deleted emission.
// AccountTypeRepo.Delete returns nil for the success path.
func newDeleteAccountTypeStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		Streaming:       emitter,
	}
}

// TestDeleteAccountTypeByID_EmitsAccountTypeDeletedEvent verifies that
// a successful DeleteAccountTypeByID call publishes exactly one
// account-type.deleted event with the expected resource/event types,
// tenant ID, subject and payload fields.
func TestDeleteAccountTypeByID_EmitsAccountTypeDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	accountTypeID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteAccountTypeStreamingTestUseCase(t, ctrl, mockEmitter)

	before := time.Now()
	err := uc.DeleteAccountTypeByID(context.Background(), orgID, ledgerID, accountTypeID)
	after := time.Now()
	require.NoError(t, err)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account-type", "deleted")

	evt := events[0]
	assert.Equal(t, "account-type.deleted", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, accountTypeID.String(), evt.Subject, "Subject must be the deleted account type ID")

	// Timestamp must be the wall-clock instant captured at the emit
	// site — bounded by the test's before/after window.
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, accountTypeID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.NotEmpty(t, payload["deletedAt"], "deletedAt must be set (RFC3339)")
}

// TestDeleteAccountTypeByID_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path: when Streaming is the NoopEmitter,
// DeleteAccountTypeByID succeeds without error and no panic.
func TestDeleteAccountTypeByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteAccountTypeStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	err := uc.DeleteAccountTypeByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}

// TestDeleteAccountTypeByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, DeleteAccountTypeByID
// must still complete successfully because durability is owned by PG +
// future DLQ/outbox, not by the synchronous Emit call.
func TestDeleteAccountTypeByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteAccountTypeStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	err := uc.DeleteAccountTypeByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

// TestDeleteAccountTypeByID_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestDeleteAccountTypeByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteAccountTypeStreamingTestUseCase(t, ctrl, nil)

	err := uc.DeleteAccountTypeByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}
