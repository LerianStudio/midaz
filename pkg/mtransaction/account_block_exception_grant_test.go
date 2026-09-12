// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const (
	grantAliasFixture = "@fraud_account"
	otherAliasFixture = "@other_account"
)

// debitLegFixture builds one debit-direction leg of a batch. indexPrefix is what
// separates a system-derived overdraft companion (it inherits the primary leg's
// positional index) from a debit the caller submitted against an overdraft-keyed
// balance (which carries its own).
func debitLegFixture(alias, balanceKey, indexPrefix, amount string) mtransaction.AccountBlockExceptionLeg {
	aliasKey := alias + "#" + balanceKey

	return mtransaction.AccountBlockExceptionLeg{
		Alias:       alias,
		BalanceKey:  balanceKey,
		EntryKey:    indexPrefix + aliasKey,
		Direction:   constant.DirectionDebit,
		Amount:      amount,
		InternalKey: "balance:{transactions}:org:ledger:" + aliasKey,
	}
}

func creditLegFixture(alias, balanceKey, indexPrefix, amount string) mtransaction.AccountBlockExceptionLeg {
	leg := debitLegFixture(alias, balanceKey, indexPrefix, amount)
	leg.Direction = constant.DirectionCredit

	return leg
}

// TestResolveAccountBlockExceptionBinding_BindsThePrimaryDebit locks the happy
// bind: the authorized amount is read off the PRIMARY leg of the batch, not
// echoed back from the grant, so the script's later comparison is against the
// transaction rather than against itself.
func TestResolveAccountBlockExceptionBinding_BindsThePrimaryDebit(t *testing.T) {
	t.Parallel()

	// The grant's own amount is deliberately wrong: if the resolver echoed it, the
	// comparison downstream would always agree and this is the only place the
	// difference would show.
	grant := &mtransaction.AccountBlockExceptionGrant{
		ID: uuid.New(), Alias: grantAliasFixture, Amount: "999",
	}

	legs := []mtransaction.AccountBlockExceptionLeg{
		debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "150.5"),
		creditLegFixture(otherAliasFixture, constant.DefaultBalanceKey, "1#", "150.5"),
	}

	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(grant, legs)
	require.NoError(t, err)
	require.NotNil(t, binding)

	assert.Equal(t, grant.ID, binding.ID)
	assert.Equal(t, grantAliasFixture, binding.Alias)
	assert.Equal(t, "150.5", binding.Amount,
		"the authorized amount must be the primary leg's, not the grant's copy")
	assert.Equal(t, []string{"balance:{transactions}:org:ledger:" + grantAliasFixture + "#default"},
		binding.InternalKeys())
}

// TestResolveAccountBlockExceptionBinding_CoversDerivedOverdraftCompanions is the
// regression for the bug that made a valid grant unusable on every direct or
// revert that draws overdraft.
//
// The system appends a companion debit leg on the SAME account alias with the
// reserved "overdraft" balance key. Counted as a second debit it made the bind
// ambiguous and rejected the transaction outright; and because the companion
// belongs to the same (blocked) account, a bypass naming only the primary would
// be rejected on the companion instead. One logical debit therefore has to cover
// both balances.
func TestResolveAccountBlockExceptionBinding_CoversDerivedOverdraftCompanions(t *testing.T) {
	t.Parallel()

	grant := &mtransaction.AccountBlockExceptionGrant{
		ID: uuid.New(), Alias: grantAliasFixture, Amount: "1000",
	}

	legs := []mtransaction.AccountBlockExceptionLeg{
		debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "1000"),
		// The companion inherits the primary's "0#" index prefix and carries only
		// the overdrawn portion of the debit.
		debitLegFixture(grantAliasFixture, constant.OverdraftBalanceKey, "0#", "400"),
		creditLegFixture(otherAliasFixture, constant.DefaultBalanceKey, "1#", "1000"),
	}

	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(grant, legs)
	require.NoError(t, err, "a derived companion must not read as an ambiguous second debit")
	require.NotNil(t, binding)

	assert.Equal(t, "1000", binding.Amount,
		"the authorized amount stays the primary's full debit, never the companion's portion")

	assert.True(t, binding.Authorizes(grantAliasFixture, constant.DefaultBalanceKey),
		"the primary balance must be covered")
	assert.True(t, binding.Authorizes(grantAliasFixture, constant.OverdraftBalanceKey),
		"the derived companion must be covered, or the block guard rejects on it")
	assert.Len(t, binding.InternalKeys(), 2,
		"the script's bypass list must name both balances of the one logical debit")
}

