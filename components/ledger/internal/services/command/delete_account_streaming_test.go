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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeleteStreamingTestUseCase wires a happy-path UseCase suitable for
// exercising the account.deleted emission. All repositories are
// gomock-backed and preconfigured to accept any call:
//   - AccountRepo.Find returns a non-external account with the requested
//     ID (so the parse-then-delete path proceeds).
//   - BalanceRepo.ListByAccountID returns an empty slice (cascade
//     delete-all-balances path is a no-op).
//   - AccountRepo.Delete succeeds.
//
// The accountID returned mirrors the one Find echoes, so test bodies can
// assert the emitted Subject and payload.id without prior coordination.
func newDeleteStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, accountID uuid.UUID, portfolioIDOnAccount *string) *UseCase {
	t.Helper()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, _ *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
			return &mmodel.Account{
				ID:             id.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				PortfolioID:    portfolioIDOnAccount,
				Type:           "deposit",
			}, nil
		}).AnyTimes()

	mockBalanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*mmodel.Balance{}, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		AccountRepo: mockAccountRepo,
		BalanceRepo: mockBalanceRepo,
		Streaming:   emitter,
	}
}

// TestDeleteAccountByID_EmitsAccountDeletedEvent verifies that a
// successful DeleteAccountByID call publishes exactly one account.deleted
// event with the expected resource/event types, tenant ID, subject and
// payload fields. PortfolioID is sourced from the pre-delete record.
func TestDeleteAccountByID_EmitsAccountDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	accountID := uuid.New()
	portfolioOnAccount := uuid.New().String()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteStreamingTestUseCase(t, ctrl, mockEmitter, accountID, &portfolioOnAccount)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	before := time.Now()
	err := uc.DeleteAccountByID(ctx, orgID, ledgerID, nil, accountID, "Bearer test")
	after := time.Now()
	require.NoError(t, err)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account", "deleted")

	evt := events[0]
	assert.Equal(t, "account.deleted", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, accountID.String(), evt.Subject, "Subject must be the deleted account ID")

	// Timestamp must be the wall-clock instant captured at the emit
	// site — bounded by the test's before/after window.
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	// Payload is json.RawMessage — decode and inspect required fields.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, accountID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.Equal(t, portfolioOnAccount, payload["portfolioId"], "portfolioId must source from the pre-delete record")
	assert.NotEmpty(t, payload["deletedAt"], "deletedAt must be set (RFC3339)")
}

// TestDeleteAccountByID_EmitsWithoutPortfolio verifies that an account
// not scoped to any portfolio emits with portfolioId serialized as JSON
// null (not stripped).
func TestDeleteAccountByID_EmitsWithoutPortfolio(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	accountID := uuid.New()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteStreamingTestUseCase(t, ctrl, mockEmitter, accountID, nil)

	err := uc.DeleteAccountByID(context.Background(), uuid.New(), uuid.New(), nil, accountID, "Bearer test")
	require.NoError(t, err)

	events := mockEmitter.Events()
	require.Len(t, events, 1)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))

	assert.Contains(t, payload, "portfolioId", "field must be present (encoded as null)")
	assert.Nil(t, payload["portfolioId"], "portfolioId must be JSON null when account is not portfolio-scoped")
}

// TestDeleteAccountByID_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, DeleteAccountByID
// succeeds without error and no panic.
func TestDeleteAccountByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), uuid.New(), nil)

	err := uc.DeleteAccountByID(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), "Bearer test")
	require.NoError(t, err)
}

// TestDeleteAccountByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, DeleteAccountByID must
// still complete successfully because durability is owned by PG +
// future DLQ/outbox, not by the synchronous Emit call.
func TestDeleteAccountByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, uuid.New(), nil)

	err := uc.DeleteAccountByID(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), "Bearer test")
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

// TestDeleteAccountByID_NilStreamingDoesNotPanic confirms that a UseCase
// with a nil Streaming field (legacy / partial wiring) still completes
// the request — the emit block must be guarded.
func TestDeleteAccountByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteStreamingTestUseCase(t, ctrl, nil, uuid.New(), nil)

	err := uc.DeleteAccountByID(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), "Bearer test")
	require.NoError(t, err)
}
