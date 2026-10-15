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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// accountOrderMocks holds the mocks whose call order the metadata-first tests assert.
type accountOrderMocks struct {
	account  *account.MockRepository
	metadata *mongodb.MockRepository
	balance  *balance.MockRepository
	emitter  *pkgStreaming.MockEmitter
}

// newAccountOrderUseCase wires a UseCase whose lookups all succeed, leaving the writes
// (metadata, account insert, balance, compensation) for each test to script.
func newAccountOrderUseCase(ctrl *gomock.Controller) (*UseCase, accountOrderMocks) {
	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	emitter := pkgStreaming.NewMockEmitter()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	mockAccountTypeRepo.EXPECT().
		FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("not found")).AnyTimes()

	mockAssetRepo.EXPECT().
		FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockBalanceRepo.EXPECT().
		ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	uc := &UseCase{
		AssetRepo:              mockAssetRepo,
		PortfolioRepo:          mockPortfolioRepo,
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		AccountTypeRepo:        mockAccountTypeRepo,
		BalanceRepo:            mockBalanceRepo,
		LedgerRepo:             mockLedgerRepo,
		Streaming:              emitter,
	}

	return uc, accountOrderMocks{account: mockAccountRepo, metadata: mockMetadataRepo, balance: mockBalanceRepo, emitter: emitter}
}

func accountOrderInput(metadata map[string]any) *mmodel.CreateAccountInput {
	return &mmodel.CreateAccountInput{
		Name:      "Metadata Order Account",
		Type:      "deposit",
		AssetCode: "USD",
		Metadata:  metadata,
	}
}

// TestCreateAccountMetadataOrder_SuccessWritesMetadataFirst asserts the canonical order of a
// create with metadata: document store, then account insert, then default balance, then the
// account.created event; and that the document is keyed by the same ID the insert persists.
func TestCreateAccountMetadataOrder_SuccessWritesMetadataFirst(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, m := newAccountOrderUseCase(ctrl)

	metadata := map[string]any{"key": "value"}

	var metadataEntityID, insertedID string

	gomock.InOrder(
		m.metadata.EXPECT().
			Create(gomock.Any(), constant.EntityAccount, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				metadataEntityID = meta.EntityID
				assert.Empty(t, m.emitter.Events(), "no event may be published before the insert")

				return nil
			}).Times(1),
		m.account.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
				insertedID = in.ID
				out := *in

				return &out, nil
			}).Times(1),
		m.balance.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Empty(t, m.emitter.Events(), "no event may be published before the default balance")

				return b, nil
			}).Times(1),
	)

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), accountOrderInput(metadata), "Bearer test", HolderOnV2)
	require.NoError(t, err)
	require.NotNil(t, acc)

	emitted := m.emitter.Events()
	require.Len(t, emitted, 1, "the created event is published once after the default balance")
	assert.Equal(t, events.AccountCreatedDefinition.Key(), emitted[0].DefinitionKey)
	require.NotEmpty(t, metadataEntityID)
	assert.Equal(t, insertedID, metadataEntityID, "metadata must be keyed by the ID the insert persists")
	assert.Equal(t, insertedID, acc.ID)
	assert.Equal(t, metadata, acc.Metadata)
}

// TestCreateAccountMetadataOrder_MetadataFailureSkipsInsert asserts that a failed metadata
// write aborts the create before any SQL write, balance or event.
func TestCreateAccountMetadataOrder_MetadataFailureSkipsInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, m := newAccountOrderUseCase(ctrl)

	metadataErr := errors.New("document store unavailable")

	m.metadata.EXPECT().
		Create(gomock.Any(), constant.EntityAccount, gomock.Any()).
		Return(metadataErr).Times(1)
	m.account.EXPECT().Create(gomock.Any(), gomock.Any()).Times(0)
	m.balance.EXPECT().Create(gomock.Any(), gomock.Any()).Times(0)
	m.metadata.EXPECT().Purge(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), accountOrderInput(map[string]any{"key": "value"}), "Bearer test", HolderOnV2)
	require.ErrorIs(t, err, metadataErr)
	assert.Nil(t, acc)
	assert.Empty(t, m.emitter.Events(), "no event may be published when the metadata write fails")
}

