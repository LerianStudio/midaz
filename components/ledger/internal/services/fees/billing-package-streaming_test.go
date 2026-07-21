// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"testing"

	billing_package "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// findEmittedByKey returns the first captured request whose DefinitionKey matches
// "<resourceType>.<eventType>", or fails the test.
func findEmittedByKey(t *testing.T, m *pkgStreaming.MockEmitter, resourceType, eventType string) libStreaming.EmitRequest {
	t.Helper()

	key := resourceType + "." + eventType
	for _, e := range m.Events() {
		if e.DefinitionKey == key {
			return e
		}
	}

	t.Fatalf("no emitted event with key %q; got %v", key, m.Events())

	return libStreaming.EmitRequest{}
}

func TestCreateBillingPackage_EmitsCreatedEvent(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockMidaz := newTestBillingPackageService(t)

	mock := pkgStreaming.NewMockEmitter()
	svc.Streaming = mock

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	bp := validVolumeBillingPackage()
	bp.OrganizationID = orgID
	bp.LedgerID = ledgerID

	mockRepo.EXPECT().
		FindMatchingPackages(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	mockMidaz.EXPECT().
		AccountExistsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *model.BillingPackage) (*model.BillingPackage, error) {
			return in, nil
		})

	result, err := svc.CreateBillingPackage(context.Background(), bp)
	require.NoError(t, err)
	require.NotNil(t, result)

	pkgStreaming.AssertEventEmitted(t, mock, "fees-billing-package", "created")

	req := findEmittedByKey(t, mock, "fees-billing-package", "created")
	assert.Equal(t, result.ID, req.Subject)

	var payload struct {
		OrganizationID string `json:"organizationId"`
		LedgerID       string `json:"ledgerId"`
	}
	require.NoError(t, json.Unmarshal(req.Payload, &payload))
	assert.Equal(t, orgID, payload.OrganizationID)
	assert.Equal(t, ledgerID, payload.LedgerID)
}

func TestUpdateBillingPackage_EmitsUpdatedEvent(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	mock := pkgStreaming.NewMockEmitter()
	svc.Streaming = mock

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	bpID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	ledgerID := "33333333-3333-3333-3333-333333333333"

	mockRepo.EXPECT().
		Update(gomock.Any(), bpID.String(), orgID.String(), gomock.Any()).
		Return(&model.BillingPackage{
			ID:             bpID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID,
			Label:          "Updated",
			Enable:         boolPtr(true),
			UpdatedAt:      "2026-01-02T00:00:00Z",
		}, nil)

	result, err := svc.UpdateBillingPackage(context.Background(), bpID, orgID, map[string]any{"label": "Updated"})
	require.NoError(t, err)
	require.NotNil(t, result)

	pkgStreaming.AssertEventEmitted(t, mock, "fees-billing-package", "updated")

	req := findEmittedByKey(t, mock, "fees-billing-package", "updated")
	assert.Equal(t, bpID.String(), req.Subject)

	var payload struct {
		OrganizationID string `json:"organizationId"`
		LedgerID       string `json:"ledgerId"`
	}
	require.NoError(t, json.Unmarshal(req.Payload, &payload))
	assert.Equal(t, orgID.String(), payload.OrganizationID)
	assert.Equal(t, ledgerID, payload.LedgerID)
}

func TestDeleteBillingPackage_EmitsDeletedEvent(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	mock := pkgStreaming.NewMockEmitter()
	svc.Streaming = mock

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	bpID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	ledgerID := "33333333-3333-3333-3333-333333333333"

	mockRepo.EXPECT().
		FindByID(gomock.Any(), bpID.String(), orgID.String()).
		Return(&model.BillingPackage{
			ID:             bpID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID,
		}, nil)

	mockRepo.EXPECT().
		SoftDelete(gomock.Any(), bpID.String(), orgID.String()).
		Return(nil)

	err := svc.DeleteBillingPackage(context.Background(), bpID, orgID)
	require.NoError(t, err)

	pkgStreaming.AssertEventEmitted(t, mock, "fees-billing-package", "deleted")

	req := findEmittedByKey(t, mock, "fees-billing-package", "deleted")
	assert.Equal(t, bpID.String(), req.Subject)

	var payload struct {
		OrganizationID string `json:"organizationId"`
		LedgerID       string `json:"ledgerId"`
	}
	require.NoError(t, json.Unmarshal(req.Payload, &payload))
	assert.Equal(t, orgID.String(), payload.OrganizationID)
	assert.Equal(t, ledgerID, payload.LedgerID)
}

func TestDeleteBillingPackage_FindByIDFails_StillDeletesNoEmit(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	mock := pkgStreaming.NewMockEmitter()
	svc.Streaming = mock

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	bpID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	mockRepo.EXPECT().
		FindByID(gomock.Any(), bpID.String(), orgID.String()).
		Return(nil, assert.AnError)

	mockRepo.EXPECT().
		SoftDelete(gomock.Any(), bpID.String(), orgID.String()).
		Return(nil)

	err := svc.DeleteBillingPackage(context.Background(), bpID, orgID)
	require.NoError(t, err)

	assert.Empty(t, mock.Events())
}

func TestBillingPackage_NilAndNoopEmitter_NoPanic(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	bpID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	setup := func(t *testing.T) (*BillingPackageService, *billing_package.MockRepository) {
		svc, mockRepo, _ := newTestBillingPackageService(t)

		mockRepo.EXPECT().
			Update(gomock.Any(), bpID.String(), orgID.String(), gomock.Any()).
			Return(&model.BillingPackage{
				ID:             bpID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       "33333333-3333-3333-3333-333333333333",
				UpdatedAt:      "2026-01-02T00:00:00Z",
			}, nil)

		mockRepo.EXPECT().
			FindByID(gomock.Any(), bpID.String(), orgID.String()).
			Return(&model.BillingPackage{
				ID:             bpID.String(),
				OrganizationID: orgID.String(),
				LedgerID:       "33333333-3333-3333-3333-333333333333",
			}, nil)

		mockRepo.EXPECT().
			SoftDelete(gomock.Any(), bpID.String(), orgID.String()).
			Return(nil)

		return svc, mockRepo
	}

	t.Run("nil emitter", func(t *testing.T) {
		t.Parallel()

		svc, _ := setup(t)
		// Streaming stays nil (default).

		_, err := svc.UpdateBillingPackage(context.Background(), bpID, orgID, map[string]any{"label": "x"})
		require.NoError(t, err)

		require.NoError(t, svc.DeleteBillingPackage(context.Background(), bpID, orgID))
	})

	t.Run("noop emitter", func(t *testing.T) {
		t.Parallel()

		svc, _ := setup(t)
		svc.Streaming = libStreaming.NewNoopEmitter()

		_, err := svc.UpdateBillingPackage(context.Background(), bpID, orgID, map[string]any{"label": "x"})
		require.NoError(t, err)

		require.NoError(t, svc.DeleteBillingPackage(context.Background(), bpID, orgID))
	})
}
