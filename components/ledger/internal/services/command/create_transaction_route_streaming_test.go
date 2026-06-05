// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newCreateTransactionRouteStreamingTestUseCase wires a happy-path
// UseCase suitable for exercising the transaction-route.created
// emission.
//
// OperationRouteRepo.FindByIDs echoes the input IDs as a slice of
// minimal source + destination routes so the type-validation passes.
// TransactionRouteRepo.Create echoes the input with a server-assigned
// ID so the test body can assert the emitted Subject and payload.id
// without prior coordination. Metadata branch is not exercised
// (CreateTransactionRouteInput.Metadata is nil).
func newCreateTransactionRouteStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, opRouteIDs []uuid.UUID) *UseCase {
	t.Helper()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	// FindByIDs returns one source + one destination route so
	// validateOperationRouteTypes passes.
	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error) {
			routes := make([]*mmodel.OperationRoute, 0, len(ids))
			for i, id := range ids {
				opType := "source"
				if i%2 == 1 {
					opType = "destination"
				}
				routes = append(routes, &mmodel.OperationRoute{
					ID:            id,
					OperationType: opType,
				})
			}
			return routes, nil
		}).AnyTimes()

	mockTransactionRouteRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, in *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
			out := *in
			out.ID = uuid.New()
			return &out, nil
		}).AnyTimes()

	_ = opRouteIDs // captured by FindByIDs DoAndReturn via ids parameter

	return &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		OperationRouteRepo:   mockOperationRouteRepo,
		Streaming:            emitter,
	}
}

// TestCreateTransactionRoute_EmitsTransactionRouteCreatedEvent verifies
// that a successful CreateTransactionRoute call publishes exactly one
// transaction-route.created event with the expected resource/event
// types, tenant ID, subject and payload fields.
func TestCreateTransactionRoute_EmitsTransactionRouteCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	opRouteSourceID := uuid.New()
	opRouteDestinationID := uuid.New()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreateTransactionRouteStreamingTestUseCase(t, ctrl, mockEmitter, []uuid.UUID{opRouteSourceID, opRouteDestinationID})

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateTransactionRouteInput{
		Title:           "Charge Settlement",
		Description:     "Settlement route for service charges",
		OperationRoutes: []uuid.UUID{opRouteSourceID, opRouteDestinationID},
	}

	tr, err := uc.CreateTransactionRoute(ctx, orgID, ledgerID, input)
	require.NoError(t, err)
	require.NotNil(t, tr)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction-route", "created")

	evt := emitted[0]
	assert.Equal(t, "transaction-route.created", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, tr.ID.String(), evt.Subject, "Subject must be the new transaction route ID")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, tr.ID.String(), payload["id"])
	assert.Equal(t, tr.OrganizationID.String(), payload["organizationId"])
	assert.Equal(t, tr.LedgerID.String(), payload["ledgerId"])
	assert.Equal(t, "Charge Settlement", payload["title"])
	assert.Equal(t, "Settlement route for service charges", payload["description"])
	assert.NotEmpty(t, payload["createdAt"], "createdAt must be set (RFC3339)")
	assert.NotEmpty(t, payload["updatedAt"], "updatedAt must be set (RFC3339)")

	ids, ok := payload["operationRouteIds"].([]any)
	require.True(t, ok, "operationRouteIds must be present as an array")
	require.Len(t, ids, 2, "operationRouteIds must include both linked operation routes")
	assert.Equal(t, opRouteSourceID.String(), ids[0])
	assert.Equal(t, opRouteDestinationID.String(), ids[1])
}

// TestCreateTransactionRoute_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path.
func TestCreateTransactionRoute_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateTransactionRouteStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), nil)

	input := &mmodel.CreateTransactionRouteInput{
		Title:           "Noop Transaction Route",
		OperationRoutes: []uuid.UUID{uuid.New(), uuid.New()},
	}

	tr, err := uc.CreateTransactionRoute(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, tr)
}

// TestCreateTransactionRoute_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture.
func TestCreateTransactionRoute_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateTransactionRouteStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, nil)

	input := &mmodel.CreateTransactionRouteInput{
		Title:           "Emit Fail Transaction Route",
		OperationRoutes: []uuid.UUID{uuid.New(), uuid.New()},
	}

	tr, err := uc.CreateTransactionRoute(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, tr)
}

// TestCreateTransactionRoute_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field still completes the request.
func TestCreateTransactionRoute_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateTransactionRouteStreamingTestUseCase(t, ctrl, nil, nil)

	input := &mmodel.CreateTransactionRouteInput{
		Title:           "Nil Streaming Transaction Route",
		OperationRoutes: []uuid.UUID{uuid.New(), uuid.New()},
	}

	tr, err := uc.CreateTransactionRoute(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, tr)
}
