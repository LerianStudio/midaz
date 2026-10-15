// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// TestCreateLedger_MetadataStagedBeforeInsert locks the create order: the name check runs
// first, then the metadata document is written keyed by the ID the use case generated and
// handed to the repository; a metadata failure never reaches the insert, and an insert
// failure discards the staged document.
func TestCreateLedger_MetadataStagedBeforeInsert(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{"region": "south"}

	newInput := func() *mmodel.CreateLedgerInput {
		return &mmodel.CreateLedgerInput{Name: "Metadata Order Ledger", Metadata: metadata}
	}

	t.Run("success writes metadata before insert with the same ID", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		ledgerRepo := ledger.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{LedgerRepo: ledgerRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}
		organizationID := uuid.New()

		var mongoID, pgID string

		gomock.InOrder(
			ledgerRepo.EXPECT().FindByName(gomock.Any(), organizationID, "Metadata Order Ledger").Return(false, nil),
			metaRepo.EXPECT().
				Create(gomock.Any(), constant.EntityLedger, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, m *mongodb.Metadata) error {
					mongoID = m.EntityID
					assert.Empty(t, emitter.Events(), "no event may be published before the insert")

					return nil
				}),
			ledgerRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, l *mmodel.Ledger) (*mmodel.Ledger, error) {
					pgID = l.ID
					out := *l

					return &out, nil
				}),
		)

		got, err := uc.CreateLedger(context.Background(), organizationID, newInput())
		require.NoError(t, err)
		require.NotNil(t, got)

		_, parseErr := uuid.Parse(mongoID)
		require.NoError(t, parseErr, "the use case must generate a UUID before staging metadata")
		assert.Equal(t, mongoID, pgID)
		assert.Equal(t, pgID, got.ID)
		assert.Equal(t, metadata, got.Metadata)

		emitted := emitter.Events()
		require.Len(t, emitted, 1, "the created event is published once after the insert")
		assert.Equal(t, events.LedgerCreatedDefinition.Key(), emitted[0].DefinitionKey)
	})

	t.Run("metadata failure never reaches the insert", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		ledgerRepo := ledger.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{LedgerRepo: ledgerRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}
		organizationID := uuid.New()
		mongoErr := errors.New("mongo unavailable")

		gomock.InOrder(
			ledgerRepo.EXPECT().FindByName(gomock.Any(), organizationID, gomock.Any()).Return(false, nil),
			metaRepo.EXPECT().Create(gomock.Any(), constant.EntityLedger, gomock.Any()).Return(mongoErr),
		)

		got, err := uc.CreateLedger(context.Background(), organizationID, newInput())
		require.ErrorIs(t, err, mongoErr)
		assert.Nil(t, got)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})

	t.Run("insert failure discards the staged metadata", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		ledgerRepo := ledger.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{LedgerRepo: ledgerRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}
		organizationID := uuid.New()
		pgErr := errors.New("insert failed")

		var mongoID, pgID string

		gomock.InOrder(
			ledgerRepo.EXPECT().FindByName(gomock.Any(), organizationID, gomock.Any()).Return(false, nil),
			metaRepo.EXPECT().
				Create(gomock.Any(), constant.EntityLedger, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, m *mongodb.Metadata) error {
					mongoID = m.EntityID
					assert.Empty(t, emitter.Events(), "no event may be published before the insert")

					return nil
				}),
			ledgerRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, l *mmodel.Ledger) (*mmodel.Ledger, error) {
					pgID = l.ID

					return nil, pgErr
				}),
			metaRepo.EXPECT().
				Purge(gomock.Any(), constant.EntityLedger, gomock.Any()).
				DoAndReturn(func(_ context.Context, _, id string) error {
					assert.Equal(t, mongoID, id, "the discard must target the staged document")

					return nil
				}),
		)

		got, err := uc.CreateLedger(context.Background(), organizationID, newInput())
		require.ErrorIs(t, err, pgErr)
		assert.Nil(t, got)
		assert.Equal(t, mongoID, pgID)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})

	t.Run("name conflict writes nothing", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		ledgerRepo := ledger.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{LedgerRepo: ledgerRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}
		organizationID := uuid.New()
		conflictErr := errors.New("name taken")

		ledgerRepo.EXPECT().FindByName(gomock.Any(), organizationID, gomock.Any()).Return(true, conflictErr)

		_, err := uc.CreateLedger(context.Background(), organizationID, newInput())
		require.ErrorIs(t, err, conflictErr)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})
}
