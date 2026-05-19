// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newCreateAccountTypeStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the account-type.created emission.
// AccountTypeRepo.Create echoes the input with a server-assigned ID so
// the test body can assert the emitted Subject and payload.id without
// prior coordination. The tests do not exercise the metadata branch —
// CreateAccountTypeInput.Metadata is nil, so CreateOnboardingMetadata
// short-circuits before calling the metadata repo.
func newCreateAccountTypeStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	mockAccountTypeRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, in *mmodel.AccountType) (*mmodel.AccountType, error) {
			out := *in
			out.ID = uuid.New()
			return &out, nil
		}).AnyTimes()

	return &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		Streaming:       emitter,
	}
}

// TestCreateAccountType_EmitsAccountTypeCreatedEvent verifies that a
// successful CreateAccountType call publishes exactly one
// account-type.created event with the expected resource/event types,
// tenant ID, subject and payload fields.
func TestCreateAccountType_EmitsAccountTypeCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreateAccountTypeStreamingTestUseCase(t, ctrl, mockEmitter)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateAccountTypeInput{
		Name:        "Current Assets",
		Description: "Assets convertible to cash within one year",
		KeyValue:    "current_assets",
	}

	a, err := uc.CreateAccountType(ctx, orgID, ledgerID, input)
	require.NoError(t, err)
	require.NotNil(t, a)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account-type", "created")

	evt := events[0]
	assert.Equal(t, "account-type.created", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, a.ID.String(), evt.Subject, "Subject must be the new account type ID")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, a.ID.String(), payload["id"])
	assert.Equal(t, a.OrganizationID.String(), payload["organizationId"])
	assert.Equal(t, a.LedgerID.String(), payload["ledgerId"])
	assert.Equal(t, "Current Assets", payload["name"])
	assert.Equal(t, "Assets convertible to cash within one year", payload["description"])
	assert.Equal(t, "current_assets", payload["keyValue"])
	assert.NotEmpty(t, payload["createdAt"], "createdAt must be set (RFC3339)")
	assert.NotEmpty(t, payload["updatedAt"], "updatedAt must be set (RFC3339)")
}

// TestCreateAccountType_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, CreateAccountType
// succeeds without error and no panic.
func TestCreateAccountType_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAccountTypeStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	input := &mmodel.CreateAccountTypeInput{
		Name:     "Noop Account Type",
		KeyValue: "noop_at",
	}

	a, err := uc.CreateAccountType(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestCreateAccountType_EmitFailureDoesNotFailRequest verifies the IMPORTANT
// posture: when Emit returns an error, CreateAccountType must still return
// the successfully-persisted account type because durability is owned by
// PG + future DLQ/outbox, not by the synchronous Emit call.
func TestCreateAccountType_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAccountTypeStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	input := &mmodel.CreateAccountTypeInput{
		Name:     "Emit Fail Account Type",
		KeyValue: "emit_fail_at",
	}

	a, err := uc.CreateAccountType(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, a)
}

// TestCreateAccountType_NilStreamingDoesNotPanic confirms that a UseCase
// with a nil Streaming field (legacy / partial wiring) still completes
// the request — the emit block must be guarded.
func TestCreateAccountType_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAccountTypeStreamingTestUseCase(t, ctrl, nil)

	input := &mmodel.CreateAccountTypeInput{
		Name:     "Nil Streaming Account Type",
		KeyValue: "nil_streaming_at",
	}

	a, err := uc.CreateAccountType(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, a)
}