// TestResolveAccountBlockExceptionBinding_RefusesAnAmbiguousBind proves the
// resolver never guesses which debit a grant covers. Rejecting BEFORE the script
// runs is what keeps the identifier alive: only the script consumes.
func TestResolveAccountBlockExceptionBinding_RefusesAnAmbiguousBind(t *testing.T) {
	t.Parallel()

	grant := &mtransaction.AccountBlockExceptionGrant{
		ID: uuid.New(), Alias: grantAliasFixture, Amount: "100",
	}

	for _, tt := range []struct {
		name string
		legs []mtransaction.AccountBlockExceptionLeg
	}{
		{
			name: "the granted alias is not debited at all",
			legs: []mtransaction.AccountBlockExceptionLeg{
				debitLegFixture(otherAliasFixture, constant.DefaultBalanceKey, "0#", "100"),
				creditLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "1#", "100"),
			},
		},
		{
			name: "the batch carries no legs",
			legs: nil,
		},
		{
			name: "the caller submitted two debits out of the granted account",
			legs: []mtransaction.AccountBlockExceptionLeg{
				debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "60"),
				debitLegFixture(grantAliasFixture, "asset-freeze", "1#", "40"),
			},
		},
		{
			name: "the caller submitted its own debit against an overdraft-keyed balance",
			legs: []mtransaction.AccountBlockExceptionLeg{
				debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "100"),
				// Its own positional index, so it is NOT a leg the system derived
				// from the primary and must still fail closed.
				debitLegFixture(grantAliasFixture, constant.OverdraftBalanceKey, "1#", "100"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binding, err := mtransaction.ResolveAccountBlockExceptionBinding(grant, tt.legs)

			require.Error(t, err)
			assert.Nil(t, binding)
			assert.Contains(t, err.Error(), constant.ErrAccountBlockExceptionInvalid.Error(),
				"an ambiguous bind must reject with the exception code, not the block code")
		})
	}
}

// TestAccountBlockExceptionBinding_AuthorizesByFullBalanceIdentity is the
// regression for the over-broad relief: the binding must be scoped to the exact
// balances it bound, never to the alias at large.
//
// Two balances of one account are two independent permission surfaces — their
// sending and receiving flags are set separately and the atomic script does not
// re-check them — so a grant minted for one segment's debit must not release an
// unrelated segment's restriction inside the same transaction.
func TestAccountBlockExceptionBinding_AuthorizesByFullBalanceIdentity(t *testing.T) {
	t.Parallel()

	grant := &mtransaction.AccountBlockExceptionGrant{
		ID: uuid.New(), Alias: grantAliasFixture, Amount: "100",
	}

	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(grant,
		[]mtransaction.AccountBlockExceptionLeg{
			debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "100"),
		})
	require.NoError(t, err)

	assert.True(t, binding.Authorizes(grantAliasFixture, constant.DefaultBalanceKey),
		"the bound balance is authorized")
	assert.False(t, binding.Authorizes(grantAliasFixture, "asset-freeze"),
		"a SIBLING balance of the same account must not be authorized")
	assert.False(t, binding.Authorizes(grantAliasFixture, constant.OverdraftBalanceKey),
		"an overdraft companion the batch never derived must not be authorized")
	assert.False(t, binding.Authorizes(otherAliasFixture, constant.DefaultBalanceKey),
		"another account must not be authorized")

	assert.True(t, binding.Authorizes(grantAliasFixture, ""),
		"an empty balance key reads as the default key, as everywhere else in the package")
}

// TestAccountBlockExceptionBinding_NilAuthorizesNothing covers the no-grant path:
// callers rely on the nil receiver so no barrier needs its own nil guard.
func TestAccountBlockExceptionBinding_NilAuthorizesNothing(t *testing.T) {
	t.Parallel()

	var absent *mtransaction.AccountBlockExceptionBinding

	assert.False(t, absent.Authorizes(grantAliasFixture, constant.DefaultBalanceKey))
	assert.False(t, absent.Authorizes("", ""))
	assert.Nil(t, absent.InternalKeys(), "the no-grant path must allocate nothing")
}

// TestResolveAccountBlockExceptionBinding_NoGrantIsNoBinding keeps the additive
// guarantee at the resolver: a request presenting nothing gets no binding and no
// error, so every barrier behaves as it did before the field existed.
func TestResolveAccountBlockExceptionBinding_NoGrantIsNoBinding(t *testing.T) {
	t.Parallel()

	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(nil,
		[]mtransaction.AccountBlockExceptionLeg{
			debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "100"),
		})

	require.NoError(t, err)
	assert.Nil(t, binding)
}
