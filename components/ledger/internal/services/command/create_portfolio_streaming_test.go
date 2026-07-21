// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newCreatePortfolioStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the portfolio.created emission. PortfolioRepo.Create
// echoes the input with a server-assigned ID so the test body can assert
// the emitted Subject and payload.id without prior coordination. The
// tests do not exercise the metadata branch — CreatePortfolioInput.Metadata
// is nil, so CreateOnboardingMetadata short-circuits before calling the
// metadata repo.
func newCreatePortfolioStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)

	mockPortfolioRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Portfolio) (*mmodel.Portfolio, error) {
			out := *in
			out.ID = uuid.New().String()
			return &out, nil
		}).AnyTimes()

	return &UseCase{
		PortfolioRepo: mockPortfolioRepo,
		Streaming:     emitter,
	}
}

// TestCreatePortfolio_EmitsPortfolioCreatedEvent verifies that a
// successful CreatePortfolio call publishes exactly one portfolio.created
// event with the expected resource/event types, tenant ID, subject and
// payload fields.
func TestCreatePortfolio_EmitsPortfolioCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreatePortfolioStreamingTestUseCase(t, ctrl, mockEmitter)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreatePortfolioInput{
		Name:     "Investment Portfolio",
		EntityID: "ext-entity-1",
	}

	p, err := uc.CreatePortfolio(ctx, orgID, ledgerID, input)
	require.NoError(t, err)
	require.NotNil(t, p)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "portfolio", "created")

	evt := events[0]
	assert.Equal(t, "portfolio.created", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, p.ID, evt.Subject, "Subject must be the new portfolio ID")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, p.ID, payload["id"])
	assert.Equal(t, p.OrganizationID, payload["organizationId"])
	assert.Equal(t, p.LedgerID, payload["ledgerId"])
	assert.Equal(t, "Investment Portfolio", payload["name"])
	assert.Equal(t, "ext-entity-1", payload["entityId"])
	assert.NotEmpty(t, payload["createdAt"], "createdAt must be set (RFC3339)")
	assert.NotEmpty(t, payload["updatedAt"], "updatedAt must be set (RFC3339)")
	assert.Contains(t, payload, "status")
}

// TestCreatePortfolio_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, CreatePortfolio
// succeeds without error and no panic.
func TestCreatePortfolio_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreatePortfolioStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	input := &mmodel.CreatePortfolioInput{Name: "Noop Portfolio"}

	p, err := uc.CreatePortfolio(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, p)
}

// TestCreatePortfolio_EmitFailureDoesNotFailRequest verifies the IMPORTANT
// posture: when Emit returns an error, CreatePortfolio must still return
// the successfully-persisted portfolio because durability is owned by
// PG + future DLQ/outbox, not by the synchronous Emit call.
func TestCreatePortfolio_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreatePortfolioStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	input := &mmodel.CreatePortfolioInput{Name: "Emit Fail Portfolio"}

	p, err := uc.CreatePortfolio(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, p)
}

// TestCreatePortfolio_NilStreamingDoesNotPanic confirms that a UseCase
// with a nil Streaming field (legacy / partial wiring) still completes
// the request — the emit block must be guarded.
func TestCreatePortfolio_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreatePortfolioStreamingTestUseCase(t, ctrl, nil)

	input := &mmodel.CreatePortfolioInput{Name: "Nil Streaming Portfolio"}

	p, err := uc.CreatePortfolio(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, p)
}
