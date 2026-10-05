// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

var (
	cascadeOrgID         = "01920000-0000-7000-8000-000000000001"
	cascadeLedgerID      = uuid.MustParse("01920000-0000-7000-8000-000000000002")
	cascadeAccountID     = uuid.MustParse("01920000-0000-7000-8000-000000000003")
	cascadeHolderID      = uuid.MustParse("01920000-0000-7000-8000-000000000004")
	cascadeInstrumentID  = uuid.MustParse("01920000-0000-7000-8000-000000000005")
	cascadeInstrumentID2 = uuid.MustParse("01920000-0000-7000-8000-000000000006")
)

func TestDeleteInstrumentsByAccount(t *testing.T) {
	notFoundErr := pkg.ValidateBusinessError(constant.ErrInstrumentNotFound, constant.EntityInstrument)
	findErr := errors.New("find failed")
	deleteErr := errors.New("delete failed")

	testCases := []struct {
		name            string
		mockSetup       func(repo *instrument.MockRepository)
		expectedCount   int
		expectedErr     error
		expectedEmitted []uuid.UUID
	}{
		{
			name: "zero refs deletes nothing",
			mockSetup: func(repo *instrument.MockRepository) {
				repo.EXPECT().
					FindLiveRefsByAccount(gomock.Any(), cascadeOrgID, cascadeLedgerID, cascadeAccountID).
					Return([]instrument.InstrumentRef{}, nil)
			},
			expectedCount: 0,
		},
		{
			name: "one ref is soft deleted",
			mockSetup: func(repo *instrument.MockRepository) {
				repo.EXPECT().
					FindLiveRefsByAccount(gomock.Any(), cascadeOrgID, cascadeLedgerID, cascadeAccountID).
					Return([]instrument.InstrumentRef{{ID: cascadeInstrumentID, HolderID: cascadeHolderID}}, nil)
				repo.EXPECT().
					Delete(gomock.Any(), cascadeOrgID, cascadeHolderID, cascadeInstrumentID, false).
					Return(nil).
					Times(1)
			},
			expectedCount:   1,
			expectedEmitted: []uuid.UUID{cascadeInstrumentID},
		},
		{
			name: "not found on a ref counts as already deleted",
			mockSetup: func(repo *instrument.MockRepository) {
				repo.EXPECT().
					FindLiveRefsByAccount(gomock.Any(), cascadeOrgID, cascadeLedgerID, cascadeAccountID).
					Return([]instrument.InstrumentRef{{ID: cascadeInstrumentID, HolderID: cascadeHolderID}}, nil)
				repo.EXPECT().
					Delete(gomock.Any(), cascadeOrgID, cascadeHolderID, cascadeInstrumentID, false).
					Return(notFoundErr)
			},
			expectedCount: 0,
		},
		{
			name: "not found on one ref does not stop the next",
			mockSetup: func(repo *instrument.MockRepository) {
				repo.EXPECT().
					FindLiveRefsByAccount(gomock.Any(), cascadeOrgID, cascadeLedgerID, cascadeAccountID).
					Return([]instrument.InstrumentRef{
						{ID: cascadeInstrumentID, HolderID: cascadeHolderID},
						{ID: cascadeInstrumentID2, HolderID: cascadeHolderID},
					}, nil)
				gomock.InOrder(
					repo.EXPECT().
						Delete(gomock.Any(), cascadeOrgID, cascadeHolderID, cascadeInstrumentID, false).
						Return(notFoundErr),
					repo.EXPECT().
						Delete(gomock.Any(), cascadeOrgID, cascadeHolderID, cascadeInstrumentID2, false).
						Return(nil),
				)
			},
			expectedCount:   1,
			expectedEmitted: []uuid.UUID{cascadeInstrumentID2},
		},
		{
			name: "find error propagates",
			mockSetup: func(repo *instrument.MockRepository) {
				repo.EXPECT().
					FindLiveRefsByAccount(gomock.Any(), cascadeOrgID, cascadeLedgerID, cascadeAccountID).
					Return(nil, findErr)
			},
			expectedCount: 0,
			expectedErr:   findErr,
		},
		{
			name: "technical delete error aborts and propagates",
			mockSetup: func(repo *instrument.MockRepository) {
				repo.EXPECT().
					FindLiveRefsByAccount(gomock.Any(), cascadeOrgID, cascadeLedgerID, cascadeAccountID).
					Return([]instrument.InstrumentRef{
						{ID: cascadeInstrumentID, HolderID: cascadeHolderID},
						{ID: cascadeInstrumentID2, HolderID: cascadeHolderID},
					}, nil)
				repo.EXPECT().
					Delete(gomock.Any(), cascadeOrgID, cascadeHolderID, cascadeInstrumentID, false).
					Return(deleteErr)
			},
			expectedCount: 0,
			expectedErr:   deleteErr,
		},
		{
			name: "technical error after a success returns the partial count",
			mockSetup: func(repo *instrument.MockRepository) {
				repo.EXPECT().
					FindLiveRefsByAccount(gomock.Any(), cascadeOrgID, cascadeLedgerID, cascadeAccountID).
					Return([]instrument.InstrumentRef{
						{ID: cascadeInstrumentID, HolderID: cascadeHolderID},
						{ID: cascadeInstrumentID2, HolderID: cascadeHolderID},
					}, nil)
				gomock.InOrder(
					repo.EXPECT().
						Delete(gomock.Any(), cascadeOrgID, cascadeHolderID, cascadeInstrumentID, false).
						Return(nil),
					repo.EXPECT().
						Delete(gomock.Any(), cascadeOrgID, cascadeHolderID, cascadeInstrumentID2, false).
						Return(deleteErr),
				)
			},
			expectedCount:   1,
			expectedErr:     deleteErr,
			expectedEmitted: []uuid.UUID{cascadeInstrumentID},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tc.mockSetup(mockInstrumentRepo)

			emitter := pkgStreaming.NewMockEmitter()

			uc := &UseCase{
				InstrumentRepo: mockInstrumentRepo,
				Streaming:      emitter,
			}

			count, err := uc.DeleteInstrumentsByAccount(context.Background(), cascadeOrgID, cascadeLedgerID, cascadeAccountID)

			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.expectedCount, count)

			emitted := emitter.Events()
			require.Len(t, emitted, len(tc.expectedEmitted))

			for i, id := range tc.expectedEmitted {
				assert.Equal(t, events.InstrumentDeletedDefinition.Key(), emitted[i].DefinitionKey)
				assert.Equal(t, id.String(), emitted[i].Subject)

				var payload struct {
					ID           string `json:"id"`
					HolderID     string `json:"holderId"`
					DeletionType string `json:"deletionType"`
				}
				require.NoError(t, json.Unmarshal(emitted[i].Payload, &payload))
				assert.Equal(t, "soft", payload.DeletionType)
				assert.Equal(t, id.String(), payload.ID)
				assert.Equal(t, cascadeHolderID.String(), payload.HolderID)
			}
		})
	}
}
