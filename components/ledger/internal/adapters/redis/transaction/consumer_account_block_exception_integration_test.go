//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// ACCOUNT-BLOCK EXCEPTION CONSUMPTION INTEGRATION TESTS
// =============================================================================
// These tests exercise the single-use grant against the real engine, which is
// the only place the properties that matter are observable:
//
//   - a valid grant bypasses the account block for the balance it authorizes,
//   - the identifier is DELETED inside the same atomic step that moved the
//     balances, so a replay finds nothing,
//   - a grant that does not match the transaction rejects with 0508 and is NOT
//     consumed,
//   - an aborted batch does NOT burn the identifier, and
//   - concurrent transactions presenting the same identifier resolve to exactly
//     one winner, which only EVAL's atomicity can guarantee.
//
// At the Lua contract level a revert is indistinguishable from a direct create
// (isPending=false, APPROVED), so the direct-shaped cases cover it too.

// exceptionOp builds a balance operation carrying both fields the grant bind
// reads: the balance's plain alias and the operation's accounting DIRECTION. The
// direction, not the operation label, is what identifies the debited leg — a
// route-validated commit debits with ON_HOLD, a direct with DEBIT, and both are
// direction=debit.
func exceptionOp(
	orgID, ledgerID uuid.UUID,
	alias string,
	available decimal.Decimal,
	blocked bool,
	operation, direction string,
	amount decimal.Decimal,
) mmodel.BalanceOperation {
	return exceptionOpOnKey(orgID, ledgerID, alias, constant.DefaultBalanceKey, "credit", nil,
		available, blocked, operation, direction, amount, "0#")
}

// exceptionOpOnKey is exceptionOp with the fields the overdraft cases need: the
// balance's own key, its accounting direction, its overdraft settings, and the
// positional index prefix on the entry key — the prefix a system-derived
// companion inherits from the leg it was derived from.
func exceptionOpOnKey(
	orgID, ledgerID uuid.UUID,
	alias, balanceKeyName, balanceDirection string,
	settings *mmodel.BalanceSettings,
	available decimal.Decimal,
	blocked bool,
	operation, direction string,
	amount decimal.Decimal,
	indexPrefix string,
) mmodel.BalanceOperation {
	balanceKey := alias + "#" + balanceKeyName

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.New().String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Alias:          alias,
			Key:            balanceKeyName,
			AssetCode:      "USD",
			Available:      available,
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			Blocked:        blocked,
			Direction:      balanceDirection,
			OverdraftUsed:  decimal.Zero,
			Settings:       settings,
			CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Alias: indexPrefix + balanceKey,
		Amount: mtransaction.Amount{
			Asset:     "USD",
			Value:     amount,
			Operation: operation,
			Direction: direction,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, balanceKey),
	}
}

// mintException seeds one grant through the production writer, so these tests
// consume exactly the bytes and key shape the create route produces.
func mintException(t *testing.T, infra *integrationTestInfra, orgID, ledgerID uuid.UUID, alias, amount string) uuid.UUID {
	t.Helper()

	exceptionID := uuid.New()

	require.NoError(t, infra.repo.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID,
		[]AccountBlockException{{ID: exceptionID, Alias: alias, Amount: amount, TTL: 300 * time.Second}}))

	requireExceptionExists(t, infra, orgID, ledgerID, exceptionID)

	return exceptionID
}

// bindTo resolves a presented grant against the batch, through the SAME resolver
// the balance step runs, so every case below feeds the script exactly what
// production feeds it — including which balances the bypass covers.
func bindTo(t *testing.T, grant *mtransaction.AccountBlockExceptionGrant, ops []mmodel.BalanceOperation) *mtransaction.AccountBlockExceptionBinding {
	t.Helper()

	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(grant, exceptionLegs(ops))
	require.NoError(t, err, "the grant must bind to a debit of this batch")
	require.NotNil(t, binding)

	return binding
}

// exceptionLegs projects the batch the way the balance step projects it.
func exceptionLegs(ops []mmodel.BalanceOperation) []mtransaction.AccountBlockExceptionLeg {
	legs := make([]mtransaction.AccountBlockExceptionLeg, 0, len(ops))

	for _, op := range ops {
		if op.Balance == nil {
			continue
		}

		legs = append(legs, mtransaction.AccountBlockExceptionLeg{
			Alias:       op.Balance.Alias,
			BalanceKey:  op.Balance.Key,
			EntryKey:    op.Alias,
			Direction:   op.Amount.Direction,
			Amount:      op.Amount.Value.String(),
			InternalKey: op.InternalKey,
		})
	}

	return legs
}

