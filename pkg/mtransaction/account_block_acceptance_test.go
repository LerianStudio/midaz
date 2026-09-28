// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"context"
	"testing"

	libConstant "github.com/LerianStudio/lib-commons/v7/commons/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// =============================================================================
// ACCOUNT-BLOCK ACCEPTANCE SUITE — Go pre-validation half
// =============================================================================
// One named case per PRD acceptance criterion that is decided in Go, so a
// criterion can be traced to a test by name. The criteria decided by the atomic
// Lua script (consumption, single use, concurrency, the cancel exemption) are
// covered by the integration suite of the same name in the redis adapter — the
// script is the authority, this layer only fast-fails.
//
// The naming is deliberate: TestRF<nn>_ prefixes are the traceability handle.

// destinationResponses builds the validate payload of a one-leg DESTINATION
// transaction on the given alias, keyed the way buildBalanceOperations keys it.
// It is the receiving-side mirror of sourceResponses.
func destinationResponses(alias string) Responses {
	return Responses{
		Asset: grantAsset,
		From:  map[string]Amount{},
		To: map[string]Amount{
			"0#" + alias + "#" + pkgConstant.DefaultBalanceKey: {
				Asset:     grantAsset,
				Value:     decimal.NewFromInt(100),
				Operation: libConstant.CREDIT,
				Direction: pkgConstant.DirectionCredit,
			},
		},
	}
}

// legWithAmount builds one debit-direction leg carrying an explicit amount, so a
// multi-source batch can give each source a DIFFERENT debited value.
func legWithAmount(alias, balanceKey, indexPrefix, amount string) AccountBlockExceptionLeg {
	aliasKey := alias + "#" + balanceKey

	return AccountBlockExceptionLeg{
		Alias:       alias,
		BalanceKey:  balanceKey,
		EntryKey:    indexPrefix + aliasKey,
		Direction:   pkgConstant.DirectionDebit,
		Amount:      amount,
		InternalKey: "balance:{transactions}:org:ledger:" + aliasKey,
	}
}

// TestRF01_BlockRejectsSendingAndReceiving covers the two RF-01 criteria that
// bite in Go: a blocked account rejects a transaction that DEBITS it and one that
// CREDITS it. The block is a property of the account, so both sides answer to it
// and both answer with the same dedicated code — never a balance-permission one,
// whose message would send the caller looking at the wrong control.
func TestRF01_BlockRejectsSendingAndReceiving(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		validate Responses
	}{
		{name: "RF-01 a blocked account cannot be debited", validate: sourceResponses(grantedAlias, false)},
		{name: "RF-01 a blocked account cannot be credited", validate: destinationResponses(grantedAlias)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Both permission flags are OPEN, so the only barrier left standing
			// is the account block itself.
			balances := []*Balance{exceptionBalance(grantedAlias, true, true, true)}

			err := ValidateBalancesRules(context.Background(), Transaction{}, tt.validate, balances, nil)

			require.Error(t, err)
			assert.Equal(t, pkgConstant.ErrAccountBlocked.Error(), codeFromError(err))
		})
	}
}

// TestRF02_UnblockRestoresLegacyEvaluation covers RF-02: once the flag is off,
// the transaction is judged by the per-balance permissions ALONE — it passes when
// they allow and rejects with the LEGACY code when they deny. A leftover block
// check would show up here as the wrong code on the deny case.
func TestRF02_UnblockRestoresLegacyEvaluation(t *testing.T) {
	t.Parallel()

	t.Run("RF-02 an unblocked account with open permissions transacts", func(t *testing.T) {
		t.Parallel()

		balances := []*Balance{exceptionBalance(grantedAlias, false, true, true)}

		require.NoError(t, ValidateBalancesRules(context.Background(), Transaction{},
			sourceResponses(grantedAlias, false), balances, nil))
	})

	t.Run("RF-02 an unblocked account is judged by its permissions alone", func(t *testing.T) {
		t.Parallel()

		balances := []*Balance{exceptionBalance(grantedAlias, false, false, true)}

		err := ValidateBalancesRules(context.Background(), Transaction{},
			sourceResponses(grantedAlias, false), balances, nil)

		require.Error(t, err)
		assert.Equal(t, pkgConstant.ErrAccountStatusTransactionRestriction.Error(), codeFromError(err),
			"an unblocked account must reject with the legacy balance code, not the block code")
	})
}

