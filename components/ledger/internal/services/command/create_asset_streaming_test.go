// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newCreateAssetStreamingTestUseCase wires a happy-path UseCase suitable
// for exercising the asset.created emission.
//
// To keep the mock surface small, ListAccountsByAlias returns a non-empty
// slice so the implicit external-account-create branch (which would
// require mocking AccountRepo.Create and the full BalancePort chain) is
// short-circuited. The emission anchor sits BEFORE this branch, so the
// test still observes the event.
func newCreateAssetStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)

	mockAssetRepo.EXPECT().
		FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockAssetRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Asset) (*mmodel.Asset, error) {
			out := *in
			out.ID = uuid.New().String()
			return &out, nil
		}).AnyTimes()

	// Short-circuit the implicit-external-account branch: return a
	// non-empty slice so len(account) > 0 and the create chain skips.
	mockAccountRepo.EXPECT().
		ListAccountsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*mmodel.Account{{ID: uuid.New().String(), Type: "external"}}, nil).AnyTimes()

	return &UseCase{
		AssetRepo:   mockAssetRepo,
		AccountRepo: mockAccountRepo,
		Streaming:   emitter,
	}
}

// TestCreateAsset_EmitsAssetCreatedEvent verifies that a successful
// CreateAsset call publishes exactly one asset.created event with the
// expected resource/event types, tenant ID, subject and payload fields.
func TestCreateAsset_EmitsAssetCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreateAssetStreamingTestUseCase(t, ctrl, mockEmitter)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateAssetInput{
		Name: "US Dollar",
		Type: "currency",
		Code: "USD",
	}

	a, err := uc.CreateAsset(ctx, orgID, ledgerID, input, "Bearer test")
	require.NoError(t, err)
	require.NotNil(t, a)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "asset", "created")

	evt := events[0]
	assert.Equal(t, "asset.created", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, a.ID, evt.Subject, "Subject must be the new asset ID")

	// Payload is json.RawMessage — decode and inspect required fields.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, a.ID, payload["id"])
	assert.Equal(t, a.OrganizationID, payload["organizationId"])
	assert.Equal(t, a.LedgerID, payload["ledgerId"])
	assert.Equal(t, "US Dollar", payload["name"])
	assert.Equal(t, "currency", payload["type"])
	assert.Equal(t, "USD", payload["code"])
	assert.NotEmpty(t, payload["createdAt"], "createdAt must be set (RFC3339)")
	assert.NotEmpty(t, payload["updatedAt"], "updatedAt must be set (RFC3339)")
	assert.Contains(t, payload, "status")
}

// TestCreateAsset_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, CreateAsset
// succeeds without error and no panic.
func TestCreateAsset_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAssetStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	input := &mmodel.CreateAssetInput{
		Name: "Brazilian Real",
		Type: "currency",
		Code: "BRL",
	}

	a, err := uc.CreateAsset(context.Background(), uuid.New(), uuid.New(), input, "Bearer test")
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestCreateAsset_EmitFailureDoesNotFailRequest verifies the IMPORTANT
// posture: when Emit returns an error, CreateAsset must still return
// the successfully-persisted asset because durability is owned by PG +
// future DLQ/outbox, not by the synchronous Emit call.
func TestCreateAsset_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAssetStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	input := &mmodel.CreateAssetInput{
		Name: "Euro",
		Type: "currency",
		Code: "EUR",
	}

	a, err := uc.CreateAsset(context.Background(), uuid.New(), uuid.New(), input, "Bearer test")
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, a)
}

// TestCreateAsset_NilStreamingDoesNotPanic confirms that a UseCase with
// a nil Streaming field (legacy / partial wiring) still completes the
// request — the emit block must be guarded.
func TestCreateAsset_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateAssetStreamingTestUseCase(t, ctrl, nil)

	input := &mmodel.CreateAssetInput{
		Name: "Pound Sterling",
		Type: "currency",
		Code: "GBP",
	}

	a, err := uc.CreateAsset(context.Background(), uuid.New(), uuid.New(), input, "Bearer test")
	require.NoError(t, err)
	require.NotNil(t, a)
}
