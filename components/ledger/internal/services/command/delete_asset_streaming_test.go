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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeleteAssetStreamingTestUseCase wires a happy-path UseCase suitable
// for exercising the asset.deleted emission.
//
// AssetRepo.Find echoes the requested identity so test bodies can assert
// the emitted Subject and payload.id without prior coordination.
// AccountRepo.ListExternalAccountsByAssetCode returns an empty slice so
// the external-account cascade is a no-op (the asset.deleted event
// must still emit; that cascade is internal plumbing).
func newDeleteAssetStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, assetID uuid.UUID) *UseCase {
	t.Helper()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)

	mockAssetRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID, id uuid.UUID) (*mmodel.Asset, error) {
			return &mmodel.Asset{
				ID:             id.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Name:           "US Dollar",
				Type:           "currency",
				Code:           "USD",
				Status:         mmodel.Status{Code: "ACTIVE"},
			}, nil
		}).AnyTimes()

	// No external accounts exist → cascade-delete branch is a no-op.
	mockAccountRepo.EXPECT().
		ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*mmodel.Account{}, nil).AnyTimes()

	mockAssetRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		AssetRepo:   mockAssetRepo,
		AccountRepo: mockAccountRepo,
		Streaming:   emitter,
	}
}

// TestDeleteAssetByID_EmitsAssetDeletedEvent verifies that a successful
// DeleteAssetByID call publishes exactly one asset.deleted event with
// the expected resource/event types, tenant ID, subject and payload
// fields.
func TestDeleteAssetByID_EmitsAssetDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	assetID := uuid.New()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteAssetStreamingTestUseCase(t, ctrl, mockEmitter, assetID)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	before := time.Now()
	err := uc.DeleteAssetByID(ctx, orgID, ledgerID, assetID)
	after := time.Now()
	require.NoError(t, err)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "asset", "deleted")

	evt := events[0]
	assert.Equal(t, "asset.deleted", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, assetID.String(), evt.Subject, "Subject must be the deleted asset ID")

	// Timestamp must be the wall-clock instant captured at the emit
	// site — bounded by the test's before/after window.
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, assetID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.NotEmpty(t, payload["deletedAt"], "deletedAt must be set (RFC3339)")
}

// TestDeleteAssetByID_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, DeleteAssetByID
// succeeds without error and no panic.
func TestDeleteAssetByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteAssetStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), uuid.New())

	err := uc.DeleteAssetByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}

// TestDeleteAssetByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, DeleteAssetByID must
// still complete successfully because durability is owned by PG +
// future DLQ/outbox, not by the synchronous Emit call.
func TestDeleteAssetByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteAssetStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, uuid.New())

	err := uc.DeleteAssetByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

// TestDeleteAssetByID_NilStreamingDoesNotPanic confirms that a UseCase
// with a nil Streaming field (legacy / partial wiring) still completes
// the request — the emit block must be guarded.
func TestDeleteAssetByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteAssetStreamingTestUseCase(t, ctrl, nil, uuid.New())

	err := uc.DeleteAssetByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}
