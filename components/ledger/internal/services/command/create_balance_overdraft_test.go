// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// defaultBalanceForOverdraft builds a default balance fixture used as the
// reference account for additional-balance creation in these tests.
func defaultBalanceForOverdraft(orgID, ledgerID, accountID uuid.UUID, alias string) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          alias,
		Key:            constant.DefaultBalanceKey,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
}

// TestCreateAdditionalBalance_WithDirection verifies that when the caller
// provides Direction="debit", the created balance carries that direction
// instead of the default "credit".
func TestCreateAdditionalBalance_WithDirection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@debit-holder"

	direction := constant.DirectionDebit
	allow := true

	cbi := &mmodel.CreateAdditionalBalance{
		Key:            "savings",
		AllowSending:   &allow,
		AllowReceiving: &allow,
		Direction:      &direction,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "savings").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.DefaultBalanceKey).
		Return(defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias), nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.Equal(t, constant.DirectionDebit, b.Direction,
				"created balance MUST carry the requested direction")
			return b, nil
		}).
		Times(1)

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, constant.DirectionDebit, result.Direction,
		"returned balance MUST expose the requested direction")
}

// TestCreateAdditionalBalance_DefaultDirection verifies that when Direction
// is omitted from the request, the created balance defaults to "credit".
func TestCreateAdditionalBalance_DefaultDirection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@default-direction"

	allow := true

	cbi := &mmodel.CreateAdditionalBalance{
		Key:            "reserve",
		AllowSending:   &allow,
		AllowReceiving: &allow,
		// Direction intentionally nil
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "reserve").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.DefaultBalanceKey).
		Return(defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias), nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.Equal(t, constant.DirectionCredit, b.Direction,
				"missing direction MUST default to credit")
			return b, nil
		}).
		Times(1)

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, constant.DirectionCredit, result.Direction,
		"returned balance MUST default to credit")
}

// TestCreateAdditionalBalance_InvalidDirection verifies that supplying an
// unsupported direction returns a validation error before any persistence
// is attempted.
func TestCreateAdditionalBalance_InvalidDirection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	invalid := "sideways"
	allow := true

	cbi := &mmodel.CreateAdditionalBalance{
		Key:            "weird",
		AllowSending:   &allow,
		AllowReceiving: &allow,
		Direction:      &invalid,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	// Create MUST NOT be called when direction is invalid.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Times(0)

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.Error(t, err, "invalid direction MUST return an error")
	assert.Nil(t, result, "no balance should be returned on validation failure")
}

// TestCreateAdditionalBalance_ReservedKey verifies that the reserved
// "overdraft" key cannot be created through the public API — it is
// exclusively managed by the system when overdraft is enabled.
func TestCreateAdditionalBalance_ReservedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	allow := true

	cases := []struct {
		name string
		key  string
	}{
		{name: "lowercase", key: "overdraft"},
		{name: "uppercase", key: "OVERDRAFT"},
		{name: "mixed case", key: "Overdraft"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cbi := &mmodel.CreateAdditionalBalance{
				Key:            tc.key,
				AllowSending:   &allow,
				AllowReceiving: &allow,
			}

			mockBalanceRepo := balance.NewMockRepository(ctrl)

			// Create MUST NOT be invoked for the reserved key.
			mockBalanceRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				Times(0)

			uc := &UseCase{BalanceRepo: mockBalanceRepo}

			result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

			require.Error(t, err, "reserved key MUST be rejected")
			assert.Nil(t, result, "no balance should be returned when key is reserved")
		})
	}
}

// TestCreateAdditionalBalance_RejectsInternalScope verifies that clients
// cannot create balances with balanceScope="internal" through the public API.
func TestCreateAdditionalBalance_RejectsInternalScope(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
	}

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.Error(t, err, "internal scope MUST be rejected on client creation")
	assert.Nil(t, result, "no balance should be returned when scope is internal")
	assert.Contains(t, err.Error(), constant.ErrInvalidBalanceSettings.Error(),
		"error must reference the invalid-settings code")
}

