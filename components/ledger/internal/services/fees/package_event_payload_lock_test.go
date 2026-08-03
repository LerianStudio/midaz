// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The ledger on the fee-package wire events is the one the STORED DOCUMENT
// carries, never the ledger the caller named. Under organization scope the
// caller names uuid.Nil, so an event that took its ledger from the argument
// would ship an all-zero ledgerId to every consumer without failing anything
// else. These tests pin the emitted bytes so that substitution cannot pass.

func TestDeletePackageByID_OrgScope_EventCarriesStoredLedger(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	packID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	storedLedger := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	mockPackRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(uuid.Nil)).
		Return(&pack.Package{ID: packID, LedgerID: storedLedger}, nil)
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(uuid.Nil)).
		Return(nil)

	svc := &UseCase{packageRepo: mockPackRepo, Streaming: mockEmitter}

	require.NoError(t, svc.DeletePackageByID(context.Background(), packID, orgID, uuid.Nil))

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	payload := unmarshalPayload(t, emitted[0].Payload)

	assert.Equal(t, packID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, storedLedger.String(), payload["ledgerId"],
		"the deleted event must carry the ledger stored on the document, not the caller's scope argument")
	assert.NotEqual(t, uuid.Nil.String(), payload["ledgerId"],
		"organization scope must never leak the nil sentinel onto the wire")

	assert.ElementsMatch(t,
		[]string{"id", "organizationId", "ledgerId", "deletedAt"},
		payloadKeys(payload),
		"the deleted payload key set must not drift")
}

func TestUpdatePackageByID_OrgScope_EventCarriesStoredLedger(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	packID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	orgID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	storedLedger := uuid.MustParse("66666666-6666-6666-6666-666666666666")

	enable := true
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mockPackRepo.EXPECT().
		FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Eq(orgID), gomock.Eq(packID)).
		Return(&model.AmountData{
			MinAmount: decimal.NewFromInt(100),
			MaxAmount: decimal.NewFromInt(1000),
			Fees:      map[string]model.Fee{},
			LedgerID:  storedLedger,
		}, nil)

	mockPackRepo.EXPECT().
		Update(gomock.Any(), gomock.Eq(packID), gomock.Eq(orgID), gomock.Eq(uuid.Nil), gomock.Any()).
		Return(&pack.Package{
			ID:        packID,
			LedgerID:  storedLedger,
			Enable:    &enable,
			CreatedAt: fixedTime,
			UpdatedAt: fixedTime,
		}, nil)

	svc := &UseCase{packageRepo: mockPackRepo, Streaming: mockEmitter}

	require.NoError(t, svc.UpdatePackageByID(context.Background(), packID, orgID, uuid.Nil,
		&model.UpdatePackageInput{FeeGroupLabel: "Renamed"}))

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	payload := unmarshalPayload(t, emitted[0].Payload)

	assert.Equal(t, packID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, storedLedger.String(), payload["ledgerId"],
		"the updated event must carry the ledger stored on the document, not the caller's scope argument")
	assert.NotEqual(t, uuid.Nil.String(), payload["ledgerId"],
		"organization scope must never leak the nil sentinel onto the wire")

	assert.ElementsMatch(t,
		[]string{"id", "organizationId", "ledgerId", "segmentId", "transactionRoute", "enable", "createdAt", "updatedAt"},
		payloadKeys(payload),
		"the updated payload key set must not drift")
}

func payloadKeys(payload map[string]any) []string {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}

	return keys
}