func requireExceptionExists(t *testing.T, infra *integrationTestInfra, orgID, ledgerID, exceptionID uuid.UUID) {
	t.Helper()

	exists, err := infra.redisContainer.Client.Exists(context.Background(),
		utils.AccountBlockExceptionInternalKey(orgID, ledgerID, exceptionID)).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, exists, "the exception must still be in the cache")
}

func requireExceptionConsumed(t *testing.T, infra *integrationTestInfra, orgID, ledgerID, exceptionID uuid.UUID) {
	t.Helper()

	exists, err := infra.redisContainer.Client.Exists(context.Background(),
		utils.AccountBlockExceptionInternalKey(orgID, ledgerID, exceptionID)).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 0, exists, "a consumed exception must be gone from the cache")
}

func requireExceptionInvalidErr(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err, "an unusable grant must reject the batch")
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountBlockExceptionInvalid.Error()),
		"error should contain 0508, got: %v", err)
}

// TestIntegration_AccountBlockException_ValidGrantBypassesTheBlockAndIsConsumed is
// the MED scenario end to end: a blocked source that would reject with 0502 posts
// its debit when the authorized identifier is presented, and the identifier is
// gone afterwards.
func TestIntegration_AccountBlockException_ValidGrantBypassesTheBlockAndIsConsumed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-valid-src"

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "100")

	op := exceptionOp(orgID, ledgerID, alias, decimal.NewFromInt(500), true,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100))

	// Same batch, same blocked account, WITHOUT the grant: the baseline the grant
	// has to overcome, asserted here so the positive case below cannot pass by the
	// account simply not being blocked.
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	requireAccountBlockedErr(t, err)
	requireExceptionExists(t, infra, orgID, ledgerID, exceptionID)

	binding := bindTo(t, &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "100"},
		[]mmodel.BalanceOperation{op})

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, binding)
	require.NoError(t, err, "a valid grant must bypass the account block")
	require.NotNil(t, result)
	require.Len(t, result.After, 1)
	assert.Equal(t, "400", result.After[0].Available.String(),
		"the debit must actually post, not merely be permitted")

	requireExceptionConsumed(t, infra, orgID, ledgerID, exceptionID)
}

// TestIntegration_AccountBlockException_ReplayIsRejected is the single-use
// guarantee in its simplest form: the second presentation of a consumed
// identifier finds no key and rejects, so the same authorization cannot move
// money twice.
func TestIntegration_AccountBlockException_ReplayIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-replay-src"

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "100")
	grant := &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "100"}

	first := exceptionOp(orgID, ledgerID, alias, decimal.NewFromInt(500), true,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{first},
		bindTo(t, grant, []mmodel.BalanceOperation{first}))
	require.NoError(t, err)

	second := exceptionOp(orgID, ledgerID, alias, decimal.NewFromInt(400), true,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{second},
		bindTo(t, grant, []mmodel.BalanceOperation{second}))
	requireExceptionInvalidErr(t, err)

	blob, err := infra.redisContainer.Client.Get(ctx, second.InternalKey).Result()
	require.NoError(t, err)
	assert.Contains(t, blob, `"Available":"400"`,
		"the replay must move nothing: the balance stays where the first debit left it")
}

// TestIntegration_AccountBlockException_AmountMismatchRejectsWithoutConsuming
// covers the mismatch only the script can see: the grant binds to a real debit
// leg of this batch, but the amount it was minted for is not the amount that leg
// debits. The batch must reject with 0508 AND leave the identifier in the cache —
// the operator's grant is still valid, it was simply presented on the wrong
// transaction, and burning it would force a second privileged mint.
func TestIntegration_AccountBlockException_AmountMismatchRejectsWithoutConsuming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-mismatch-amount"

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "100")

	// Debits 101 while the grant authorizes 100.
	op := exceptionOp(orgID, ledgerID, alias, decimal.NewFromInt(500), true,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(101))

	binding := bindTo(t, &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "100"},
		[]mmodel.BalanceOperation{op})

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, binding)
	requireExceptionInvalidErr(t, err)

	requireExceptionExists(t, infra, orgID, ledgerID, exceptionID)

	exists, err := infra.redisContainer.Client.Exists(ctx, op.InternalKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "a rejected batch must not create the balance blob")
}