// expectAdditionalBalancePrechecks wires the two lookups every successful
// CreateAdditionalBalance performs: the requested key (not found) and the
// default balance (found, supplies the inherited fields).
func expectAdditionalBalancePrechecks(mockBalanceRepo *balance.MockRepository, orgID, ledgerID, accountID uuid.UUID, key string, defaultBalance *mmodel.Balance) {
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, key).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.DefaultBalanceKey).
		Return(defaultBalance, nil).
		Times(1)
}

// TestCreateAdditionalBalance_AllowOverdraft_CreatesCompanionBeforeParent
// verifies the companion-first invariant on the POST path: when the request
// carries settings.allowOverdraft=true, the system-managed "overdraft"
// companion balance is created BEFORE the parent, with the expected shape,
// and the request emits balance.created (parent) followed by
// balance.config_changed{overdraft_enabled} (companion).
func TestCreateAdditionalBalance_AllowOverdraft_CreatesCompanionBeforeParent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@overdraft-holder"

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope:   mmodel.BalanceScopeTransactional,
			AllowOverdraft: true,
		},
	}

	defaultBalance := defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	expectAdditionalBalancePrechecks(mockBalanceRepo, orgID, ledgerID, accountID, "savings", defaultBalance)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	gomock.InOrder(
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, constant.OverdraftBalanceKey, b.Key,
					"companion MUST be created BEFORE the parent")
				assert.Equal(t, constant.DirectionDebit, b.Direction,
					"companion MUST have debit direction")
				require.NotNil(t, b.Settings, "companion MUST carry settings")
				assert.Equal(t, mmodel.BalanceScopeInternal, b.Settings.BalanceScope,
					"companion MUST be scoped as internal")
				assert.Equal(t, alias, b.Alias, "companion MUST inherit the parent alias")
				assert.Equal(t, defaultBalance.AssetCode, b.AssetCode,
					"companion MUST inherit the parent asset code")
				assert.Equal(t, accountID.String(), b.AccountID,
					"companion MUST belong to the same account")
				out := *b
				return &out, nil
			}).
			Times(1),
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, "savings", b.Key,
					"parent MUST be created after the companion")
				out := *b
				return &out, nil
			}).
			Times(1),
	)

	uc := &UseCase{BalanceRepo: mockBalanceRepo, Streaming: mockEmitter}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 2,
		"expected balance.created (parent) then balance.config_changed (companion)")

	assert.Equal(t, "balance.created", emitted[0].DefinitionKey)
	assert.Equal(t, result.ID, emitted[0].Subject)

	assert.Equal(t, "balance.config-changed", emitted[1].DefinitionKey)
	assert.NotEmpty(t, emitted[1].Subject)
	assert.NotEqual(t, result.ID, emitted[1].Subject,
		"config_changed subject MUST be the companion, not the parent")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(emitted[1].Payload, &payload))
	assert.Equal(t, "overdraft_enabled", payload["changeType"])
	assert.Equal(t, constant.OverdraftBalanceKey, payload["key"])
}

// TestCreateAdditionalBalance_AllowOverdraft_IdempotentCompanion verifies
// that when the companion already exists, only the parent is created and
// no overdraft_enabled event is emitted.
func TestCreateAdditionalBalance_AllowOverdraft_IdempotentCompanion(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@idempotent-holder"

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope:   mmodel.BalanceScopeTransactional,
			AllowOverdraft: true,
		},
	}

	defaultBalance := defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias)

	existingCompanion := &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          alias,
		Key:            constant.OverdraftBalanceKey,
		AssetCode:      "USD",
		Direction:      constant.DirectionDebit,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	expectAdditionalBalancePrechecks(mockBalanceRepo, orgID, ledgerID, accountID, "savings", defaultBalance)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
		Return(existingCompanion, nil).
		Times(1)

	// Only the parent is created — the companion already exists.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.Equal(t, "savings", b.Key)
			out := *b
			return &out, nil
		}).
		Times(1)

	uc := &UseCase{BalanceRepo: mockBalanceRepo, Streaming: mockEmitter}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "existing companion MUST NOT re-emit overdraft_enabled")
	assert.Equal(t, "balance.created", emitted[0].DefinitionKey)
}

