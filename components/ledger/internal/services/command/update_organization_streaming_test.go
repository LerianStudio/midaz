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
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newUpdateOrganizationStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time) *UseCase {
	t.Helper()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockOrganizationRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id uuid.UUID, in *mmodel.Organization) (*mmodel.Organization, error) {
			out := *in
			out.ID = id.String()
			out.UpdatedAt = fixedUpdatedAt

			return &out, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		OrganizationRepo:       mockOrganizationRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		Streaming:              emitter,
		StreamingSource:        "lerian.midaz.ledger.test",
	}
}

func TestUpdateOrganizationByID_EmitsOrganizationUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := libStreaming.NewMockEmitter()
	uc := newUpdateOrganizationStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt)

	orgID := uuid.New()
	org, err := uc.UpdateOrganizationByID(context.Background(), orgID, &mmodel.UpdateOrganizationInput{
		LegalName: "Updated Organization",
		Address:   mmodel.Address{Country: "US"},
		Status:    mmodel.Status{Code: "ACTIVE"},
	})
	require.NoError(t, err)
	require.NotNil(t, org)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")
	libStreaming.AssertEventEmitted(t, mockEmitter, "organization", "updated")

	evt := events[0]
	assert.Equal(t, "organization", evt.ResourceType)
	assert.Equal(t, "updated", evt.EventType)
	assert.Equal(t, "1.0.0", evt.SchemaVersion)
	assert.Equal(t, "default", evt.TenantID)
	assert.Equal(t, "lerian.midaz.ledger.test", evt.Source)
	assert.Equal(t, orgID.String(), evt.Subject)
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, orgID.String(), payload["id"])
	assert.Equal(t, "Updated Organization", payload["legalName"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
	assert.Contains(t, payload, "address")
	assert.Contains(t, payload, "status")
}

func TestUpdateOrganizationByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateOrganizationStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt)

	org, err := uc.UpdateOrganizationByID(context.Background(), uuid.New(), &mmodel.UpdateOrganizationInput{
		LegalName: "Noop Updated Organization",
		Address:   mmodel.Address{Country: "US"},
		Status:    mmodel.Status{Code: "ACTIVE"},
	})
	require.NoError(t, err)
	require.NotNil(t, org)
}

func TestUpdateOrganizationByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateOrganizationStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt)

	org, err := uc.UpdateOrganizationByID(context.Background(), uuid.New(), &mmodel.UpdateOrganizationInput{
		LegalName: "Emit Fail Updated Organization",
		Address:   mmodel.Address{Country: "US"},
		Status:    mmodel.Status{Code: "ACTIVE"},
	})
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, org)
}

func TestUpdateOrganizationByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateOrganizationStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt)

	org, err := uc.UpdateOrganizationByID(context.Background(), uuid.New(), &mmodel.UpdateOrganizationInput{
		LegalName: "Nil Streaming Updated Organization",
		Address:   mmodel.Address{Country: "US"},
		Status:    mmodel.Status{Code: "ACTIVE"},
	})
	require.NoError(t, err)
	require.NotNil(t, org)
}
