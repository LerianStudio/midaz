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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

func newPortfolioMetadataOrderUseCase(ctrl *gomock.Controller) (*UseCase, *portfolio.MockRepository, *mongodb.MockRepository, *pkgStreaming.MockEmitter) {
	portfolioRepo := portfolio.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	return &UseCase{
		PortfolioRepo:          portfolioRepo,
		OnboardingMetadataRepo: metadataRepo,
		Streaming:              emitter,
	}, portfolioRepo, metadataRepo, emitter
}

// TestCreatePortfolio_StagesMetadataBeforeInsert proves the metadata document is written before
// the PostgreSQL insert, keyed by the same ID the insert persists, and returned in the response.
func TestCreatePortfolio_StagesMetadataBeforeInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, portfolioRepo, metadataRepo, emitter := newPortfolioMetadataOrderUseCase(ctrl)

	input := &mmodel.CreatePortfolioInput{Name: "Ordered Portfolio", Metadata: map[string]any{"tier": "gold"}}

	var stagedID, insertedID string

	gomock.InOrder(
		metadataRepo.EXPECT().
			Create(gomock.Any(), constant.EntityPortfolio, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				stagedID = meta.EntityID
				assert.Empty(t, emitter.Events(), "no event may be published before the insert")

				return nil
			}),
		portfolioRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in *mmodel.Portfolio) (*mmodel.Portfolio, error) {
				insertedID = in.ID
				out := *in

				return &out, nil
			}),
	)

	result, err := uc.CreatePortfolio(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotEmpty(t, stagedID)
	assert.Equal(t, insertedID, stagedID, "metadata must be keyed by the ID sent to PostgreSQL")
	assert.Equal(t, insertedID, result.ID)
	assert.Equal(t, map[string]any{"tier": "gold"}, result.Metadata)
	assert.Len(t, emitter.Events(), 1, "the created event is published once after the insert")
}

// TestCreatePortfolio_MetadataFailureSkipsInsert proves a metadata-store failure aborts the create
// before PostgreSQL is touched and before any event is published.
func TestCreatePortfolio_MetadataFailureSkipsInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, portfolioRepo, metadataRepo, emitter := newPortfolioMetadataOrderUseCase(ctrl)

	metadataErr := errors.New("metadata store unavailable")

	gomock.InOrder(
		metadataRepo.EXPECT().
			Create(gomock.Any(), constant.EntityPortfolio, gomock.Any()).
			Return(metadataErr),
	)
	portfolioRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Times(0)
	metadataRepo.EXPECT().Purge(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	input := &mmodel.CreatePortfolioInput{Name: "Mongo Down Portfolio", Metadata: map[string]any{"tier": "gold"}}

	result, err := uc.CreatePortfolio(context.Background(), uuid.New(), uuid.New(), input)
	require.ErrorIs(t, err, metadataErr)
	assert.Nil(t, result)
	assert.Empty(t, emitter.Events())
}

// TestCreatePortfolio_InsertFailureDiscardsStagedMetadata proves a failed insert purges the
// staged metadata document under the same ID and returns the insert error unchanged.
func TestCreatePortfolio_InsertFailureDiscardsStagedMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, portfolioRepo, metadataRepo, emitter := newPortfolioMetadataOrderUseCase(ctrl)

	insertErr := errors.New("insert failed")

	var stagedID, insertedID string

	gomock.InOrder(
		metadataRepo.EXPECT().
			Create(gomock.Any(), constant.EntityPortfolio, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				stagedID = meta.EntityID

				return nil
			}),
		portfolioRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in *mmodel.Portfolio) (*mmodel.Portfolio, error) {
				insertedID = in.ID

				return nil, insertErr
			}),
		metadataRepo.EXPECT().
			Purge(gomock.Any(), constant.EntityPortfolio, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, id string) error {
				assert.Equal(t, stagedID, id, "discard must target the staged document")

				return nil
			}),
	)

	input := &mmodel.CreatePortfolioInput{Name: "Insert Fail Portfolio", Metadata: map[string]any{"tier": "gold"}}

	result, err := uc.CreatePortfolio(context.Background(), uuid.New(), uuid.New(), input)
	require.ErrorIs(t, err, insertErr)
	assert.Nil(t, result)
	assert.Equal(t, insertedID, stagedID)
	assert.Empty(t, emitter.Events())
}
