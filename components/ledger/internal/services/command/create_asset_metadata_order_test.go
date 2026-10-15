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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// TestCreateAsset_MetadataStagedBeforeInsert locks the create order: the name/code check
// runs first, then the metadata document is written keyed by the ID the use case generated
// and handed to the repository, and only then the insert and the external-account step; a
// metadata failure never reaches the insert, and an insert failure discards the staged
// document.
func TestCreateAsset_MetadataStagedBeforeInsert(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{"issuer": "central-bank"}

	newInput := func() *mmodel.CreateAssetInput {
		return &mmodel.CreateAssetInput{Name: "US Dollar", Type: "currency", Code: "USD", Metadata: metadata}
	}

	t.Run("success writes metadata before insert with the same ID", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		assetRepo := asset.NewMockRepository(ctrl)
		accountRepo := account.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{AssetRepo: assetRepo, AccountRepo: accountRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}
		organizationID, ledgerID := uuid.New(), uuid.New()
		externalAlias := constant.DefaultExternalAccountAliasPrefix + "USD"

		var mongoID, pgID string

		gomock.InOrder(
			assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), organizationID, ledgerID, "US Dollar", "USD").Return(false, nil),
			metaRepo.EXPECT().
				Create(gomock.Any(), constant.EntityAsset, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, m *mongodb.Metadata) error {
					mongoID = m.EntityID
					assert.Empty(t, emitter.Events(), "no event may be published before the insert")

					return nil
				}),
			assetRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, a *mmodel.Asset) (*mmodel.Asset, error) {
					pgID = a.ID
					out := *a

					return &out, nil
				}),
			// The external account already exists, so the post-commit step only looks it up.
			accountRepo.EXPECT().
				ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{externalAlias}).
				Return([]*mmodel.Account{{ID: uuid.New().String(), Alias: &externalAlias}}, nil),
		)

		got, err := uc.CreateAsset(context.Background(), organizationID, ledgerID, newInput(), "")
		require.NoError(t, err)
		require.NotNil(t, got)

		_, parseErr := uuid.Parse(mongoID)
		require.NoError(t, parseErr, "the use case must generate a UUID before staging metadata")
		assert.Equal(t, mongoID, pgID)
		assert.Equal(t, pgID, got.ID)
		assert.Equal(t, metadata, got.Metadata)

		emitted := emitter.Events()
		require.Len(t, emitted, 1, "the created event is published once after the insert")
		assert.Equal(t, events.AssetCreatedDefinition.Key(), emitted[0].DefinitionKey)
	})

	t.Run("metadata failure never reaches the insert", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		assetRepo := asset.NewMockRepository(ctrl)
		accountRepo := account.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{AssetRepo: assetRepo, AccountRepo: accountRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}
		organizationID, ledgerID := uuid.New(), uuid.New()
		mongoErr := errors.New("mongo unavailable")

		gomock.InOrder(
			assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).Return(false, nil),
			metaRepo.EXPECT().Create(gomock.Any(), constant.EntityAsset, gomock.Any()).Return(mongoErr),
		)

		got, err := uc.CreateAsset(context.Background(), organizationID, ledgerID, newInput(), "")
		require.ErrorIs(t, err, mongoErr)
		assert.Nil(t, got)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})

	t.Run("insert failure discards the staged metadata", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		assetRepo := asset.NewMockRepository(ctrl)
		accountRepo := account.NewMockRepository(ctrl)
		metaRepo := mongodb.NewMockRepository(ctrl)

		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{AssetRepo: assetRepo, AccountRepo: accountRepo, OnboardingMetadataRepo: metaRepo, Streaming: emitter}
		organizationID, ledgerID := uuid.New(), uuid.New()
		pgErr := errors.New("insert failed")

		var mongoID, pgID string

		gomock.InOrder(
			assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).Return(false, nil),
			metaRepo.EXPECT().
				Create(gomock.Any(), constant.EntityAsset, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, m *mongodb.Metadata) error {
					mongoID = m.EntityID
					assert.Empty(t, emitter.Events(), "no event may be published before the insert")

					return nil
				}),
			assetRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, a *mmodel.Asset) (*mmodel.Asset, error) {
					pgID = a.ID

					return nil, pgErr
				}),
			metaRepo.EXPECT().
				Purge(gomock.Any(), constant.EntityAsset, gomock.Any()).
				DoAndReturn(func(_ context.Context, _, id string) error {
					assert.Equal(t, mongoID, id, "the discard must target the staged document")

					return nil
				}),
		)

		got, err := uc.CreateAsset(context.Background(), organizationID, ledgerID, newInput(), "")
		require.ErrorIs(t, err, pgErr)
		assert.Nil(t, got)
		assert.Equal(t, mongoID, pgID)
		assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
	})
}