// TestCreateAccountMetadataOrder_InsertFailureDiscardsMetadata asserts that a failed account
// insert discards the staged document under the same ID and returns the insert error.
func TestCreateAccountMetadataOrder_InsertFailureDiscardsMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, m := newAccountOrderUseCase(ctrl)

	insertErr := errors.New("insert failed")

	var metadataEntityID, insertedID string

	gomock.InOrder(
		m.metadata.EXPECT().
			Create(gomock.Any(), constant.EntityAccount, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				metadataEntityID = meta.EntityID

				return nil
			}).Times(1),
		m.account.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
				insertedID = in.ID

				return nil, insertErr
			}).Times(1),
		m.metadata.EXPECT().
			Purge(gomock.Any(), constant.EntityAccount, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, id string) error {
				assert.Equal(t, metadataEntityID, id, "discard must target the staged document")

				return nil
			}).Times(1),
	)
	m.balance.EXPECT().Create(gomock.Any(), gomock.Any()).Times(0)

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), accountOrderInput(map[string]any{"key": "value"}), "Bearer test", HolderOnV2)
	require.ErrorIs(t, err, insertErr)
	assert.Nil(t, acc)
	assert.Equal(t, insertedID, metadataEntityID)
	assert.Empty(t, m.emitter.Events(), "no event may be published when the insert fails")
}

// TestCreateAccountMetadataOrder_BalanceFailureCompensatesBoth asserts that a failed default
// balance deletes the account and then purges the staged document, and still returns the
// account-creation-failed business error when the purge itself fails.
func TestCreateAccountMetadataOrder_BalanceFailureCompensatesBoth(t *testing.T) {
	tests := []struct {
		name     string
		purgeErr error
	}{
		{name: "purge succeeds"},
		{name: "purge fails", purgeErr: errors.New("purge failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc, m := newAccountOrderUseCase(ctrl)

			organizationID := uuid.New()
			ledgerID := uuid.New()

			var metadataEntityID string

			gomock.InOrder(
				m.metadata.EXPECT().
					Create(gomock.Any(), constant.EntityAccount, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
						metadataEntityID = meta.EntityID

						return nil
					}).Times(1),
				m.account.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
						out := *in

						return &out, nil
					}).Times(1),
				m.balance.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("create balance error")).Times(1),
				m.account.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, id uuid.UUID) error {
						assert.Equal(t, metadataEntityID, id.String(), "account delete must target the inserted account")

						return nil
					}).Times(1),
				m.metadata.EXPECT().
					Purge(gomock.Any(), constant.EntityAccount, gomock.Any()).
					DoAndReturn(func(_ context.Context, _, id string) error {
						assert.Equal(t, metadataEntityID, id, "discard must target the staged document")

						return tt.purgeErr
					}).Times(1),
			)

			acc, err := uc.CreateAccount(context.Background(), organizationID, ledgerID, accountOrderInput(map[string]any{"key": "value"}), "Bearer test", HolderOnV2)
			require.Error(t, err)
			assert.Nil(t, acc)

			assert.Equal(t, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, constant.EntityAccount), err)

			assert.Empty(t, m.emitter.Events(), "no event may be published when the default balance fails")
		})
	}
}

// TestCreateAccountMetadataOrder_FailedCompensationKeepsMetadata asserts that when the
// compensating account delete fails the account survives, so its staged metadata is kept: the
// document store is never purged, and the create still fails with the business error.
func TestCreateAccountMetadataOrder_FailedCompensationKeepsMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, m := newAccountOrderUseCase(ctrl)

	gomock.InOrder(
		m.metadata.EXPECT().
			Create(gomock.Any(), constant.EntityAccount, gomock.Any()).
			Return(nil).Times(1),
		m.account.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
				out := *in

				return &out, nil
			}).Times(1),
		m.balance.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("create balance error")).Times(1),
		m.account.EXPECT().
			Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("delete failed")).Times(1),
	)
	m.metadata.EXPECT().Purge(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), accountOrderInput(map[string]any{"key": "value"}), "Bearer test", HolderOnV2)
	require.Error(t, err)
	assert.Nil(t, acc)
	assert.Equal(t, pkg.ValidateBusinessError(constant.ErrAccountCreationFailed, constant.EntityAccount), err)
	assert.Empty(t, m.emitter.Events(), "no event may be published when the default balance fails")
}

// TestCreateAccountMetadataOrder_NoMetadataSkipsDocumentStore asserts that a create without
// metadata never touches the document store, neither to write nor to discard.
func TestCreateAccountMetadataOrder_NoMetadataSkipsDocumentStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc, m := newAccountOrderUseCase(ctrl)

	m.metadata.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	m.metadata.EXPECT().Purge(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	m.account.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in

			return &out, nil
		}).Times(1)
	m.balance.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("create balance error")).Times(1)
	m.account.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(1)

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), accountOrderInput(nil), "Bearer test", HolderOnV2)
	require.Error(t, err)
	assert.Nil(t, acc)
}
