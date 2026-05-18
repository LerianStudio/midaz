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
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newUpdateStreamingTestUseCase wires a happy-path UseCase suitable for
// exercising the account.updated emission. All repositories are
// gomock-backed and preconfigured to accept any call:
//   - AccountRepo.Find returns a pre-update record carrying the request
//     identity (ID/OrganizationID/LedgerID). The use case merges this
//     with the PATCH input in-memory for the emission payload, so the
//     mock does not need to model the post-update state.
//   - AccountRepo.Update stamps UpdatedAt to a fixed timestamp so the
//     emit-site assertion can lock on the value.
//   - OnboardingMetadataRepo.FindByEntity + Update both succeed.
func newUpdateStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time) *UseCase {
	t.Helper()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, _ *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
			return &mmodel.Account{
				ID:             id.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Type:           "deposit",
			}, nil
		}).AnyTimes()

	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			out.UpdatedAt = fixedUpdatedAt
			return &out, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mongodb.Metadata{Data: map[string]any{}}, nil).AnyTimes()

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		Streaming:              emitter,
	}
}

// TestUpdateAccount_EmitsAccountUpdatedEvent verifies that a successful
// UpdateAccount call publishes exactly one account.updated event with the
// expected resource/event types, tenant ID, subject and payload fields.
func TestUpdateAccount_EmitsAccountUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newUpdateStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	input := &mmodel.UpdateAccountInput{
		Name:   "Updated Streaming Account",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	acc, err := uc.UpdateAccount(ctx, orgID, ledgerID, nil, accountID, input)
	require.NoError(t, err)
	require.NotNil(t, acc)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account", "updated")

	evt := events[0]
	assert.Equal(t, "account.updated", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	// Subject and identity fields come from the post-update re-fetch
	// (AccountRepo.Find), NOT from AccountRepo.Update's buggy input-derived
	// return value. See big comment in update_account.go for context.
	assert.Equal(t, accountID.String(), evt.Subject, "Subject must be the request-path account ID")
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp, "Timestamp must pin to persisted UpdatedAt")

	// Payload is json.RawMessage — decode and inspect required fields.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, accountID.String(), payload["id"], "payload.id must source from post-update Find")
	assert.Equal(t, orgID.String(), payload["organizationId"], "payload.organizationId must source from post-update Find")
	assert.Equal(t, ledgerID.String(), payload["ledgerId"], "payload.ledgerId must source from post-update Find")
	assert.Equal(t, "Updated Streaming Account", payload["name"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
	assert.Contains(t, payload, "status")
	assert.Contains(t, payload, "blocked")
}

// TestUpdateAccount_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, UpdateAccount
// succeeds without error and no panic.
func TestUpdateAccount_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt)

	input := &mmodel.UpdateAccountInput{
		Name:   "Noop Updated Account",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	acc, err := uc.UpdateAccount(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, acc)
}

// TestUpdateAccount_EmitFailureDoesNotFailRequest verifies the IMPORTANT
// posture: when Emit returns an error, UpdateAccount must still return
// the successfully-persisted account because durability is owned by PG
// + future DLQ/outbox, not by the synchronous Emit call.
func TestUpdateAccount_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt)

	input := &mmodel.UpdateAccountInput{
		Name:   "Emit Fail Updated Account",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	acc, err := uc.UpdateAccount(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, acc)
}

// TestUpdateAccount_NilStreamingDoesNotPanic confirms that a UseCase with
// a nil Streaming field (legacy / partial wiring) still completes the
// request — the emit block must be guarded.
func TestUpdateAccount_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt)

	input := &mmodel.UpdateAccountInput{
		Name:   "Nil Streaming Updated Account",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	acc, err := uc.UpdateAccount(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, acc)
}
