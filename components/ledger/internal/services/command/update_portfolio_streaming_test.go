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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newUpdatePortfolioStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the portfolio.updated emission.
//
// PortfolioRepo.Update is mocked to return a post-commit record carrying
// the request identity (ID/OrganizationID/LedgerID) plus the persisted
// UpdatedAt and a canonical name preserved from create time. This
// mirrors the contract of the refactored squirrel + RETURNING repo
// method — the use case trusts the repo's return value directly without
// merging against a pre-update fetch.
func newUpdatePortfolioStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time, canonicalName string) *UseCase {
	t.Helper()

	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockPortfolioRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, id uuid.UUID, in *mmodel.Portfolio) (*mmodel.Portfolio, error) {
			out := &mmodel.Portfolio{
				ID:             id.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Name:           canonicalName,
				EntityID:       in.EntityID,
				Status:         in.Status,
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
		PortfolioRepo:          mockPortfolioRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		Streaming:              emitter,
	}
}

// TestUpdatePortfolioByID_EmitsPortfolioUpdatedEvent verifies that a
// successful UpdatePortfolioByID call publishes exactly one
// portfolio.updated event with the expected resource/event types, tenant
// ID, subject and payload fields. Subject and identity fields must source
// from the post-commit repo return value (NOT a re-fetch).
func TestUpdatePortfolioByID_EmitsPortfolioUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newUpdatePortfolioStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt, "Canonical Name")

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()

	input := &mmodel.UpdatePortfolioInput{
		Name:   "Updated Portfolio Name",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	p, err := uc.UpdatePortfolioByID(ctx, orgID, ledgerID, portfolioID, input)
	require.NoError(t, err)
	require.NotNil(t, p)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "portfolio", "updated")

	evt := events[0]
	assert.Equal(t, "portfolio.updated", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, portfolioID.String(), evt.Subject, "Subject must be the request-path portfolio ID (from repo RETURNING)")
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp, "Timestamp must pin to persisted UpdatedAt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, portfolioID.String(), payload["id"], "payload.id must source from repo RETURNING, not regenerated UUID")
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.Equal(t, "Updated Portfolio Name", payload["name"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
	assert.Contains(t, payload, "status")
}

// TestUpdatePortfolioByID_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, UpdatePortfolioByID
// succeeds without error and no panic.
func TestUpdatePortfolioByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdatePortfolioStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt, "Canonical Name")

	input := &mmodel.UpdatePortfolioInput{
		Name:   "Noop Updated Portfolio",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	p, err := uc.UpdatePortfolioByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, p)
}

// TestUpdatePortfolioByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, UpdatePortfolioByID must
// still return the successfully-persisted portfolio because durability
// is owned by PG + future DLQ/outbox, not by the synchronous Emit call.
func TestUpdatePortfolioByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdatePortfolioStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt, "Canonical Name")

	input := &mmodel.UpdatePortfolioInput{
		Name:   "Emit Fail Updated Portfolio",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	p, err := uc.UpdatePortfolioByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, p)
}

// TestUpdatePortfolioByID_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestUpdatePortfolioByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdatePortfolioStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt, "Canonical Name")

	input := &mmodel.UpdatePortfolioInput{
		Name:   "Nil Streaming Updated Portfolio",
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	p, err := uc.UpdatePortfolioByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, p)
}
