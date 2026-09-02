// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// R11 closes the gap where a balance was born with account_blocked=false under
// an account that is (or becomes) blocked. Four creation sites feed the balance
// read model and every one of them must inherit the owning account's block
// state at INSERT time, then re-verify it against the account row afterwards so
// the creation x block interleaving cannot leave the two disagreeing.
//
// The allow flags are a SEPARATE mechanism (RF-03) and MUST NOT move: every
// assertion below pins them to the value the site produces today.

// allowBlockReverificationRead arms the post-INSERT account re-read that every
// balance-creation path now performs, for suites whose subject is not the block
// projection. It answers with the block state given, so those suites stay
// converged and no realigning UPDATE is issued.
//
// It MUST be declared after a suite's own Find expectations: gomock serves the
// first non-exhausted match, so a catch-all declared first would shadow them.
func allowBlockReverificationRead(m *account.MockRepository, blocked *bool) {
	m.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, _ *uuid.UUID, id uuid.UUID, _ mmodel.HolderPolicy) (*mmodel.Account, error) {
			return blockedAccountRecord(orgID, ledgerID, id, blocked), nil
		}).AnyTimes()
}

// unblockedAccountRepo is the drop-in AccountRepo for balance suites that are
// not about the block projection: every inheritance and re-verification read
// answers "not blocked", which matches the balances those suites create, so no
// realigning UPDATE is ever issued.
func unblockedAccountRepo(t *testing.T) *account.MockRepository {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	m := account.NewMockRepository(ctrl)
	allowBlockReverificationRead(m, nil)

	return m
}

// blockedAccountRecord builds the account record the reconcile read returns.
func blockedAccountRecord(organizationID, ledgerID, accountID uuid.UUID, blocked *bool) *mmodel.Account {
	return &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "Inheritance Account",
		Type:           "deposit",
		AssetCode:      "USD",
		Blocked:        blocked,
	}
}

// TestCreateDefaultBalance_PersistsInheritedAccountBlocked pins site (1)+(2)'s
// persistence leg: CreateBalanceInput.AccountBlocked must reach the persisted
// mmodel.Balance. Without it the field is silently dropped between the input
// the caller fills and the row the repository writes.
func TestCreateDefaultBalance_PersistsInheritedAccountBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		accountBlocked bool
	}{
		{name: "blocked account seeds a blocked default balance", accountBlocked: true},
		{name: "unblocked account seeds an unblocked default balance", accountBlocked: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			uc := &UseCase{BalanceRepo: mockBalanceRepo}

			organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()

			mockBalanceRepo.EXPECT().
				ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
				Return(false, nil).
				Times(1)

			mockBalanceRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
					assert.Equal(t, tt.accountBlocked, b.AccountBlocked,
						"the default balance must inherit the account block state carried by the input")
					assert.True(t, b.AllowSending, "allow flags are an independent mechanism and must not move")
					assert.True(t, b.AllowReceiving, "allow flags are an independent mechanism and must not move")

					return b, nil
				}).
				Times(1)

			created, err := uc.CreateDefaultBalance(context.Background(), mmodel.CreateBalanceInput{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "@inherit",
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AccountBlocked: tt.accountBlocked,
			})

			require.NoError(t, err)
			require.NotNil(t, created)
			assert.Equal(t, tt.accountBlocked, created.AccountBlocked)
		})
	}
}

// createAccountInheritanceFixture wires the minimum CreateAccount surface with
// permissive expectations, leaving the balance Create and the reconcile reads
// to the individual test.
type createAccountInheritanceFixture struct {
	uc          *UseCase
	accountRepo *account.MockRepository
	balanceRepo *balance.MockRepository

	organizationID uuid.UUID
	ledgerID       uuid.UUID
}

