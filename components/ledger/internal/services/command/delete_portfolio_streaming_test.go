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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeletePortfolioStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the portfolio.deleted emission. PortfolioRepo.Delete
// returns nil for the success path.
func newDeletePortfolioStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)

	mockPortfolioRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		PortfolioRepo: mockPortfolioRepo,
		Streaming:     emitter,
	}
}

// TestDeletePortfolioByID_EmitsPortfolioDeletedEvent verifies that a
// successful DeletePortfolioByID call publishes exactly one
// portfolio.deleted event with the expected resource/event types,
// tenant ID, subject and payload fields.
func TestDeletePortfolioByID_EmitsPortfolioDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	portfolioID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeletePortfolioStreamingTestUseCase(t, ctrl, mockEmitter)

	before := time.Now()
	err := uc.DeletePortfolioByID(context.Background(), orgID, ledgerID, portfolioID)
	after := time.Now()
	require.NoError(t, err)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "portfolio", "deleted")

	evt := events[0]
	assert.Equal(t, "portfolio.deleted", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, portfolioID.String(), evt.Subject, "Subject must be the deleted portfolio ID")

	// Timestamp must be the wall-clock instant captured at the emit
	// site — bounded by the test's before/after window.
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, portfolioID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.NotEmpty(t, payload["deletedAt"], "deletedAt must be set (RFC3339)")
}

// TestDeletePortfolioByID_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, DeletePortfolioByID
// succeeds without error and no panic.
func TestDeletePortfolioByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeletePortfolioStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	err := uc.DeletePortfolioByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}

// TestDeletePortfolioByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, DeletePortfolioByID
// must still complete successfully because durability is owned by PG +
// future DLQ/outbox, not by the synchronous Emit call.
func TestDeletePortfolioByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeletePortfolioStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	err := uc.DeletePortfolioByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

// TestDeletePortfolioByID_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestDeletePortfolioByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeletePortfolioStreamingTestUseCase(t, ctrl, nil)

	err := uc.DeletePortfolioByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}
