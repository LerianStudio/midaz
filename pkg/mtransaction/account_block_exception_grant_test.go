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
	return mtransaction.AccountBlockExceptionLeg{
		Alias:       alias,
		BalanceKey:  balanceKey,
		EntryKey:    indexPrefix + alias + "#" + balanceKey,
		Direction:   constant.DirectionDebit,
		Amount:      amount,
		InternalKey: internalKeyFixture(alias, balanceKey),
	}
}

// internalKeyFixture is the unprefixed balance internal key debitLegFixture
// assigns to a leg, so a test can name the exact bypass list it expects.
func internalKeyFixture(alias, balanceKey string) string {
	return "balance:{transactions}:org:ledger:" + alias + "#" + balanceKey
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
	assert.Equal(t, []string{internalKeyFixture(grantAliasFixture, constant.DefaultBalanceKey)},
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

	assert.Equal(t, []string{
		internalKeyFixture(grantAliasFixture, constant.DefaultBalanceKey),
		internalKeyFixture(grantAliasFixture, constant.OverdraftBalanceKey),
	}, binding.InternalKeys(),
		"the script's bypass list must name the primary and its derived companion, or the block guard rejects on the companion")
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

// TestAccountBlockExceptionBinding_CoversOnlyTheBoundBalances is the regression
// for the over-broad relief: the binding must be scoped to the exact balances it
// bound, never to the alias at large.
//
// Two balances of one account are two independent permission surfaces, so a
// grant minted for one segment's debit must not release an unrelated segment's
// restriction inside the same transaction. The batch below touches a sibling
// balance of the granted account and a balance of a co-funding account; the
// script's bypass list must name neither.
func TestAccountBlockExceptionBinding_CoversOnlyTheBoundBalances(t *testing.T) {
	t.Parallel()

	grant := &mtransaction.AccountBlockExceptionGrant{
		ID: uuid.New(), Alias: grantAliasFixture, Amount: "100",
	}

	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(grant,
		[]mtransaction.AccountBlockExceptionLeg{
			debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "100"),
			creditLegFixture(grantAliasFixture, "asset-freeze", "1#", "100"),
			debitLegFixture(otherAliasFixture, constant.DefaultBalanceKey, "2#", "50"),
		})
	require.NoError(t, err)

	assert.Equal(t, []string{internalKeyFixture(grantAliasFixture, constant.DefaultBalanceKey)},
		binding.InternalKeys(),
		"only the bound balance may be bypassed: not a sibling balance of the same account, not another account")
}

// TestAccountBlockExceptionBinding_MatchesTheGrantedSourceInAMultiSourceBatch
// covers a MULTI-SOURCE transaction: the value the grant is checked against is
// the amount debited from the GRANTED source, not the transaction total.
//
// Binding to the total would make every single-source grant unusable in a split
// transaction and would let a grant minted for the total release a debit it
// never authorized.
func TestAccountBlockExceptionBinding_MatchesTheGrantedSourceInAMultiSourceBatch(t *testing.T) {
	t.Parallel()

	// The grant's own minted amount is deliberately unlike every leg and unlike
	// the total, so echoing it back would surface as 999.
	grant := &mtransaction.AccountBlockExceptionGrant{
		ID: uuid.New(), Alias: grantAliasFixture, Amount: "999",
	}

	// Two sources fund one transaction: 60 out of the granted account, 40 out of
	// another. The transaction total is 100 and matches neither leg.
	binding, err := mtransaction.ResolveAccountBlockExceptionBinding(grant,
		[]mtransaction.AccountBlockExceptionLeg{
			debitLegFixture(grantAliasFixture, constant.DefaultBalanceKey, "0#", "60"),
			debitLegFixture(otherAliasFixture, constant.DefaultBalanceKey, "1#", "40"),
		})
	require.NoError(t, err)
	require.NotNil(t, binding)

	assert.Equal(t, "60", binding.Amount,
		"the value the script compares must be the debited value of the granted source")
	assert.Equal(t, []string{internalKeyFixture(grantAliasFixture, constant.DefaultBalanceKey)},
		binding.InternalKeys(),
		"the co-funding source keeps every barrier it had")
}

// TestAccountBlockExceptionBinding_NilCoversNothing covers the no-grant path:
// callers rely on the nil receiver, so the no-grant path needs no guard of its own.
func TestAccountBlockExceptionBinding_NilCoversNothing(t *testing.T) {
	t.Parallel()

	var absent *mtransaction.AccountBlockExceptionBinding

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