func newCreateAccountInheritanceFixture(t *testing.T) *createAccountInheritanceFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	f := &createAccountInheritanceFixture{
		accountRepo:    account.NewMockRepository(ctrl),
		balanceRepo:    balance.NewMockRepository(ctrl),
		organizationID: uuid.New(),
		ledgerID:       uuid.New(),
	}

	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	accountTypeRepo := accounttype.NewMockRepository(ctrl)
	ledgerRepo := ledger.NewMockRepository(ctrl)

	ledgerRepo.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	accountTypeRepo.EXPECT().FindByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).AnyTimes()
	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()
	metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	f.accountRepo.EXPECT().FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()
	f.accountRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			return &out, nil
		}).AnyTimes()

	f.balanceRepo.EXPECT().
		ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	f.uc = &UseCase{
		AssetRepo:              assetRepo,
		AccountRepo:            f.accountRepo,
		OnboardingMetadataRepo: metadataRepo,
		AccountTypeRepo:        accountTypeRepo,
		BalanceRepo:            f.balanceRepo,
		LedgerRepo:             ledgerRepo,
	}

	return f
}

// TestCreateAccount_DefaultBalanceInheritsBlockedState covers site (1): an
// account created with blocked=true must seed a default balance that already
// carries the block projection, with the allow flags untouched.
func TestCreateAccount_DefaultBalanceInheritsBlockedState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		inputParam  *bool
		wantBlocked bool
	}{
		{name: "blocked=true is inherited by the default balance", inputParam: boolPtr(true), wantBlocked: true},
		{name: "blocked=false is inherited by the default balance", inputParam: boolPtr(false), wantBlocked: false},
		{name: "blocked omitted defaults to an unblocked balance", inputParam: nil, wantBlocked: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newCreateAccountInheritanceFixture(t)

			f.balanceRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
					assert.Equal(t, tt.wantBlocked, b.AccountBlocked,
						"default balance must be born with the account's block state")
					assert.True(t, b.AllowSending)
					assert.True(t, b.AllowReceiving)

					return b, nil
				}).
				Times(1)

			// Post-insert re-verification reads the account back. Converged
			// here, so no realigning UPDATE may be issued.
			f.accountRepo.EXPECT().
				Find(gomock.Any(), f.organizationID, f.ledgerID, nil, gomock.Any(), mmodel.HolderOffV1).
				DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, _ *uuid.UUID, id uuid.UUID, _ mmodel.HolderPolicy) (*mmodel.Account, error) {
					return blockedAccountRecord(orgID, ledgerID, id, boolPtr(tt.wantBlocked)), nil
				}).
				Times(1)

			acc, err := f.uc.CreateAccount(context.Background(), f.organizationID, f.ledgerID, &mmodel.CreateAccountInput{
				Name:      "Inheritance Account",
				Type:      "deposit",
				AssetCode: "USD",
				Blocked:   tt.inputParam,
			}, "", HolderOffV1)

			require.NoError(t, err)
			require.NotNil(t, acc)
		})
	}
}

// TestCreateAccount_ReverificationRealignsAfterConcurrentBlock is the race the
// insert+re-verification pair exists for: a BlockAccount call lands between the
// account INSERT and the balance INSERT, so its balance-wide UPDATE ran before
// the row existed. Re-reading the account after the balance INSERT catches the
// divergence and realigns every balance of the account.
func TestCreateAccount_ReverificationRealignsAfterConcurrentBlock(t *testing.T) {
	t.Parallel()

	f := newCreateAccountInheritanceFixture(t)

	f.balanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.False(t, b.AccountBlocked, "the row is written with the state observed at creation time")
			return b, nil
		}).
		Times(1)

	// The re-read observes the concurrent block.
	f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, gomock.Any(), mmodel.HolderOffV1).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, _ *uuid.UUID, id uuid.UUID, _ mmodel.HolderPolicy) (*mmodel.Account, error) {
			return blockedAccountRecord(orgID, ledgerID, id, boolPtr(true)), nil
		}).
		Times(1)

	f.balanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), f.organizationID, f.ledgerID, gomock.Any(), true).
		Return(nil).
		Times(1)

	acc, err := f.uc.CreateAccount(context.Background(), f.organizationID, f.ledgerID, &mmodel.CreateAccountInput{
		Name:      "Raced Account",
		Type:      "deposit",
		AssetCode: "USD",
		Blocked:   boolPtr(false),
	}, "", HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, acc)
}