// TestRF03_BalancePermissionsAreUntouchedByTheFeature covers RF-03: the
// send/receive control keeps its exact behavior and its exact code on an account
// the feature never blocked, in both directions and with no grant in play.
func TestRF03_BalancePermissionsAreUntouchedByTheFeature(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name           string
		allowSending   bool
		allowReceiving bool
		validate       Responses
	}{
		{
			name:           "RF-03 sending deny rejects exactly as before",
			allowSending:   false,
			allowReceiving: true,
			validate:       sourceResponses(grantedAlias, false),
		},
		{
			name:           "RF-03 receiving deny rejects exactly as before",
			allowSending:   true,
			allowReceiving: false,
			validate:       destinationResponses(grantedAlias),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balances := []*Balance{exceptionBalance(grantedAlias, false, tt.allowSending, tt.allowReceiving)}

			err := ValidateBalancesRules(context.Background(), Transaction{}, tt.validate, balances, nil)

			require.Error(t, err)
			assert.Equal(t, pkgConstant.ErrAccountStatusTransactionRestriction.Error(), codeFromError(err))
		})
	}
}

// TestRF04_DecisionMatrix walks the design's decision matrix row by row, in the
// order the document states it, for the rows Go decides. The two rows the atomic
// script owns alone — the cancel exemption and the consumption of the identifier
// — are asserted in the integration suite, because Go never sees either.
//
// Row 2 ("blocked + valid grant") passes HERE and is consumed THERE: the Go layer
// stops fast-failing, the script decides. That split is the point of the matrix.
func TestRF04_DecisionMatrix(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		blocked  bool
		allows   bool
		granted  bool
		wantCode string
	}{
		{
			name:     "RF-04 row 1 · blocked account, no grant · rejected",
			blocked:  true,
			allows:   true,
			wantCode: pkgConstant.ErrAccountBlocked.Error(),
		},
		{
			name:    "RF-04 row 2 · blocked account, valid grant · passes to the script",
			blocked: true,
			allows:  true,
			granted: true,
		},
		{
			name:    "RF-04 row 3 · open account, balance allows · legacy path, passes",
			blocked: false,
			allows:  true,
		},
		{
			name:     "RF-04 row 4 · open account, balance denies, no grant · rejected by the legacy rule",
			blocked:  false,
			allows:   false,
			wantCode: pkgConstant.ErrAccountStatusTransactionRestriction.Error(),
		},
		{
			name:    "RF-04 row 5 · open account, balance denies, valid grant · passes",
			blocked: false,
			allows:  false,
			granted: true,
		},
		{
			// Both barriers are down at once and neither is waived. The block is
			// reported, not the balance deny: the account-level verdict is the one
			// the caller has to act on, and a deny message would send them to a
			// control that is not what is stopping them.
			name:     "RF-04 · with both barriers closed the account block is what is reported",
			blocked:  true,
			allows:   false,
			wantCode: pkgConstant.ErrAccountBlocked.Error(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balances := []*Balance{exceptionBalance(grantedAlias, tt.blocked, tt.allows, true)}

			var binding *AccountBlockExceptionBinding
			if tt.granted {
				binding = bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey)
			}

			err := ValidateBalancesRules(context.Background(), Transaction{},
				sourceResponses(grantedAlias, false), balances, binding)

			if tt.wantCode == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Equal(t, tt.wantCode, codeFromError(err))
		})
	}
}

