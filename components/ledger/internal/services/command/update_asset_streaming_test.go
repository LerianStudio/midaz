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
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newUpdateAssetStreamingTestUseCase wires a happy-path UseCase suitable
// for exercising the asset.updated emission.
//
// AssetRepo.Update is mocked to return a post-commit record carrying the
// request identity (ID/OrganizationID/LedgerID) plus the persisted
// UpdatedAt. This mirrors the contract of the refactored squirrel +
// RETURNING repo method — the use case trusts the repo's return value
// directly without merging against a pre-update fetch.
func newUpdateAssetStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time) *UseCase {
	t.Helper()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockAssetRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, id uuid.UUID, in *mmodel.Asset) (*mmodel.Asset, error) {
			return &mmodel.Asset{
				ID:             id.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Name:           in.Name,
				Type:           "currency",
				Code:           "USD",
				Status:         in.Status,
				UpdatedAt:      fixedUpdatedAt,
			}, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mongodb.Metadata{Data: map[string]any{}}, nil).AnyTimes()

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		AssetRepo:              mockAssetRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		Streaming:              emitter,
	}
}

// TestUpdateAssetByID_EmitsAssetUpdatedEvent verifies that a successful
// UpdateAssetByID call publishes exactly one asset.updated event with
// the expected resource/event types, tenant ID, subject and payload
// fields. Subject and identity fields must source from the post-commit
// repo return value (NOT a re-fetch).
func TestUpdateAssetByID_EmitsAssetUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newUpdateAssetStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	assetID := uuid.New()

	input := &mmodel.UpdateAssetInput{
		Name:   "US Dollar Updated",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	a, err := uc.UpdateAssetByID(ctx, orgID, ledgerID, assetID, input)
	require.NoError(t, err)
	require.NotNil(t, a)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "asset", "updated")

	evt := events[0]
	assert.Equal(t, "asset.updated", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, assetID.String(), evt.Subject, "Subject must be the request-path asset ID (from repo RETURNING)")
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp, "Timestamp must pin to persisted UpdatedAt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, assetID.String(), payload["id"], "payload.id must source from repo RETURNING, not regenerated UUID")
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.Equal(t, "US Dollar Updated", payload["name"])
	assert.Equal(t, "currency", payload["type"])
	assert.Equal(t, "USD", payload["code"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
	assert.Contains(t, payload, "status")
}

// TestUpdateAssetByID_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, UpdateAssetByID
// succeeds without error and no panic.
func TestUpdateAssetByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateAssetStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt)

	input := &mmodel.UpdateAssetInput{
		Name:   "Noop Updated Asset",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	a, err := uc.UpdateAssetByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, a)
}

// TestUpdateAssetByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, UpdateAssetByID must
// still return the successfully-persisted asset because durability is
// owned by PG + future DLQ/outbox, not by the synchronous Emit call.
func TestUpdateAssetByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateAssetStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt)

	input := &mmodel.UpdateAssetInput{
		Name:   "Emit Fail Updated Asset",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	a, err := uc.UpdateAssetByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, a)
}

// TestUpdateAssetByID_NilStreamingDoesNotPanic confirms that a UseCase
// with a nil Streaming field (legacy / partial wiring) still completes
// the request — the emit block must be guarded.
func TestUpdateAssetByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateAssetStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt)

	input := &mmodel.UpdateAssetInput{
		Name:   "Nil Streaming Updated Asset",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	a, err := uc.UpdateAssetByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, a)
}