// TestCreateAccount_ReverificationFailureSurfaces keeps the guarantee
// fail-closed: an unprovable projection must not be confirmed to the caller.
func TestCreateAccount_ReverificationFailureSurfaces(t *testing.T) {
	t.Parallel()

	f := newCreateAccountInheritanceFixture(t)

	realignErr := errors.New("balance projection update exploded")

	f.balanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) { return b, nil }).
		Times(1)

	f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, gomock.Any(), mmodel.HolderOffV1).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, _ *uuid.UUID, id uuid.UUID, _ mmodel.HolderPolicy) (*mmodel.Account, error) {
			return blockedAccountRecord(orgID, ledgerID, id, boolPtr(true)), nil
		}).
		Times(1)

	f.balanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), f.organizationID, f.ledgerID, gomock.Any(), true).
		Return(realignErr).
		Times(1)

	acc, err := f.uc.CreateAccount(context.Background(), f.organizationID, f.ledgerID, &mmodel.CreateAccountInput{
		Name:      "Raced Account",
		Type:      "deposit",
		AssetCode: "USD",
	}, "", HolderOffV1)

	require.Error(t, err)
	assert.Nil(t, acc)
}

// TestCreateAsset_ExternalDefaultBalanceDerivesAccountBlocked covers site (2).
// External accounts cannot be blocked (guard 0074), so the derived value is
// always false in practice — but it MUST be derived from the persisted account
// rather than hardcoded, so the mechanism stays uniform across the four sites.
func TestCreateAsset_ExternalDefaultBalanceDerivesAccountBlocked(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	assetRepo := asset.NewMockRepository(ctrl)
	accountRepo := account.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	organizationID, ledgerID := uuid.New(), uuid.New()

	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()
	assetRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, a *mmodel.Asset) (*mmodel.Asset, error) {
			out := *a
			out.ID = uuid.New().String()
			return &out, nil
		}).Times(1)
	metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	accountRepo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.Account{}, nil).Times(1)

	var createdAccountID string

	accountRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			createdAccountID = out.ID

			return &out, nil
		}).Times(1)

	balanceRepo.EXPECT().ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).Times(1)
	balanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.False(t, b.AccountBlocked, "an external account is never blocked, so the derived state is false")
			assert.True(t, b.AllowSending)
			assert.True(t, b.AllowReceiving)

			return b, nil
		}).Times(1)

	accountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, nil, gomock.Any(), mmodel.HolderOffV1).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, _ *uuid.UUID, id uuid.UUID, _ mmodel.HolderPolicy) (*mmodel.Account, error) {
			require.Equal(t, createdAccountID, id.String())
			acc := blockedAccountRecord(orgID, ledgerID, id, boolPtr(false))
			acc.Type = "external"

			return acc, nil
		}).Times(1)

	uc := &UseCase{
		AssetRepo:              assetRepo,
		AccountRepo:            accountRepo,
		BalanceRepo:            balanceRepo,
		OnboardingMetadataRepo: metadataRepo,
	}

	created, err := uc.CreateAsset(context.Background(), organizationID, ledgerID, &mmodel.CreateAssetInput{
		Name: "US Dollar",
		Type: "currency",
		Code: "USD",
	}, "")

	require.NoError(t, err)
	require.NotNil(t, created)
}

// additionalBalanceInheritanceFixture wires the CreateAdditionalBalance surface.
type additionalBalanceInheritanceFixture struct {
	uc          *UseCase
	accountRepo *account.MockRepository
	balanceRepo *balance.MockRepository

	organizationID uuid.UUID
	ledgerID       uuid.UUID
	accountID      uuid.UUID
	defaultBalance *mmodel.Balance
}