// TestIntegration_AccountBlockException_UnbindableGrantNeverReachesTheScript
// covers the mismatches the BIND catches, before any Redis call: a grant minted
// for an account this transaction does not debit, and a caller-submitted second
// debit out of the granted account that leaves the grant unable to name which
// debit it covers.
//
// Both must be refused without the script running at all, which is what proves
// the identifier cannot have been consumed.
func TestIntegration_AccountBlockException_UnbindableGrantNeverReachesTheScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	for _, tt := range []struct {
		name  string
		alias string
		build func(orgID, ledgerID uuid.UUID) []mmodel.BalanceOperation
	}{
		{
			name:  "the grant names an account this transaction does not debit",
			alias: "@abe-unbindable-alias",
			build: func(orgID, ledgerID uuid.UUID) []mmodel.BalanceOperation {
				return []mmodel.BalanceOperation{
					exceptionOp(orgID, ledgerID, "@abe-somewhere-else", decimal.NewFromInt(500), true,
						constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100)),
				}
			},
		},
		{
			name:  "the caller submitted two debits out of the granted account",
			alias: "@abe-unbindable-double",
			build: func(orgID, ledgerID uuid.UUID) []mmodel.BalanceOperation {
				return []mmodel.BalanceOperation{
					exceptionOpOnKey(orgID, ledgerID, "@abe-unbindable-double", constant.DefaultBalanceKey,
						"credit", nil, decimal.NewFromInt(500), true,
						constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(60), "0#"),
					exceptionOpOnKey(orgID, ledgerID, "@abe-unbindable-double", "asset-freeze",
						"credit", nil, decimal.NewFromInt(500), true,
						constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(40), "1#"),
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			orgID, ledgerID := uuid.New(), uuid.New()

			exceptionID := mintException(t, infra, orgID, ledgerID, tt.alias, "100")
			ops := tt.build(orgID, ledgerID)

			binding, err := mtransaction.ResolveAccountBlockExceptionBinding(
				&mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: tt.alias, Amount: "100"},
				exceptionLegs(ops),
			)

			requireExceptionInvalidErr(t, err)
			assert.Nil(t, binding)

			requireExceptionExists(t, infra, orgID, ledgerID, exceptionID)

			for _, op := range ops {
				exists, existsErr := infra.redisContainer.Client.Exists(ctx, op.InternalKey).Result()
				require.NoError(t, existsErr)
				assert.Zero(t, exists, "the script never ran, so no balance blob may exist")
			}
		})
	}
}

// TestIntegration_AccountBlockException_UnknownIdentifierRejects covers the grant
// that is not in the cache at all — never minted, already used, or expired by its
// TTL. All three are one outcome, because expiry is Redis's job and the script
// compares no timestamps.
func TestIntegration_AccountBlockException_UnknownIdentifierRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-unknown-src"

	op := exceptionOp(orgID, ledgerID, alias, decimal.NewFromInt(500), true,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100))

	binding := bindTo(t, &mtransaction.AccountBlockExceptionGrant{ID: uuid.New(), Alias: alias, Amount: "100"},
		[]mmodel.BalanceOperation{op})

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, binding)
	requireExceptionInvalidErr(t, err)
}

// TestIntegration_AccountBlockException_ConsumedEvenWhenNoBypassWasNeeded is row
// five of the decision matrix: an UNBLOCKED account posts as it always would, and
// the identifier is still consumed.
//
// Leaving it alive would hand the caller a grant that outlived the request it was
// presented on — a live bypass nobody is tracking any more.
func TestIntegration_AccountBlockException_ConsumedEvenWhenNoBypassWasNeeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-no-bypass-src"

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "100")
	grant := &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "100"}

	op := exceptionOp(orgID, ledgerID, alias, decimal.NewFromInt(500), false,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op},
		bindTo(t, grant, []mmodel.BalanceOperation{op}))
	require.NoError(t, err)
	require.Len(t, result.After, 1)
	assert.Equal(t, "400", result.After[0].Available.String())

	requireExceptionConsumed(t, infra, orgID, ledgerID, exceptionID)
}

