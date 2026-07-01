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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newCreateOrganizationStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedCreatedAt time.Time) *UseCase {
	t.Helper()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	mockOrganizationRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Organization) (*mmodel.Organization, error) {
			out := *in
			out.ID = uuid.New().String()
			out.CreatedAt = fixedCreatedAt
			out.UpdatedAt = fixedCreatedAt

			return &out, nil
		}).AnyTimes()

	return &UseCase{
		OrganizationRepo: mockOrganizationRepo,
		Streaming:        emitter,
	}
}

func TestCreateOrganization_EmitsOrganizationCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedCreatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreateOrganizationStreamingTestUseCase(t, ctrl, mockEmitter, fixedCreatedAt)

	dba := "Lerian FS"
	org, err := uc.CreateOrganization(context.Background(), &mmodel.CreateOrganizationInput{
		LegalName:       "Lerian Financial Services Ltd.",
		DoingBusinessAs: &dba,
		LegalDocument:   "123456789012345",
		Address:         mmodel.Address{Country: "US"},
	})
	require.NoError(t, err)
	require.NotNil(t, org)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "organization", "created")

	evt := events[0]
	assert.Equal(t, "organization.created", evt.DefinitionKey)
	assert.Equal(t, "default", evt.TenantID)
	assert.Equal(t, org.ID, evt.Subject)
	assert.Equal(t, fixedCreatedAt, evt.Timestamp)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))
	assert.Equal(t, org.ID, payload["id"])
	assert.Equal(t, org.LegalName, payload["legalName"])
	assert.Equal(t, org.LegalDocument, payload["legalDocument"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["createdAt"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
	assert.Contains(t, payload, "address")
	assert.Contains(t, payload, "status")
}

func TestCreateOrganization_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedCreatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newCreateOrganizationStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedCreatedAt)

	org, err := uc.CreateOrganization(context.Background(), &mmodel.CreateOrganizationInput{
		LegalName:     "Noop Org",
		LegalDocument: "123456789012345",
		Address:       mmodel.Address{Country: "US"},
	})
	require.NoError(t, err)
	require.NotNil(t, org)
}

func TestCreateOrganization_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedCreatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newCreateOrganizationStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedCreatedAt)

	org, err := uc.CreateOrganization(context.Background(), &mmodel.CreateOrganizationInput{
		LegalName:     "Emit Fail Org",
		LegalDocument: "123456789012345",
		Address:       mmodel.Address{Country: "US"},
	})
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, org)
}

func TestCreateOrganization_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedCreatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newCreateOrganizationStreamingTestUseCase(t, ctrl, nil, fixedCreatedAt)

	org, err := uc.CreateOrganization(context.Background(), &mmodel.CreateOrganizationInput{
		LegalName:     "Nil Streaming Org",
		LegalDocument: "123456789012345",
		Address:       mmodel.Address{Country: "US"},
	})
	require.NoError(t, err)
	require.NotNil(t, org)
}
