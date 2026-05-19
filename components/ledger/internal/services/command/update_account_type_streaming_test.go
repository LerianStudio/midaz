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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newUpdateAccountTypeStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the account-type.updated emission.
//
// AccountTypeRepo.Update is mocked to return a post-commit record
// carrying the request identity (ID/OrganizationID/LedgerID), the
// canonical KeyValue preserved from create time, and the persisted
// UpdatedAt. This mirrors the contract of the refactored squirrel +
// RETURNING repo method — the use case trusts the repo's return value
// directly without merging against a pre-update fetch.
func newUpdateAccountTypeStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time, canonicalName, canonicalKeyValue string) *UseCase {
	t.Helper()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockAccountTypeRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, id uuid.UUID, in *mmodel.AccountType) (*mmodel.AccountType, error) {
			out := &mmodel.AccountType{
				ID:             id,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Name:           canonicalName,
				Description:    in.Description,
				KeyValue:       canonicalKeyValue,
				UpdatedAt:      fixedUpdatedAt,
			}
			// Mirror partial-update fidelity: if caller supplied a non-empty
			// Name, that beats the canonical name. If empty, canonical wins
			// (preserving create-time state).
			if in.Name != "" {
				out.Name = in.Name
			}
			return out, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mongodb.Metadata{Data: map[string]any{}}, nil).AnyTimes()

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		Streaming:              emitter,
	}
}

// TestUpdateAccountType_EmitsAccountTypeUpdatedEvent verifies that a
// successful UpdateAccountType call publishes exactly one
// account-type.updated event with the expected resource/event types,
// tenant ID, subject and payload fields. Subject and identity fields
// must source from the post-commit repo return value (NOT a re-fetch).
func TestUpdateAccountType_EmitsAccountTypeUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newUpdateAccountTypeStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt, "Canonical Name", "canonical_key")

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountTypeID := uuid.New()

	input := &mmodel.UpdateAccountTypeInput{
		Name:        "Updated Account Type Name",
		Description: "Updated description",
	}

	a, err := uc.UpdateAccountType(ctx, orgID, ledgerID, accountTypeID, input)
	require.NoError(t, err)
	require.NotNil(t, a)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account-type", "updated")

	evt := events[0]
	assert.Equal(t, "account-type.updated", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, accountTypeID.String(), evt.Subject, "Subject must be the request-path account type ID (from repo RETURNING)")
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp, "Timestamp must pin to persisted UpdatedAt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, accountTypeID.String(), payload["id"], "payload.id must source from repo RETURNING")
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.Equal(t, "Updated Account Type Name", payload["name"])
	assert.Equal(t, "Updated description", payload["description"])
	assert.Equal(t, "canonical_key", payload["keyValue"], "keyValue is immutable post-create; canonical preserved")
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
}

// TestUpdateAccountType_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter,
// UpdateAccountType succeeds without error and no panic.
func TestUpdateAccountType_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateAccountTypeStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt, "Canonical Name", "canonical_key")

	input := &mmodel.UpdateAccountTypeInput{
		Name: "Noop Updated Account Type",
	}

	a, err := uc.UpdateAccountType(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestUpdateAccountType_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, UpdateAccountType must
// still return the successfully-persisted account type because
// durability is owned by PG + future DLQ/outbox, not by the synchronous
// Emit call.
func TestUpdateAccountType_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateAccountTypeStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt, "Canonical Name", "canonical_key")

	input := &mmodel.UpdateAccountTypeInput{
		Name: "Emit Fail Updated Account Type",
	}

	a, err := uc.UpdateAccountType(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, a)
}

// TestUpdateAccountType_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestUpdateAccountType_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateAccountTypeStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt, "Canonical Name", "canonical_key")

	input := &mmodel.UpdateAccountTypeInput{
		Name: "Nil Streaming Updated Account Type",
	}

	a, err := uc.UpdateAccountType(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, a)
}
