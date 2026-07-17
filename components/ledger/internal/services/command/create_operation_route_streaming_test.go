// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newCreateOperationRouteStreamingTestUseCase wires a happy-path
// UseCase suitable for exercising the operation-route.created emission.
// OperationRouteRepo.Create echoes the input with a server-assigned ID
// so the test body can assert the emitted Subject and payload.id
// without prior coordination. The tests do not exercise the metadata
// branch — CreateOperationRouteInput.Metadata is nil.
func newCreateOperationRouteStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, in *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			out := *in
			out.ID = uuid.New()
			return &out, nil
		}).AnyTimes()

	return &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
		Streaming:          emitter,
	}
}

// TestCreateOperationRoute_EmitsOperationRouteCreatedEvent verifies
// that a successful CreateOperationRoute call publishes exactly one
// operation-route.created event with the expected resource/event
// types, tenant ID, subject and payload fields.
func TestCreateOperationRoute_EmitsOperationRouteCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreateOperationRouteStreamingTestUseCase(t, ctrl, mockEmitter)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateOperationRouteInput{
		Title:         "Cashin from service charge",
		Description:   "Operation route for service charges",
		OperationType: "source",
		Account: &mmodel.AccountRule{
			RuleType: "alias",
			ValidIf:  "@cash_account",
		},
	}

	o, err := uc.CreateOperationRoute(ctx, orgID, ledgerID, input)
	require.NoError(t, err)
	require.NotNil(t, o)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "operation-route", "created")

	evt := emitted[0]
	assert.Equal(t, "operation-route.created", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, o.ID.String(), evt.Subject, "Subject must be the new operation route ID")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, o.ID.String(), payload["id"])
	assert.Equal(t, o.OrganizationID.String(), payload["organizationId"])
	assert.Equal(t, o.LedgerID.String(), payload["ledgerId"])
	assert.Equal(t, "Cashin from service charge", payload["title"])
	assert.Equal(t, "Operation route for service charges", payload["description"])
	assert.Equal(t, "source", payload["operationType"])
	assert.NotEmpty(t, payload["createdAt"], "createdAt must be set (RFC3339)")
	assert.NotEmpty(t, payload["updatedAt"], "updatedAt must be set (RFC3339)")

	account, ok := payload["account"].(map[string]any)
	require.True(t, ok, "account must be a nested object")
	assert.Equal(t, "alias", account["ruleType"])
	assert.Equal(t, "@cash_account", account["validIf"])
}

// TestCreateOperationRoute_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path: when Streaming is the NoopEmitter,
// CreateOperationRoute succeeds without error and no panic.
func TestCreateOperationRoute_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateOperationRouteStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	input := &mmodel.CreateOperationRouteInput{
		Title:         "Noop Operation Route",
		OperationType: "source",
	}

	o, err := uc.CreateOperationRoute(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, o)
}

// TestCreateOperationRoute_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, CreateOperationRoute
// must still return the successfully-persisted operation route because
// durability is owned by PG + future DLQ/outbox, not by the synchronous
// Emit call.
func TestCreateOperationRoute_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateOperationRouteStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	input := &mmodel.CreateOperationRouteInput{
		Title:         "Emit Fail Operation Route",
		OperationType: "source",
	}

	o, err := uc.CreateOperationRoute(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, o)
}

// TestCreateOperationRoute_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestCreateOperationRoute_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateOperationRouteStreamingTestUseCase(t, ctrl, nil)

	input := &mmodel.CreateOperationRouteInput{
		Title:         "Nil Streaming Operation Route",
		OperationType: "source",
	}

	o, err := uc.CreateOperationRoute(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, o)
}