// TestCreateAdditionalBalance_AllowOverdraft_CompanionRaceIsBenign verifies
// that a 23505 on the companion Create resolved by a follow-up Find is
// treated as idempotent success: the parent is still created and no
// overdraft_enabled event is emitted (the race winner emits it).
func TestCreateAdditionalBalance_AllowOverdraft_CompanionRaceIsBenign(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@race-holder"

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope:   mmodel.BalanceScopeTransactional,
			AllowOverdraft: true,
		},
	}

	defaultBalance := defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias)

	peerCompanion := &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          alias,
		Key:            constant.OverdraftBalanceKey,
		AssetCode:      "USD",
		Direction:      constant.DirectionDebit,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	expectAdditionalBalancePrechecks(mockBalanceRepo, orgID, ledgerID, accountID, "savings", defaultBalance)

	gomock.InOrder(
		// First companion lookup: peer request has not committed yet.
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
			Times(1),
		// Companion Create loses the race on the partial UNIQUE index.
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, constant.OverdraftBalanceKey, b.Key)
				return nil, &pgconn.PgError{Code: constant.UniqueViolationCode}
			}).
			Times(1),
		// Reload resolves the peer-created row: benign race.
		mockBalanceRepo.EXPECT().
			FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
			Return(peerCompanion, nil).
			Times(1),
		// Parent Create proceeds normally.
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, "savings", b.Key)
				out := *b
				return &out, nil
			}).
			Times(1),
	)

	uc := &UseCase{BalanceRepo: mockBalanceRepo, Streaming: mockEmitter}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err, "a benign 23505 race MUST NOT surface as an error")
	require.NotNil(t, result)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "race loser MUST NOT emit overdraft_enabled")
	assert.Equal(t, "balance.created", emitted[0].DefinitionKey)
}

// TestCreateAdditionalBalance_AllowOverdraft_CompanionFailureBlocksParent
// verifies the companion-first ordering guarantee: when the companion
// Create fails with a technical error, the request fails and the parent
// is NEVER created.
func TestCreateAdditionalBalance_AllowOverdraft_CompanionFailureBlocksParent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@failure-holder"

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope:   mmodel.BalanceScopeTransactional,
			AllowOverdraft: true,
		},
	}

	defaultBalance := defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	expectAdditionalBalancePrechecks(mockBalanceRepo, orgID, ledgerID, accountID, "savings", defaultBalance)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	createErr := errors.New("connection reset by peer")

	// Exactly ONE Create happens (the companion) and it fails. The parent
	// Create MUST NOT run — Times(1) makes a second call fail the test.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.Equal(t, constant.OverdraftBalanceKey, b.Key,
				"the only Create attempt MUST be the companion")
			return nil, createErr
		}).
		Times(1)

	uc := &UseCase{BalanceRepo: mockBalanceRepo, Streaming: mockEmitter}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.Error(t, err, "companion provisioning failure MUST fail the request")
	assert.ErrorIs(t, err, createErr, "the original Create error MUST be preserved")
	assert.Nil(t, result)
	assert.Empty(t, mockEmitter.Events(), "no events on a failed request")
}