// TestRF4A_BlockIsReasonAgnostic covers RF-4A: the primitive carries no business
// semantics. Whatever else the balance is — whatever account type, accounting
// direction, balance key or permission posture it has — a blocked account rejects
// with the SAME code, because nothing but the flag feeds the decision.
//
// The matrix is the assertion: if any business attribute ever started steering
// the block, exactly one of these rows would answer differently.
func TestRF4A_BlockIsReasonAgnostic(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		accountType string
		direction   string
		allows      bool
	}{
		{name: "RF-4A deposit account, credit direction", accountType: "deposit", direction: pkgConstant.DirectionCredit, allows: true},
		{name: "RF-4A creditCard account, debit direction", accountType: "creditCard", direction: pkgConstant.DirectionDebit, allows: true},
		{name: "RF-4A marketplace account with its sending already denied", accountType: "marketplace", direction: pkgConstant.DirectionCredit, allows: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := exceptionBalance(grantedAlias, true, tt.allows, true)
			balance.AccountType = tt.accountType
			balance.Direction = tt.direction

			err := ValidateBalancesRules(context.Background(), Transaction{},
				sourceResponses(grantedAlias, false), []*Balance{balance}, nil)

			require.Error(t, err)
			assert.Equal(t, pkgConstant.ErrAccountBlocked.Error(), codeFromError(err),
				"the block decision must not depend on any business attribute of the account")
		})
	}
}

// TestRF06_ExceptionIsNarrow covers the narrowness criterion of RF-06 from the
// binding side: a presented identifier authorizes ONE debit leg, so a grant that
// cannot be tied to exactly one debit of this batch is refused outright — and
// refused WITHOUT consuming anything, because only the script consumes.
func TestRF06_ExceptionIsNarrow(t *testing.T) {
	t.Parallel()

	grant := &AccountBlockExceptionGrant{ID: uuid.New(), Alias: grantedAlias, Amount: "100"}

	t.Run("RF-06 a grant for an account this transaction does not debit is refused", func(t *testing.T) {
		t.Parallel()

		binding, err := ResolveAccountBlockExceptionBinding(grant, []AccountBlockExceptionLeg{
			debitLeg(ungrantedAlias, pkgConstant.DefaultBalanceKey, "0#"),
		})

		require.Error(t, err)
		assert.Nil(t, binding)
		assert.Equal(t, pkgConstant.ErrAccountBlockExceptionInvalid.Error(), codeFromError(err))
	})

	t.Run("RF-06 a grant that cannot name which debit it covers is refused", func(t *testing.T) {
		t.Parallel()

		binding, err := ResolveAccountBlockExceptionBinding(grant, []AccountBlockExceptionLeg{
			debitLeg(grantedAlias, pkgConstant.DefaultBalanceKey, "0#"),
			debitLeg(grantedAlias, "savings", "1#"),
		})

		require.Error(t, err)
		assert.Nil(t, binding)
		assert.Equal(t, pkgConstant.ErrAccountBlockExceptionInvalid.Error(), codeFromError(err))
	})
}

// TestD1_GrantIsMatchedAgainstTheDebitedOperation covers decision D1: in a
// MULTI-SOURCE transaction the value the grant is checked against is the amount
// debited from the GRANTED source, not the transaction total.
//
// This is the difference between a grant minted for one participant's share and a
// grant that happens to equal the sum of everyone's. Binding to the total would
// make every single-source grant unusable in a split transaction and would let a
// grant minted for the total release a debit it never authorized.
func TestD1_GrantIsMatchedAgainstTheDebitedOperation(t *testing.T) {
	t.Parallel()

	// The grant's own minted amount is deliberately unlike EVERY leg and unlike
	// the total. The binding must be sourced from the leg, so echoing the grant
	// back — the regression this case exists to catch — would surface as 999
	// rather than silently agreeing with the right answer.
	grant := &AccountBlockExceptionGrant{ID: uuid.New(), Alias: grantedAlias, Amount: "999"}

	// Two sources fund one transaction: 60 out of the granted account, 40 out of
	// another. The transaction total is 100 and matches neither leg.
	binding, err := ResolveAccountBlockExceptionBinding(grant, []AccountBlockExceptionLeg{
		legWithAmount(grantedAlias, pkgConstant.DefaultBalanceKey, "0#", "60"),
		legWithAmount(ungrantedAlias, pkgConstant.DefaultBalanceKey, "1#", "40"),
	})

	require.NoError(t, err)
	require.NotNil(t, binding)

	assert.Equal(t, "60", binding.Amount,
		"the value the script compares must be the debited value of the granted source")
	assert.True(t, binding.Authorizes(grantedAlias, pkgConstant.DefaultBalanceKey),
		"the granted source's balance is covered")
	assert.False(t, binding.Authorizes(ungrantedAlias, pkgConstant.DefaultBalanceKey),
		"the co-funding source keeps every barrier it had")
}