func newAdditionalBalanceInheritanceFixture(t *testing.T) *additionalBalanceInheritanceFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	f := &additionalBalanceInheritanceFixture{
		accountRepo:    account.NewMockRepository(ctrl),
		balanceRepo:    balance.NewMockRepository(ctrl),
		organizationID: uuid.New(),
		ledgerID:       uuid.New(),
		accountID:      uuid.New(),
	}

	f.defaultBalance = &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          "@inherit",
		Key:            constant.DefaultBalanceKey,
		OrganizationID: f.organizationID.String(),
		LedgerID:       f.ledgerID.String(),
		AccountID:      f.accountID.String(),
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
	}

	f.uc = &UseCase{AccountRepo: f.accountRepo, BalanceRepo: f.balanceRepo}

	return f
}

// TestCreateAdditionalBalance_InheritsAccountBlocked covers site (3): a balance
// created under an already-blocked account must be born blocked, otherwise the
// new key is an open door on a closed account.
func TestCreateAdditionalBalance_InheritsAccountBlocked(t *testing.T) {
	t.Parallel()

	f := newAdditionalBalanceInheritanceFixture(t)

	f.balanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, "savings").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)
	f.balanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, constant.DefaultBalanceKey).
		Return(f.defaultBalance, nil).
		Times(1)

	// Inheritance read: the owning account is blocked at creation time.
	f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
		Return(blockedAccountRecord(f.organizationID, f.ledgerID, f.accountID, boolPtr(true)), nil).
		Times(2) // one before the INSERT, one for the post-INSERT re-verification

	f.balanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.True(t, b.AccountBlocked, "an additional balance under a blocked account must be born blocked")
			assert.True(t, b.AllowSending, "allow flags stay independent of the block mechanism")
			assert.True(t, b.AllowReceiving)

			return b, nil
		}).
		Times(1)

	created, err := f.uc.CreateAdditionalBalance(context.Background(), f.organizationID, f.ledgerID, f.accountID,
		&mmodel.CreateAdditionalBalance{Key: "savings"})

	require.NoError(t, err)
	require.NotNil(t, created)
	assert.True(t, created.AccountBlocked)
}

// TestCreateAdditionalBalance_OverdraftCompanionInheritsAccountBlocked covers
// site (4) through its create-time caller: the system-managed companion is a
// balance of the same account and must carry the same projection.
func TestCreateAdditionalBalance_OverdraftCompanionInheritsAccountBlocked(t *testing.T) {
	t.Parallel()

	f := newAdditionalBalanceInheritanceFixture(t)

	f.balanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, "savings").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)
	f.balanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, constant.DefaultBalanceKey).
		Return(f.defaultBalance, nil).
		Times(1)
	f.balanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, constant.OverdraftBalanceKey).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
		Return(blockedAccountRecord(f.organizationID, f.ledgerID, f.accountID, boolPtr(true)), nil).
		Times(2)

	seen := map[string]bool{}

	f.balanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			seen[b.Key] = b.AccountBlocked
			return b, nil
		}).
		Times(2)

	created, err := f.uc.CreateAdditionalBalance(context.Background(), f.organizationID, f.ledgerID, f.accountID,
		&mmodel.CreateAdditionalBalance{
			Key:      "savings",
			Settings: &mmodel.BalanceSettings{AllowOverdraft: true},
		})

	require.NoError(t, err)
	require.NotNil(t, created)
	assert.True(t, seen[constant.OverdraftBalanceKey], "the overdraft companion must inherit the account block state")
	assert.True(t, seen["savings"], "the parent balance must inherit the account block state")
}

// newBlockProjectionHelperFixture wires only the two ports the inheritance and
// re-verification helpers touch, so their branches can be driven directly
// instead of through a full creation flow.
func newBlockProjectionHelperFixture(t *testing.T) (*UseCase, *account.MockRepository, *balance.MockRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	accountRepo := account.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)

	return &UseCase{AccountRepo: accountRepo, BalanceRepo: balanceRepo}, accountRepo, balanceRepo
}