// TestCreateAdditionalBalance_AllowOverdraft_CompanionLookupError_BlocksParent
// verifies the companion-first guarantee at the earliest failure point:
// when the pre-create existence lookup for the "overdraft" companion fails
// with a technical (infrastructure) error — not an EntityNotFoundError and
// not a 23505 unique violation — ensureOverdraftBalance propagates it, the
// request fails, and NEITHER the companion NOR the parent is created. This is
// exactly the case companion-first exists to guard: a swallowed lookup error
// would let the parent be created with AllowOverdraft=true and no companion.
func TestCreateAdditionalBalance_AllowOverdraft_CompanionLookupError_BlocksParent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@lookup-failure-holder"

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope:   mmodel.BalanceScopeTransactional,
			AllowOverdraft: true,
		},
	}

	defaultBalance := defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	expectAdditionalBalancePrechecks(mockBalanceRepo, orgID, ledgerID, accountID, "savings", defaultBalance)

	// Infrastructure failure on the companion existence lookup: a generic db
	// error that is neither EntityNotFoundError nor a 23505 unique violation,
	// so ensureOverdraftBalance takes the propagation branch.
	lookupErr := errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
		Return(nil, lookupErr).
		Times(1)

	// The flow aborts at the companion lookup: NO Create runs — neither the
	// companion nor the parent. Times(0) makes any Create attempt fail the test.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Times(0)

	uc := &UseCase{BalanceRepo: mockBalanceRepo, Streaming: mockEmitter}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.Error(t, err, "a technical companion-lookup failure MUST fail the request")
	assert.ErrorIs(t, err, lookupErr, "the original lookup error MUST be propagated")
	assert.Nil(t, result, "no balance should be returned when the companion lookup fails")
	assert.Empty(t, mockEmitter.Events(), "no events on a failed request")
}

// TestCreateAdditionalBalance_NoOverdraft_SkipsCompanion verifies that
// requests without settings, or with allowOverdraft=false, take the
// original single-Create path: no companion lookup, no companion Create,
// no extra event.
func TestCreateAdditionalBalance_NoOverdraft_SkipsCompanion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		settings *mmodel.BalanceSettings
	}{
		{name: "nil settings", settings: nil},
		{
			name: "allowOverdraft false",
			settings: &mmodel.BalanceSettings{
				BalanceScope:   mmodel.BalanceScopeTransactional,
				AllowOverdraft: false,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			ctx := context.Background()
			orgID := uuid.New()
			ledgerID := uuid.New()
			accountID := uuid.New()
			alias := "@no-overdraft"

			cbi := &mmodel.CreateAdditionalBalance{
				Key:      "savings",
				Settings: tc.settings,
			}

			defaultBalance := defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias)

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockEmitter := pkgStreaming.NewMockEmitter()

			expectAdditionalBalancePrechecks(mockBalanceRepo, orgID, ledgerID, accountID, "savings", defaultBalance)

			// The companion lookup MUST NOT run.
			mockBalanceRepo.EXPECT().
				FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
				Times(0)

			mockBalanceRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
					assert.Equal(t, "savings", b.Key)
					out := *b
					return &out, nil
				}).
				Times(1)

			uc := &UseCase{BalanceRepo: mockBalanceRepo, Streaming: mockEmitter}

			result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

			require.NoError(t, err)
			require.NotNil(t, result)

			emitted := mockEmitter.Events()
			require.Len(t, emitted, 1, "only balance.created is emitted without an overdraft transition")
			assert.Equal(t, "balance.created", emitted[0].DefinitionKey)
		})
	}
}

// TestCreateAdditionalBalance_AllowOverdraft_WithLimit verifies that a POST
// carrying overdraftLimitEnabled=true and a valid overdraftLimit still
// provisions the companion normally.
func TestCreateAdditionalBalance_AllowOverdraft_WithLimit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@limit-holder"

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("1000"),
		},
	}

	defaultBalance := defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockEmitter := pkgStreaming.NewMockEmitter()

	expectAdditionalBalancePrechecks(mockBalanceRepo, orgID, ledgerID, accountID, "savings", defaultBalance)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.OverdraftBalanceKey).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	gomock.InOrder(
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, constant.OverdraftBalanceKey, b.Key)
				out := *b
				return &out, nil
			}).
			Times(1),
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, "savings", b.Key)
				out := *b
				return &out, nil
			}).
			Times(1),
	)

	uc := &UseCase{BalanceRepo: mockBalanceRepo, Streaming: mockEmitter}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Settings)
	assert.True(t, result.Settings.OverdraftLimitEnabled)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 2, "companion provisioning with a limit still emits both events")
	assert.Equal(t, "balance.created", emitted[0].DefinitionKey)
	assert.Equal(t, "balance.config-changed", emitted[1].DefinitionKey)
}
