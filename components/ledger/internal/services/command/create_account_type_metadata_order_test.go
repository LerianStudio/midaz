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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func newAccountTypeMetadataOrderUseCase(ctrl *gomock.Controller) (*UseCase, *accounttype.MockRepository, *mongodb.MockRepository) {
	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	return &UseCase{
		AccountTypeRepo:        accountTypeRepo,
		OnboardingMetadataRepo: metadataRepo,
	}, accountTypeRepo, metadataRepo
}

func newAccountTypeMetadataOrderInput() *mmodel.CreateAccountTypeInput {
	return &mmodel.CreateAccountTypeInput{
		Name:     "Ordered Type",
		KeyValue: "ordered_type",
		Metadata: map[string]any{"tier": "gold"},
	}
}

// TestCreateAccountType_StagesMetadataBeforeInsert proves the metadata document is written
// before the PostgreSQL insert, keyed by the same ID the insert persists, and returned in the
// response.
func TestCreateAccountType_StagesMetadataBeforeInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, accountTypeRepo, metadataRepo := newAccountTypeMetadataOrderUseCase(ctrl)

	organizationID, ledgerID := uuid.New(), uuid.New()

	var (
		stagedID   string
		insertedID uuid.UUID
	)

	gomock.InOrder(
		metadataRepo.EXPECT().
			Create(gomock.Any(), constant.EntityAccountType, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				stagedID = meta.EntityID

				return nil
			}),
		accountTypeRepo.EXPECT().
			Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ uuid.UUID, in *mmodel.AccountType) (*mmodel.AccountType, error) {
				insertedID = in.ID
				out := *in

				return &out, nil
			}),
	)

	result, err := uc.CreateAccountType(context.Background(), organizationID, ledgerID, newAccountTypeMetadataOrderInput())
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotEmpty(t, stagedID)
	assert.Equal(t, insertedID.String(), stagedID, "metadata must be keyed by the ID sent to PostgreSQL")
	assert.Equal(t, insertedID, result.ID)
	assert.Equal(t, map[string]any{"tier": "gold"}, result.Metadata)
}

// TestCreateAccountType_MetadataFailureSkipsInsert proves a metadata-store failure aborts the
// create before PostgreSQL is touched.
func TestCreateAccountType_MetadataFailureSkipsInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, accountTypeRepo, metadataRepo := newAccountTypeMetadataOrderUseCase(ctrl)

	metadataErr := errors.New("metadata store unavailable")

	metadataRepo.EXPECT().
		Create(gomock.Any(), constant.EntityAccountType, gomock.Any()).
		Return(metadataErr)
	accountTypeRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	metadataRepo.EXPECT().Purge(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	result, err := uc.CreateAccountType(context.Background(), uuid.New(), uuid.New(), newAccountTypeMetadataOrderInput())
	require.ErrorIs(t, err, metadataErr)
	assert.Nil(t, result)
}

// TestCreateAccountType_InsertFailureDiscardsStagedMetadata proves a failed insert purges
// the staged metadata document under the same ID and returns the insert error unchanged.
func TestCreateAccountType_InsertFailureDiscardsStagedMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, accountTypeRepo, metadataRepo := newAccountTypeMetadataOrderUseCase(ctrl)

	insertErr := errors.New("insert failed")

	var (
		stagedID   string
		insertedID uuid.UUID
	)

	gomock.InOrder(
		metadataRepo.EXPECT().
			Create(gomock.Any(), constant.EntityAccountType, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				stagedID = meta.EntityID

				return nil
			}),
		accountTypeRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ uuid.UUID, in *mmodel.AccountType) (*mmodel.AccountType, error) {
				insertedID = in.ID

				return nil, insertErr
			}),
		metadataRepo.EXPECT().
			Purge(gomock.Any(), constant.EntityAccountType, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, id string) error {
				assert.Equal(t, stagedID, id, "discard must target the staged document")

				return nil
			}),
	)

	result, err := uc.CreateAccountType(context.Background(), uuid.New(), uuid.New(), newAccountTypeMetadataOrderInput())
	require.ErrorIs(t, err, insertErr)
	assert.Nil(t, result)
	assert.Equal(t, insertedID.String(), stagedID)
}