// TestResolveAccountBlockedState covers the read that both helpers sit on: a
// repository failure must propagate (guessing "not blocked" would mint an open
// balance under a possibly closed account) and a missing row must be reported as
// not-found rather than as an unblocked account.
func TestResolveAccountBlockedState(t *testing.T) {
	t.Parallel()

	readErr := errors.New("accounts table unreachable")

	tests := []struct {
		name        string
		record      *mmodel.Account
		repoErr     error
		wantBlocked bool
		wantFound   bool
		wantErr     error
	}{
		{
			name:      "repository failure propagates instead of defaulting to unblocked",
			repoErr:   readErr,
			wantErr:   readErr,
			wantFound: false,
		},
		{
			name:      "missing account is reported as not found",
			record:    nil,
			wantFound: false,
		},
		{
			name:        "nil blocked column reads as not blocked",
			record:      &mmodel.Account{Blocked: nil},
			wantBlocked: false,
			wantFound:   true,
		},
		{
			name:        "blocked column is reported verbatim",
			record:      &mmodel.Account{Blocked: boolPtr(true)},
			wantBlocked: true,
			wantFound:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, accountRepo, _ := newBlockProjectionHelperFixture(t)
			organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()

			accountRepo.EXPECT().
				Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
				Return(tt.record, tt.repoErr).
				Times(1)

			blocked, found, err := uc.resolveAccountBlockedState(context.Background(), organizationID, ledgerID, accountID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantBlocked, blocked)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

// TestInheritAccountBlockedState_PropagatesReadFailure pins the fail-closed
// posture of the inheritance leg.
func TestInheritAccountBlockedState_PropagatesReadFailure(t *testing.T) {
	t.Parallel()

	uc, accountRepo, _ := newBlockProjectionHelperFixture(t)
	organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()

	readErr := errors.New("accounts table unreachable")

	accountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
		Return(nil, readErr).
		Times(1)

	blocked, err := uc.inheritAccountBlockedState(context.Background(), organizationID, ledgerID, accountID)

	require.ErrorIs(t, err, readErr)
	assert.False(t, blocked)
}

// TestReconcileBalanceAccountBlocked covers the re-verification branches that
// the creation flows cannot reach: the nil account id used by minimal fixtures,
// an account deleted between the INSERT and the re-read, and a failing read.
func TestReconcileBalanceAccountBlocked(t *testing.T) {
	t.Parallel()

	readErr := errors.New("accounts table unreachable")

	t.Run("nil account id reads nothing and realigns nothing", func(t *testing.T) {
		t.Parallel()

		uc, _, _ := newBlockProjectionHelperFixture(t)

		// No expectations armed: any repository call fails the controller.
		require.NoError(t, uc.reconcileBalanceAccountBlocked(context.Background(), uuid.New(), uuid.New(), uuid.Nil, false))
	})

	t.Run("account deleted before the re-read leaves nothing to converge", func(t *testing.T) {
		t.Parallel()

		uc, accountRepo, _ := newBlockProjectionHelperFixture(t)
		organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()

		accountRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
			Return(nil, nil).
			Times(1)

		// No UpdateAccountBlockedByAccountID expectation: realigning the
		// balances of an account that no longer exists is meaningless.
		require.NoError(t, uc.reconcileBalanceAccountBlocked(context.Background(), organizationID, ledgerID, accountID, true))
	})

	t.Run("read failure surfaces so the creation is not confirmed", func(t *testing.T) {
		t.Parallel()

		uc, accountRepo, _ := newBlockProjectionHelperFixture(t)
		organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()

		accountRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
			Return(nil, readErr).
			Times(1)

		require.ErrorIs(t, uc.reconcileBalanceAccountBlocked(context.Background(), organizationID, ledgerID, accountID, false), readErr)
	})

	t.Run("unblock racing a creation realigns downward too", func(t *testing.T) {
		t.Parallel()

		uc, accountRepo, balanceRepo := newBlockProjectionHelperFixture(t)
		organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()

		accountRepo.EXPECT().
			Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
			Return(blockedAccountRecord(organizationID, ledgerID, accountID, boolPtr(false)), nil).
			Times(1)

		balanceRepo.EXPECT().
			UpdateAccountBlockedByAccountID(gomock.Any(), organizationID, ledgerID, accountID, false).
			Return(nil).
			Times(1)

		require.NoError(t, uc.reconcileBalanceAccountBlocked(context.Background(), organizationID, ledgerID, accountID, true))
	})
}

// TestReverifyCompanionBlockProjection covers the overdraft companion wrapper's
// guards: nothing created, no account to reconcile against, and a malformed
// account id.
func TestReverifyCompanionBlockProjection(t *testing.T) {
	t.Parallel()

	accountID := uuid.New()

	tests := []struct {
		name      string
		current   *mmodel.Balance
		companion *mmodel.Balance
		wantErr   bool
	}{
		{
			name:      "no companion created is a no-op",
			current:   &mmodel.Balance{AccountID: accountID.String()},
			companion: nil,
		},
		{
			name:      "nil current balance is a no-op",
			current:   nil,
			companion: &mmodel.Balance{},
		},
		{
			name:      "empty account id names no account to reconcile against",
			current:   &mmodel.Balance{AccountID: ""},
			companion: &mmodel.Balance{},
		},
		{
			name:      "malformed account id surfaces instead of being skipped",
			current:   &mmodel.Balance{AccountID: "not-a-uuid"},
			companion: &mmodel.Balance{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, _, _ := newBlockProjectionHelperFixture(t)

			ctx := context.Background()
			logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
			ctx, span := tracer.Start(ctx, "test.reverify_companion")

			defer span.End()

			// No repository expectations: every case below must short-circuit
			// before reaching persistence.
			err := uc.reverifyCompanionBlockProjection(ctx, span, logger, uuid.New(), uuid.New(), tt.current, tt.companion)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestCreateAdditionalBalance_BlockProjectionFailuresSurface pins the
// fail-closed posture on the additional-balance path: neither the inheritance
// read nor the post-INSERT re-verification may be swallowed, because a balance
// whose block projection is unknown is a balance the hot path would treat as
// transactable.
func TestCreateAdditionalBalance_BlockProjectionFailuresSurface(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("accounts table unreachable")

	t.Run("inheritance read failure aborts before the balance is written", func(t *testing.T) {
		t.Parallel()

		f := newAdditionalBalanceInheritanceFixture(t)

		f.balanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, "savings").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
			Times(1)
		f.balanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, constant.DefaultBalanceKey).
			Return(f.defaultBalance, nil).
			Times(1)

		f.accountRepo.EXPECT().
			Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
			Return(nil, repoErr).
			Times(1)

		// No BalanceRepo.Create expectation: nothing may be written when the
		// state the row must be born with cannot be established.
		created, err := f.uc.CreateAdditionalBalance(context.Background(), f.organizationID, f.ledgerID, f.accountID,
			&mmodel.CreateAdditionalBalance{Key: "savings"})

		require.ErrorIs(t, err, repoErr)
		assert.Nil(t, created)
	})

	t.Run("re-verification failure is not confirmed to the caller", func(t *testing.T) {
		t.Parallel()

		f := newAdditionalBalanceInheritanceFixture(t)

		f.balanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, "savings").
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
			Times(1)
		f.balanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, constant.DefaultBalanceKey).
			Return(f.defaultBalance, nil).
			Times(1)

		gomock.InOrder(
			f.accountRepo.EXPECT().
				Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
				Return(blockedAccountRecord(f.organizationID, f.ledgerID, f.accountID, boolPtr(false)), nil).
				Times(1),
			f.accountRepo.EXPECT().
				Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
				Return(nil, repoErr).
				Times(1),
		)

		f.balanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) { return b, nil }).
			Times(1)

		created, err := f.uc.CreateAdditionalBalance(context.Background(), f.organizationID, f.ledgerID, f.accountID,
			&mmodel.CreateAdditionalBalance{Key: "savings"})

		require.ErrorIs(t, err, repoErr)
		assert.Nil(t, created)
	})
}