// TestIntegration_AccountBlockException_DoesNotReleaseABlockedDestination keeps the
// block bidirectional. A grant authorizes a debit out of ONE account; a blocked
// counterparty still rejects with 0502, and the identifier survives because the
// batch never reached the consume step.
func TestIntegration_AccountBlockException_DoesNotReleaseABlockedDestination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const (
		sourceAlias      = "@abe-bidir-src"
		destinationAlias = "@abe-bidir-dst"
	)

	exceptionID := mintException(t, infra, orgID, ledgerID, sourceAlias, "100")
	grant := &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: sourceAlias, Amount: "100"}

	source := exceptionOp(orgID, ledgerID, sourceAlias, decimal.NewFromInt(500), true,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100))
	destination := exceptionOp(orgID, ledgerID, destinationAlias, decimal.NewFromInt(0), true,
		constant.CREDIT, constant.DirectionCredit, decimal.NewFromInt(100))

	ops := []mmodel.BalanceOperation{source, destination}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops, bindTo(t, grant, ops))

	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountBlocked.Error()),
		"a blocked destination must reject with 0502, not with the exception code; got: %v", err)

	requireExceptionExists(t, infra, orgID, ledgerID, exceptionID)
}

// TestIntegration_AccountBlockException_AbortedBatchDoesNotBurnTheGrant is the
// reason the DEL sits at the END of the script rather than beside the validation.
//
// Redis has no rollback: a script that returns an error leaves its writes applied.
// A grant deleted up front would therefore be burned by a batch that moved no
// money at all — here, one rejected for insufficient funds. The identifier must
// survive so the caller can retry the transaction that failed for an unrelated
// reason.
func TestIntegration_AccountBlockException_AbortedBatchDoesNotBurnTheGrant(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-abort-src"

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "1000")
	grant := &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "1000"}

	// Authorized amount and debited amount agree, so the grant itself is valid;
	// the batch dies on funds instead.
	op := exceptionOp(orgID, ledgerID, alias, decimal.NewFromInt(10), true,
		constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(1000))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op},
		bindTo(t, grant, []mmodel.BalanceOperation{op}))

	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()),
		"the batch must fail on funds, proving the grant was accepted first; got: %v", err)

	requireExceptionExists(t, infra, orgID, ledgerID, exceptionID)
}

// TestIntegration_AccountBlockException_ConcurrentPresentationsHaveOneWinner is the
// property no unit test can show: N transactions presenting the SAME identifier at
// the same instant resolve to exactly one success.
//
// It holds because the read, the comparison and the DEL are one EVAL, which Redis
// runs to completion before the next: the losers find no key. A grant validated in
// Go and deleted in a second round trip would let several of these through.
func TestIntegration_AccountBlockException_ConcurrentPresentationsHaveOneWinner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const (
		alias      = "@abe-concurrent-src"
		contenders = 16
	)

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "100")
	grant := &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "100"}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		rejected  int
	)

	// Available covers every contender, so a loser can only be losing the race for
	// the identifier — never running out of funds.
	available := decimal.NewFromInt(100 * contenders)

	start := make(chan struct{})

	for i := 0; i < contenders; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			op := exceptionOp(orgID, ledgerID, alias, available, true,
				constant.DEBIT, constant.DirectionDebit, decimal.NewFromInt(100))
			binding := bindTo(t, grant, []mmodel.BalanceOperation{op})

			<-start

			_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
				uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, binding)

			mu.Lock()
			defer mu.Unlock()

			switch {
			case err == nil:
				successes++
			case strings.Contains(err.Error(), constant.ErrAccountBlockExceptionInvalid.Error()):
				rejected++
			default:
				t.Errorf("a loser must reject with 0508, got: %v", err)
			}
		}()
	}

	close(start)
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one transaction may consume the identifier")
	assert.Equal(t, contenders-1, rejected, "every other contender must be rejected as invalid")

	requireExceptionConsumed(t, infra, orgID, ledgerID, exceptionID)
}

