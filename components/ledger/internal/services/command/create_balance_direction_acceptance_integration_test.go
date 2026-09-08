//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// acceptanceDirectionHarness wires the real account-type and balance
// repositories against real PostgreSQL for the default-balance-direction
// acceptance tests. The account_type table lives in the onboarding migration
// set and the balance table lives in the transaction migration set; those two
// sets share a single golang-migrate schema_migrations bookkeeping table and
// therefore cannot both be applied to one database. They are provisioned in two
// containers here — the balance table carries no foreign key to account_type or
// account, and the initial-balance seam only reads the type via AccountTypeRepo
// and writes the balance via BalanceRepo, so splitting the schemas is faithful
// to production behavior.
//
// Reusable by Tasks 2.2.3 (additional-balance) and 2.2.4 (edit-default): call
// setupAcceptanceDirectionHarness once per test, seed types via seedAccountType,
// and drive the same UseCase.
type acceptanceDirectionHarness struct {
	uc *UseCase

	// onboardingDB is the raw handle to the database holding the account_type
	// and account tables; seed types and accounts here.
	onboardingDB *sql.DB

	// balanceDB is the raw handle to the database holding the balance table;
	// used for direct read-back assertions.
	balanceDB *sql.DB

	orgID    uuid.UUID
	ledgerID uuid.UUID
}

// setupAcceptanceDirectionHarness spins two PostgreSQL containers (onboarding
// for account_type/account, transaction for balance), constructs the real
// AccountTypePostgreSQLRepository and BalancePostgreSQLRepository, wires them
// into a single *UseCase, and seeds one organization + ledger on both databases
// so the shared org/ledger IDs resolve on either side.
func setupAcceptanceDirectionHarness(t *testing.T) *acceptanceDirectionHarness {
	t.Helper()

	onboardingContainer := pgtestutil.SetupContainer(t)
	transactionContainer := pgtestutil.SetupContainer(t)

	accountTypeRepo := newAccountTypeRepoForHarness(t, onboardingContainer)
	balanceRepo := newBalanceRepoForHarness(t, transactionContainer)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Seed org + ledger on the onboarding side only: the account_type table
	// references them there. The balance table (transaction side) carries no
	// foreign keys, so it needs no organization/ledger rows.
	seedOrganizationAndLedger(t, onboardingContainer.DB, orgID, ledgerID)

	uc := &UseCase{
		AccountTypeRepo: accountTypeRepo,
		BalanceRepo:     balanceRepo,
	}

	return &acceptanceDirectionHarness{
		uc:           uc,
		onboardingDB: onboardingContainer.DB,
		balanceDB:    transactionContainer.DB,
		orgID:        orgID,
		ledgerID:     ledgerID,
	}
}

// newAccountTypeRepoForHarness applies the onboarding migrations and returns a
// real account-type repository bound to the container connection.
func newAccountTypeRepoForHarness(t *testing.T, container *pgtestutil.ContainerResult) *accounttype.AccountTypePostgreSQLRepository {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, container.Config.DBName, migrationsPath)

	return accounttype.NewAccountTypePostgreSQLRepository(conn)
}

// newBalanceRepoForHarness applies the transaction migrations and returns a real
// balance repository bound to the container connection.
func newBalanceRepoForHarness(t *testing.T, container *pgtestutil.ContainerResult) *balance.BalancePostgreSQLRepository {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, container.Config.DBName, migrationsPath)

	return balance.NewBalancePostgreSQLRepository(conn, false)
}

// seedOrganizationAndLedger inserts an organization and ledger with the given
// IDs directly, so both container databases share the same scope identifiers.
// The INSERTs are delegated to the shared fixtures
// (pgtestutil.CreateTestOrganizationWithID / CreateTestLedgerWithID) to avoid
// duplicating the org/ledger seed SQL and its "Test Org"/"Test Ledger" literals.
func seedOrganizationAndLedger(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID) {
	t.Helper()

	pgtestutil.CreateTestOrganizationWithID(t, db, orgID)
	pgtestutil.CreateTestLedgerWithID(t, db, ledgerID, orgID)
}

