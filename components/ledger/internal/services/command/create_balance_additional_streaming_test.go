// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newCreateAdditionalBalanceStreamingTestUseCase wires a happy-path
// UseCase for exercising the balance.created emission via
// CreateAdditionalBalance.
//
// Repo expectations:
//
//   - FindByAccountIDAndKey is called twice: first for the requested
//     key (returns 404 — the key does not yet exist), then for the
//     default key (returns a usable parent default balance whose
//     fields the additional inherits).
//   - Create echoes the constructed balance with its input ID and
//     timestamps so the test can assert wire fields without coordination.
func newCreateAdditionalBalanceStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, requestedKey string) *UseCase {
	t.Helper()

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	// First lookup (for the requested key): not-found, so the use case
	// proceeds to materialize a new balance.
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), strings.ToLower(requestedKey)).
		Return(nil, pkg.EntityNotFoundError{EntityType: constant.EntityBalance}).
		Times(1)

	// Second lookup (for the default key): supplies the parent default
	// balance whose alias/asset/accountType the additional balance
	// inherits. The Direction is not used by the additional balance
	// (the caller may override) but is set to credit for realism.
	defaultBalance := &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          "@inherited",
		Key:            constant.DefaultBalanceKey,
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		AccountID:      uuid.New().String(),
		AssetCode:      "USD",
		AccountType:    "deposit",
		Available:      decimal.NewFromInt(0),
		OnHold:         decimal.NewFromInt(0),
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
	}

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), constant.DefaultBalanceKey).
		Return(defaultBalance, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Balance) (*mmodel.Balance, error) {
			out := *in
			return &out, nil
		}).
		AnyTimes()

	return &UseCase{
		BalanceRepo: mockBalanceRepo,
		Streaming:   emitter,
	}
}

// TestCreateAdditionalBalance_EmitsBalanceCreatedEvent verifies that a
// successful CreateAdditionalBalance call publishes exactly one
// balance.created event with the expected resource/event types, tenant
// ID, subject and payload fields.
func TestCreateAdditionalBalance_EmitsBalanceCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreateAdditionalBalanceStreamingTestUseCase(t, ctrl, mockEmitter, "asset-freeze")

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "asset-freeze",
	}

	created, err := uc.CreateAdditionalBalance(context.Background(), uuid.New(), uuid.New(), uuid.New(), cbi)
	require.NoError(t, err)
	require.NotNil(t, created)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "balance", "created")

	evt := emitted[0]
	assert.Equal(t, "balance.created", evt.DefinitionKey)
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, created.ID, evt.Subject)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, created.ID, payload["id"])
	assert.Equal(t, created.OrganizationID, payload["organizationId"])
	assert.Equal(t, created.LedgerID, payload["ledgerId"])
	assert.Equal(t, created.AccountID, payload["accountId"])
	assert.Equal(t, "asset-freeze", payload["key"])
	assert.Equal(t, "USD", payload["assetCode"])
	assert.Equal(t, true, payload["allowSending"])
	assert.Equal(t, true, payload["allowReceiving"])
	assert.Equal(t, "credit", payload["direction"])
}

// TestCreateAdditionalBalance_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path: when Streaming is the NoopEmitter,
// CreateAdditionalBalance succeeds without error.
func TestCreateAdditionalBalance_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAdditionalBalanceStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), "savings")

	cbi := &mmodel.CreateAdditionalBalance{Key: "savings"}
	_, err := uc.CreateAdditionalBalance(context.Background(), uuid.New(), uuid.New(), uuid.New(), cbi)
	require.NoError(t, err)
}

// TestCreateAdditionalBalance_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, CreateAdditionalBalance
// must still return the persisted balance.
func TestCreateAdditionalBalance_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAdditionalBalanceStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, "savings")

	cbi := &mmodel.CreateAdditionalBalance{Key: "savings"}
	created, err := uc.CreateAdditionalBalance(context.Background(), uuid.New(), uuid.New(), uuid.New(), cbi)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, created)
}

// TestCreateAdditionalBalance_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field still completes the request.
func TestCreateAdditionalBalance_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAdditionalBalanceStreamingTestUseCase(t, ctrl, nil, "savings")

	cbi := &mmodel.CreateAdditionalBalance{Key: "savings"}
	_, err := uc.CreateAdditionalBalance(context.Background(), uuid.New(), uuid.New(), uuid.New(), cbi)
	require.NoError(t, err)
}
