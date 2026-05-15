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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newDeleteOrganizationStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	mockOrganizationRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		OrganizationRepo: mockOrganizationRepo,
		Streaming:        emitter,
		StreamingSource:  "lerian.midaz.ledger.test",
	}
}

func TestDeleteOrganizationByID_EmitsOrganizationDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := libStreaming.NewMockEmitter()
	uc := newDeleteOrganizationStreamingTestUseCase(t, ctrl, mockEmitter)
	orgID := uuid.New()

	before := time.Now()
	err := uc.DeleteOrganizationByID(context.Background(), orgID)
	after := time.Now()
	require.NoError(t, err)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")
	libStreaming.AssertEventEmitted(t, mockEmitter, "organization", "deleted")

	evt := events[0]
	assert.Equal(t, "organization", evt.ResourceType)
	assert.Equal(t, "deleted", evt.EventType)
	assert.Equal(t, "1.0.0", evt.SchemaVersion)
	assert.Equal(t, "default", evt.TenantID)
	assert.Equal(t, "lerian.midaz.ledger.test", evt.Source)
	assert.Equal(t, orgID.String(), evt.Subject)
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, orgID.String(), payload["id"])
	assert.NotEmpty(t, payload["deletedAt"])
}

func TestDeleteOrganizationByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteOrganizationStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	err := uc.DeleteOrganizationByID(context.Background(), uuid.New())
	require.NoError(t, err)
}

func TestDeleteOrganizationByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteOrganizationStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	err := uc.DeleteOrganizationByID(context.Background(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

func TestDeleteOrganizationByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteOrganizationStreamingTestUseCase(t, ctrl, nil)

	err := uc.DeleteOrganizationByID(context.Background(), uuid.New())
	require.NoError(t, err)
}
