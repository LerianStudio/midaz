// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	billing_package "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The ledger on the billing-package wire events is the one the STORED DOCUMENT
// carries, never the ledger the caller named. Under organization scope the caller
// names uuid.Nil, so an event that took its ledger from the argument would ship an
// empty ledgerId to every consumer without failing anything else. These tests pin
// the emitted bytes so that substitution cannot pass.

func TestDeleteBillingPackage_OrgScope_EventCarriesStoredLedger(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	mockEmitter := pkgStreaming.NewMockEmitter()
	svc.Streaming = mockEmitter

	bpID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	storedLedger := "33333333-3333-3333-3333-333333333333"

	mockRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(billing_package.AnyLedger)).
		Return(&model.BillingPackage{
			ID:             bpID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       storedLedger,
		}, nil)
	mockRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(billing_package.AnyLedger)).
		Return(nil)

	require.NoError(t, svc.DeleteBillingPackage(context.Background(), bpID, orgID, uuid.Nil))

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	payload := unmarshalPayload(t, emitted[0].Payload)

	assert.Equal(t, bpID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, storedLedger, payload["ledgerId"],
		"the deleted event must carry the ledger stored on the document, not the caller's scope argument")
	assert.NotEmpty(t, payload["ledgerId"],
		"organization scope must never leak the empty sentinel onto the wire")

	assert.ElementsMatch(t,
		[]string{"id", "organizationId", "ledgerId", "deletedAt"},
		payloadKeys(payload),
		"the deleted payload key set must not drift")
}

func TestUpdateBillingPackage_OrgScope_EventCarriesStoredLedger(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	mockEmitter := pkgStreaming.NewMockEmitter()
	svc.Streaming = mockEmitter

	bpID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	orgID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	storedLedger := "66666666-6666-6666-6666-666666666666"

	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(billing_package.AnyLedger), gomock.Any()).
		Return(&model.BillingPackage{
			ID:             bpID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       storedLedger,
			Type:           model.BillingPackageTypeVolume,
			Enable:         boolPtr(true),
			CreatedAt:      "2026-01-01T00:00:00Z",
			UpdatedAt:      "2026-01-02T00:00:00Z",
		}, nil)

	_, err := svc.UpdateBillingPackage(context.Background(), bpID, orgID, uuid.Nil,
		map[string]any{"label": "Renamed"})
	require.NoError(t, err)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	payload := unmarshalPayload(t, emitted[0].Payload)

	assert.Equal(t, bpID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, storedLedger, payload["ledgerId"],
		"the updated event must carry the ledger stored on the document, not the caller's scope argument")
	assert.NotEmpty(t, payload["ledgerId"],
		"organization scope must never leak the empty sentinel onto the wire")

	assert.ElementsMatch(t,
		[]string{"id", "organizationId", "ledgerId", "type", "pricingModel", "countMode", "enable", "createdAt", "updatedAt"},
		payloadKeys(payload),
		"the updated payload key set must not drift")
}