// seedAccountType inserts an account type with the given key and optional
// default direction on the onboarding database. Reusable by 2.2.3/2.2.4.
func (h *acceptanceDirectionHarness) seedAccountType(t *testing.T, key string, defaultDirection *string) {
	t.Helper()

	params := pgtestutil.AccountTypeParams{
		Name:             key + " type",
		Description:      "acceptance seed",
		KeyValue:         key,
		DefaultDirection: defaultDirection,
	}
	pgtestutil.CreateTestAccountType(t, h.onboardingDB, h.orgID, h.ledgerID, params)
}

// readBackBalanceDirection reads the persisted direction for a balance by ID
// directly from the balance database.
func (h *acceptanceDirectionHarness) readBackBalanceDirection(ctx context.Context, t *testing.T, balanceID string) string {
	t.Helper()

	require.NoError(t, ctx.Err())

	var direction string

	err := h.balanceDB.QueryRowContext(ctx, `SELECT direction FROM balance WHERE id = $1`, balanceID).Scan(&direction)
	require.NoError(t, err, "failed to read back balance direction")

	return direction
}

// countBalancesForAccount returns the number of balance rows persisted for the
// given account, used to assert that a rejected additional-balance request
// wrote nothing.
func (h *acceptanceDirectionHarness) countBalancesForAccount(ctx context.Context, t *testing.T, accountID uuid.UUID) int {
	t.Helper()

	require.NoError(t, ctx.Err())

	var count int

	err := h.balanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM balance WHERE account_id = $1`, accountID).Scan(&count)
	require.NoError(t, err, "failed to count balances for account")

	return count
}

// TestIntegration_AcceptanceInitialBalanceDirection exercises the initial
// balance direction resolution seam (CreateAccount ->
// resolveDefaultBalanceDirectionForType -> CreateDefaultBalance) end-to-end
// against real PostgreSQL, asserting both the returned balance's Direction and
// the direction persisted in the balance row.
//
// Rows mirror the acceptance table:
//   - A1: type default "debit" (non-external)        -> debit
//   - A2: type default "credit" (non-external)       -> credit
//   - A3: type with no configured default            -> credit (implied default)
//   - A4: external bypass (type not seeded)          -> debit, no error
//   - A7: unseeded type (FindByKey miss)             -> credit, no error
//
// A3 note: the resolver's terminal fallback (defaultBalanceDirection) supplies
// "credit" when the type carries no configured default. CreateDefaultBalance
// always persists the resolved value, so the balance direction is set by the
// resolver, never by the balance column's own default.
func TestIntegration_AcceptanceInitialBalanceDirection(t *testing.T) {
	h := setupAcceptanceDirectionHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name              string
		typeKey           string
		seedType          bool
		seedDirection     *string
		isExternal        bool
		expectedDirection string
	}{
		{
			name:              "A1 type default debit -> debit",
			typeKey:           "loan",
			seedType:          true,
			seedDirection:     testutils.Ptr(constant.DirectionDebit),
			isExternal:        false,
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:              "A2 type default credit -> credit",
			typeKey:           "deposit",
			seedType:          true,
			seedDirection:     testutils.Ptr(constant.DirectionCredit),
			isExternal:        false,
			expectedDirection: constant.DirectionCredit,
		},
		{
			name:              "A3 type without configured default -> credit (implied default)",
			typeKey:           "savings",
			seedType:          true,
			seedDirection:     nil,
			isExternal:        false,
			expectedDirection: constant.DirectionCredit,
		},
		{
			name:              "A4 external bypass -> debit, no error",
			typeKey:           constant.ExternalAccountType,
			seedType:          false,
			seedDirection:     nil,
			isExternal:        true,
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:              "A7 unseeded type FindByKey miss -> credit, no error",
			typeKey:           "unregistered",
			seedType:          false,
			seedDirection:     nil,
			isExternal:        false,
			expectedDirection: constant.DirectionCredit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.seedType {
				h.seedAccountType(t, tt.typeKey, tt.seedDirection)
			}

			accountID := uuid.Must(libCommons.GenerateUUIDv7())

			// Reproduce the create_account seam: resolve the type default, then
			// create the default balance with the resolved direction.
			dir := h.uc.resolveDefaultBalanceDirectionForType(ctx, h.orgID, h.ledgerID, tt.typeKey, tt.isExternal)

			bal, err := h.uc.CreateDefaultBalance(ctx, mmodel.CreateBalanceInput{
				OrganizationID:   h.orgID,
				LedgerID:         h.ledgerID,
				AccountID:        accountID,
				Alias:            "@acceptance/" + tt.typeKey,
				AssetCode:        "USD",
				AccountType:      tt.typeKey,
				DefaultDirection: dir,
				AllowSending:     true,
				AllowReceiving:   true,
			})

			// A4/A7 assert graceful degradation: no error on external bypass or
			// on a type-lookup miss.
			require.NoError(t, err, "CreateDefaultBalance must not fail")
			require.NotNil(t, bal, "created balance must not be nil")

			assert.Equal(t, tt.expectedDirection, bal.Direction, "returned balance direction")

			persisted := h.readBackBalanceDirection(ctx, t, bal.ID)
			assert.Equal(t, tt.expectedDirection, persisted, "persisted balance direction")
		})
	}
}

// createDefaultBalanceForAccount reproduces the create_account initial-balance
// seam (resolveDefaultBalanceDirectionForType -> CreateDefaultBalance) for the
// given account and type key, returning the created balance. Reused by the
// additional-balance (A5) and edit-default (A6) acceptance tests: A5 needs a
// default balance to exist so CreateAdditionalBalance can read the account's
// AccountType from it; A6 uses it to materialize B1/B2 at distinct type-default
// snapshots.
func (h *acceptanceDirectionHarness) createDefaultBalanceForAccount(ctx context.Context, t *testing.T, accountID uuid.UUID, typeKey string) *mmodel.Balance {
	t.Helper()

	dir := h.uc.resolveDefaultBalanceDirectionForType(ctx, h.orgID, h.ledgerID, typeKey, false)

	bal, err := h.uc.CreateDefaultBalance(ctx, mmodel.CreateBalanceInput{
		OrganizationID:   h.orgID,
		LedgerID:         h.ledgerID,
		AccountID:        accountID,
		Alias:            "@acceptance/" + accountID.String(),
		AssetCode:        "USD",
		AccountType:      typeKey,
		DefaultDirection: dir,
		AllowSending:     true,
		AllowReceiving:   true,
	})
	require.NoError(t, err, "CreateDefaultBalance must not fail")
	require.NotNil(t, bal, "created default balance must not be nil")

	return bal
}

// TestIntegration_AcceptanceAdditionalBalanceOverride exercises the additional
// -balance direction seam (CreateAdditionalBalance) against real PostgreSQL.
// CreateAdditionalBalance loads the account's existing DEFAULT balance to read
// its AccountType, so each subtest seeds a default balance first, then adds an
// additional balance and asserts the resolved/persisted direction.
//
// Acceptance row A5:
//   - override wins:            explicit Direction=debit beats type default credit -> debit
//   - no override inherits type: Direction=nil inherits the type default          -> credit
func TestIntegration_AcceptanceAdditionalBalanceOverride(t *testing.T) {
	h := setupAcceptanceDirectionHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	const typeKey = "wallet"

	// Type default is "credit"; the additional-balance override/inherit paths
	// resolve against it.
	h.seedAccountType(t, typeKey, testutils.Ptr(constant.DirectionCredit))

	tests := []struct {
		name              string
		key               string
		direction         *string
		expectedDirection string
	}{
		{
			name:              "override wins: explicit debit beats type default credit",
			key:               "override-key",
			direction:         testutils.Ptr(constant.DirectionDebit),
			expectedDirection: constant.DirectionDebit,
		},
		{
			name:              "no override inherits type default credit",
			key:               "inherit-key",
			direction:         nil,
			expectedDirection: constant.DirectionCredit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accountID := uuid.Must(libCommons.GenerateUUIDv7())

			// The default balance must exist first: CreateAdditionalBalance reads
			// the account's AccountType from it (and rejects external types).
			h.createDefaultBalanceForAccount(ctx, t, accountID, typeKey)

			additional, err := h.uc.CreateAdditionalBalance(ctx, h.orgID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
				Key:       tt.key,
				Direction: tt.direction,
			})
			require.NoError(t, err, "CreateAdditionalBalance must not fail")
			require.NotNil(t, additional, "created additional balance must not be nil")

			assert.Equal(t, tt.expectedDirection, additional.Direction, "returned additional balance direction")

			persisted := h.readBackBalanceDirection(ctx, t, additional.ID)
			assert.Equal(t, tt.expectedDirection, persisted, "persisted additional balance direction")
		})
	}
}

// TestIntegration_AcceptanceAdditionalBalanceExternalRejected proves that
// CreateAdditionalBalance rejects an external account BEFORE any direction
// resolution: the guard (create_balance_additional.go) fires on the default
// balance's AccountType == constant.ExternalAccountType and returns the
// additional-balance-not-allowed business error, persisting no additional row.
//
// The external default balance is seeded directly (AccountType = external, Key =
// default) because the harness's default-balance helper resolves direction and is
// meant for non-external types; here the type never reaches the resolver.
func TestIntegration_AcceptanceAdditionalBalanceExternalRejected(t *testing.T) {
	h := setupAcceptanceDirectionHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// Seed the account's DEFAULT balance with the external account type. This is
	// the row CreateAdditionalBalance loads to read the AccountType from.
	externalDefaultParams := pgtestutil.DefaultBalanceParams()
	externalDefaultParams.Alias = "@acceptance/external/" + accountID.String()
	externalDefaultParams.Key = constant.DefaultBalanceKey
	externalDefaultParams.AccountType = constant.ExternalAccountType
	pgtestutil.CreateTestBalance(t, h.balanceDB, h.orgID, h.ledgerID, accountID, externalDefaultParams)

	require.Equal(t, 1, h.countBalancesForAccount(ctx, t, accountID), "only the seeded default balance must exist before the rejected request")

	additional, err := h.uc.CreateAdditionalBalance(ctx, h.orgID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key: "external-additional",
	})

	require.Error(t, err, "CreateAdditionalBalance must reject an external account type")
	require.Nil(t, additional, "no additional balance must be returned on rejection")

	// Assert the error IS the additional-balance-not-allowed business error: it
	// maps to constant.ErrAdditionalBalanceNotAllowed (code "0124") through
	// pkg.ValidateBusinessError -> UnprocessableOperationError.
	var unprocessable pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &unprocessable, "error must be an UnprocessableOperationError")
	assert.Equal(t, constant.ErrAdditionalBalanceNotAllowed.Error(), unprocessable.Code, "error code must be the additional-balance-not-allowed sentinel")

	// No additional row was persisted: still only the seeded default balance.
	assert.Equal(t, 1, h.countBalancesForAccount(ctx, t, accountID), "no additional balance row must be persisted after rejection")
}

// TestIntegration_AcceptanceEditDefaultAffectsFutureOnly proves that editing an
// account type's default direction affects only balances created AFTER the
// edit: direction is resolved at balance-creation time and persisted on the
// row, never re-derived from the type. Acceptance row A6.
//
// The account type is seeded via the fixture directly (not the harness helper)
// because this test needs the returned account-type ID to call
// AccountTypeRepo.Update.
func TestIntegration_AcceptanceEditDefaultAffectsFutureOnly(t *testing.T) {
	h := setupAcceptanceDirectionHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	const typeKey = "ledger-cash"

	// Seed the type with default "credit"; capture the ID for the later Update.
	accountTypeID := pgtestutil.CreateTestAccountType(t, h.onboardingDB, h.orgID, h.ledgerID, pgtestutil.AccountTypeParams{
		Name:             typeKey + " type",
		Description:      "acceptance seed",
		KeyValue:         typeKey,
		DefaultDirection: testutils.Ptr(constant.DirectionCredit),
	})

	// B1: created while the type default is credit.
	accountID1 := uuid.Must(libCommons.GenerateUUIDv7())
	b1 := h.createDefaultBalanceForAccount(ctx, t, accountID1, typeKey)
	assert.Equal(t, constant.DirectionCredit, b1.Direction, "B1 must inherit the type default credit at creation")

	// Flip the type default to debit.
	_, err := h.uc.AccountTypeRepo.Update(ctx, h.orgID, h.ledgerID, accountTypeID, &mmodel.AccountType{
		DefaultDirection: constant.DirectionDebit,
	})
	require.NoError(t, err, "AccountTypeRepo.Update must not fail")

	// B2: same type key, created AFTER the edit -> inherits the new debit default.
	accountID2 := uuid.Must(libCommons.GenerateUUIDv7())
	b2 := h.createDefaultBalanceForAccount(ctx, t, accountID2, typeKey)
	assert.Equal(t, constant.DirectionDebit, b2.Direction, "B2 must inherit the updated type default debit at creation")

	// B1 re-read: immutable — still credit despite the type edit.
	b1Persisted := h.readBackBalanceDirection(ctx, t, b1.ID)
	assert.Equal(t, constant.DirectionCredit, b1Persisted, "B1 must remain credit after the type default is edited")
}