// TestIntegration_AccountBlockException_CoversTheOverdraftCompanion is the
// end-to-end regression for the interaction between the grant and the overdraft
// split.
//
// A debit larger than Available on an overdraft-enabled balance makes the system
// append a companion debit leg on the SAME account, keyed "overdraft". Two things
// used to break: the companion read as a second debit and made the bind
// ambiguous, and — once that was fixed — the companion's blob carries the same
// account-level Blocked flag, so a bypass naming only the primary was rejected on
// the companion instead. Either way a valid grant was unusable on every
// transaction that draws overdraft.
//
// The transaction must post, and the identifier must be consumed EXACTLY ONCE
// even though the bypass covered two balances.
func TestIntegration_AccountBlockException_CoversTheOverdraftCompanion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-overdraft-src"

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "1000")
	grant := &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "1000"}

	settings := &mmodel.BalanceSettings{
		BalanceScope:   mmodel.BalanceScopeTransactional,
		AllowOverdraft: true,
	}

	// Debits 1000 against 600 available: 400 overdraws and rides the companion.
	primary := exceptionOpOnKey(orgID, ledgerID, alias, constant.DefaultBalanceKey, "credit", settings,
		decimal.NewFromInt(600), true, constant.DEBIT, constant.DirectionDebit,
		decimal.NewFromInt(1000), "0#")

	// The companion the enrichment appends: same account, reserved "overdraft"
	// key, direction=debit, carrying only the overdrawn portion, and inheriting
	// the primary's "0#" positional index prefix.
	companion := exceptionOpOnKey(orgID, ledgerID, alias, constant.OverdraftBalanceKey, "debit", nil,
		decimal.Zero, true, constant.DEBIT, constant.DirectionDebit,
		decimal.NewFromInt(400), "0#")

	ops := []mmodel.BalanceOperation{primary, companion}

	binding := bindTo(t, grant, ops)
	require.Len(t, binding.InternalKeys(), 2,
		"one logical debit must bind both the primary and its derived companion")

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops, binding)
	require.NoError(t, err,
		"a valid grant must bypass the block on the companion too, or overdrawing transactions can never use one")
	require.NotNil(t, result)
	require.NotEmpty(t, result.After)

	// Consumed exactly once: the script deletes the identifier a single time no
	// matter how many balances the bypass covered.
	requireExceptionConsumed(t, infra, orgID, ledgerID, exceptionID)

	replay := exceptionOpOnKey(orgID, ledgerID, alias, constant.DefaultBalanceKey, "credit", settings,
		decimal.NewFromInt(600), true, constant.DEBIT, constant.DirectionDebit,
		decimal.NewFromInt(1000), "0#")

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{replay},
		bindTo(t, grant, []mmodel.BalanceOperation{replay}))
	requireExceptionInvalidErr(t, err)
}

// TestIntegration_AccountBlockException_DoesNotReleaseASiblingBalance is the
// end-to-end regression for the over-broad relief: a grant bound to one balance's
// debit must not let a SIBLING balance of the same blocked account through.
//
// The sibling is not an overdraft companion the system derived — it is a second
// balance of the account — so the block guard must still reject on it. Without
// per-balance scoping the alias match alone would have released it.
func TestIntegration_AccountBlockException_DoesNotReleaseASiblingBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	const alias = "@abe-sibling-src"

	exceptionID := mintException(t, infra, orgID, ledgerID, alias, "100")
	grant := &mtransaction.AccountBlockExceptionGrant{ID: exceptionID, Alias: alias, Amount: "100"}

	debited := exceptionOpOnKey(orgID, ledgerID, alias, constant.DefaultBalanceKey, "credit", nil,
		decimal.NewFromInt(500), true, constant.DEBIT, constant.DirectionDebit,
		decimal.NewFromInt(100), "0#")

	// A sibling balance of the same blocked account, receiving rather than
	// debiting, so the bind stays unambiguous and the guard is what decides.
	sibling := exceptionOpOnKey(orgID, ledgerID, alias, "asset-freeze", "credit", nil,
		decimal.Zero, true, constant.CREDIT, constant.DirectionCredit,
		decimal.NewFromInt(100), "1#")

	ops := []mmodel.BalanceOperation{debited, sibling}

	binding := bindTo(t, grant, ops)
	require.Len(t, binding.InternalKeys(), 1,
		"the bind must cover only the debited balance, not every balance of the alias")
	assert.False(t, binding.Authorizes(alias, "asset-freeze"),
		"a sibling balance is a separate permission surface and must stay unauthorized")

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops, binding)

	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountBlocked.Error()),
		"the blocked sibling must reject with 0502, not be waved through; got: %v", err)

	requireExceptionExists(t, infra, orgID, ledgerID, exceptionID)
}
