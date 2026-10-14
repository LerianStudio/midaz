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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// TestCreateOrganization_MetadataStagedBeforeInsert locks the create order: the metadata
// document is written before the PostgreSQL insert, keyed by the ID the use case generated
// and handed to the repository; a metadata failure never reaches PostgreSQL, and an insert
// failure discards the staged document.
func TestCreateOrganization_MetadataStagedBeforeInsert(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{"segment": "retail"}

	newInput := func() *mmodel.CreateOrganizationInput {
		return &mmodel.CreateOrganizationInput{
			LegalName:     "Metadata Order Org",
			LegalDocument: "123456789",
			Metadata:      metadata,
		}
	}

	t.Run("success writes metadata before insert with the same ID", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		orgRepo := organization.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}

		var mongoID, pgID string

		gomock.InOrder(
			metaRepo.EXPECT().
				Create(gomock.Any(), constant.EntityOrganization, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, m *mongodb.Metadata) error {
					mongoID = m.EntityID
					assert.Empty(t, emitter.Events(), "no event may be published before the insert")

					return nil
				}),
			orgRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, o *mmodel.Organization) (*mmodel.Organization, error) {
					pgID = o.ID
					out := *o

					return &out, nil
				}),
		)

		got, err := uc.CreateOrganization(context.Background(), newInput())
		require.NoError(t, err)
		require.NotNil(t, got)

		_, parseErr := uuid.Parse(mongoID)
		require.NoError(t, parseErr, "the use case must generate a UUID before staging metadata")
		assert.Equal(t, mongoID, pgID)
		assert.Equal(t, pgID, got.ID)
		assert.Equal(t, metadata, got.Metadata)

		emitted := emitter.Events()
		require.Len(t, emitted, 1, "the created event is published once after the insert")
		assert.Equal(t, events.OrganizationCreatedDefinition.Key(), emitted[0].DefinitionKey)
	})

	t.Run("metadata failure never reaches the repository", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		orgRepo := organization.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}

		mongoErr := errors.New("mongo unavailable")

		metaRepo.EXPECT().
			Create(gomock.Any(), constant.EntityOrganization, gomock.Any()).
			Return(mongoErr)

		got, err := uc.CreateOrganization(context.Background(), newInput())
		require.ErrorIs(t, err, mongoErr)
		assert.Nil(t, got)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})

	t.Run("insert failure discards the staged metadata", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		orgRepo := organization.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}

		pgErr := errors.New("insert failed")

		var mongoID, pgID string

		gomock.InOrder(
			metaRepo.EXPECT().
				Create(gomock.Any(), constant.EntityOrganization, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, m *mongodb.Metadata) error {
					mongoID = m.EntityID
					assert.Empty(t, emitter.Events(), "no event may be published before the insert")

					return nil
				}),
			orgRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, o *mmodel.Organization) (*mmodel.Organization, error) {
					pgID = o.ID

					return nil, pgErr
				}),
			metaRepo.EXPECT().
				Purge(gomock.Any(), constant.EntityOrganization, gomock.Any()).
				DoAndReturn(func(_ context.Context, _, id string) error {
					assert.Equal(t, mongoID, id, "the discard must target the staged document")

					return nil
				}),
		)

		got, err := uc.CreateOrganization(context.Background(), newInput())
		require.ErrorIs(t, err, pgErr)
		assert.Nil(t, got)
		assert.Equal(t, mongoID, pgID)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})

	t.Run("no metadata touches neither write nor discard", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		orgRepo := organization.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{OrganizationRepo: orgRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}

		pgErr := errors.New("insert failed")

		orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, pgErr)

		input := newInput()
		input.Metadata = nil

		_, err := uc.CreateOrganization(context.Background(), input)
		require.ErrorIs(t, err, pgErr)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})
}
