// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"context"
	"testing"

	libConstant "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// These tests cover the two barriers a single-use account-block exception is
// allowed to relax in the Go pre-validation — the account block and the
// per-balance sending/receiving deny — and, just as importantly, everything it is
// NOT allowed to relax. Consumption is never asserted here because it never
// happens here: the Go layer only reads the grant, the atomic script consumes it.

const (
	grantedAlias   = "@granted_account"
	ungrantedAlias = "@ungranted_account"
	grantAsset     = "USD"
)

// bindingFor resolves a real binding for a one-leg debit on the given
// alias#balanceKey, THROUGH THE PRODUCTION RESOLVER rather than by hand-filling
// the struct — so the relief these tests exercise is scoped exactly the way the
// pipeline scopes it, and a change to the bind rule shows up here.
func bindingFor(t *testing.T, alias, balanceKey string) *AccountBlockExceptionBinding {
	t.Helper()

	grant := &AccountBlockExceptionGrant{ID: uuid.New(), Alias: alias, Amount: "100"}

	binding, err := ResolveAccountBlockExceptionBinding(grant, []AccountBlockExceptionLeg{
		debitLeg(alias, balanceKey, "0#"),
	})
	require.NoError(t, err)
	require.NotNil(t, binding)

	return binding
}

// debitLeg builds one debit-direction leg of a batch, with the positional index
// prefix the caller wants on its entry key.
func debitLeg(alias, balanceKey, indexPrefix string) AccountBlockExceptionLeg {
	aliasKey := alias + "#" + balanceKey

	return AccountBlockExceptionLeg{
		Alias:       alias,
		BalanceKey:  balanceKey,
		EntryKey:    indexPrefix + aliasKey,
		Direction:   pkgConstant.DirectionDebit,
		Amount:      "100",
		InternalKey: "balance:{transactions}:org:ledger:" + aliasKey,
	}
}

// exceptionBalance builds one source balance with the two barrier flags under test
// dialed independently, so a case can isolate the block from the sending deny.
func exceptionBalance(alias string, blocked, allowSending, allowReceiving bool) *Balance {
	return &Balance{
		ID:             uuid.New().String(),
		Alias:          alias,
		Key:            pkgConstant.DefaultBalanceKey,
		AssetCode:      grantAsset,
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   allowSending,
		AllowReceiving: allowReceiving,
		Blocked:        blocked,
	}
}

// sourceResponses builds the validate payload of a one-leg source transaction on
// the given alias, keyed the way buildBalanceOperations keys it.
func sourceResponses(alias string, pending bool) Responses {
	return Responses{
		Asset:   grantAsset,
		Pending: pending,
		From: map[string]Amount{
			"0#" + alias + "#" + pkgConstant.DefaultBalanceKey: {
				Asset:     grantAsset,
				Value:     decimal.NewFromInt(100),
				Operation: libConstant.DEBIT,
				Direction: pkgConstant.DirectionDebit,
			},
		},
		To: map[string]Amount{},
	}
}

// TestValidateBalancesRules_GrantRelaxesTheBlockAndTheDeny walks the barrier
// matrix: a grant minted for the balance releases the account block and the
// sending deny, and a grant minted for a DIFFERENT alias releases neither.
//
// The negative half is the load-bearing one. A grant that released barriers on
// every balance in the batch would turn one authorized debit into a blanket
// bypass for the whole transaction.
func TestValidateBalancesRules_GrantRelaxesTheBlockAndTheDeny(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		blocked  bool
		sending  bool
		binding  func(*testing.T) *AccountBlockExceptionBinding
		wantCode string
	}{
		{
			name:     "blocked, no grant, rejects with the block code",
			blocked:  true,
			sending:  true,
			binding:  nil,
			wantCode: pkgConstant.ErrAccountBlocked.Error(),
		},
		{
			name:    "blocked, matching grant, passes",
			blocked: true,
			sending: true,
			binding: func(t *testing.T) *AccountBlockExceptionBinding {
				return bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey)
			},
		},
		{
			name:    "blocked, grant for another alias, still rejects",
			blocked: true,
			sending: true,
			binding: func(t *testing.T) *AccountBlockExceptionBinding {
				return bindingFor(t, ungrantedAlias, pkgConstant.DefaultBalanceKey)
			},
			wantCode: pkgConstant.ErrAccountBlocked.Error(),
		},
		{
			name:     "sending denied, no grant, rejects with the legacy code",
			blocked:  false,
			sending:  false,
			binding:  nil,
			wantCode: pkgConstant.ErrAccountStatusTransactionRestriction.Error(),
		},
		{
			name:    "sending denied, matching grant, passes",
			blocked: false,
			sending: false,
			binding: func(t *testing.T) *AccountBlockExceptionBinding {
				return bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey)
			},
		},
		{
			name:    "sending denied, grant for another alias, still rejects",
			blocked: false,
			sending: false,
			binding: func(t *testing.T) *AccountBlockExceptionBinding {
				return bindingFor(t, ungrantedAlias, pkgConstant.DefaultBalanceKey)
			},
			wantCode: pkgConstant.ErrAccountStatusTransactionRestriction.Error(),
		},
		{
			name:    "blocked AND sending denied, matching grant, passes both",
			blocked: true,
			sending: false,
			binding: func(t *testing.T) *AccountBlockExceptionBinding {
				return bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balances := []*Balance{exceptionBalance(grantedAlias, tt.blocked, tt.sending, true)}

			var binding *AccountBlockExceptionBinding
			if tt.binding != nil {
				binding = tt.binding(t)
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

// TestValidateBalancesRules_GrantRelaxesTheReceivingDeny covers the destination
// half of the deny relief. The grant is bound to a source account, so this only
// bites when the granted alias also appears as a destination — but the relief has
// to be per-balance, not per-side, or the two sides would need two rules.
func TestValidateBalancesRules_GrantRelaxesTheReceivingDeny(t *testing.T) {
	t.Parallel()

	validate := Responses{
		Asset: grantAsset,
		From:  map[string]Amount{},
		To: map[string]Amount{
			"0#" + grantedAlias + "#" + pkgConstant.DefaultBalanceKey: {
				Asset:     grantAsset,
				Value:     decimal.NewFromInt(100),
				Operation: libConstant.CREDIT,
				Direction: pkgConstant.DirectionCredit,
			},
		},
	}

	balances := []*Balance{exceptionBalance(grantedAlias, false, true, false)}

	err := ValidateBalancesRules(context.Background(), Transaction{}, validate, balances, nil)
	require.Error(t, err, "a receiving deny must still reject without a grant")
	assert.Equal(t, pkgConstant.ErrAccountStatusTransactionRestriction.Error(), codeFromError(err))

	require.NoError(t,
		ValidateBalancesRules(context.Background(), Transaction{}, validate, balances, bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey)),
		"a grant minted for the balance must release its receiving deny too")
}

// TestValidateBalancesRules_GrantOnlyReleasesTheGrantedBalance proves the relief
// is scoped to ONE balance inside a multi-balance batch: a grant for the source
// does not let a blocked destination through.
//
// This is the property that keeps the block bidirectional. A grant authorizes a
// debit out of one account; it says nothing about who may receive.
func TestValidateBalancesRules_GrantOnlyReleasesTheGrantedBalance(t *testing.T) {
	t.Parallel()

	validate := sourceResponses(grantedAlias, false)
	validate.To = map[string]Amount{
		"1#" + ungrantedAlias + "#" + pkgConstant.DefaultBalanceKey: {
			Asset:     grantAsset,
			Value:     decimal.NewFromInt(100),
			Operation: libConstant.CREDIT,
			Direction: pkgConstant.DirectionCredit,
		},
	}

	balances := []*Balance{
		exceptionBalance(grantedAlias, true, true, true),
		exceptionBalance(ungrantedAlias, true, true, true),
	}

	err := ValidateBalancesRules(context.Background(), Transaction{}, validate, balances,
		bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey))

	require.Error(t, err, "a grant for the source must not release a blocked destination")
	assert.Equal(t, pkgConstant.ErrAccountBlocked.Error(), codeFromError(err))
}

// TestValidateBalancesRules_GrantRelaxesNothingStructural is the negative half of
// RF-06: a grant reaches the block and the balance deny and nothing else, so every
// structural barrier rejects with its CURRENT code even with a matching grant
// presented on an otherwise-blocked account.
//
// Asserting the code, not just the error, is what makes this bite: a grant that
// short-circuited the whole per-balance pass would still fail these cases, but
// with the block code instead of the structural one.
func TestValidateBalancesRules_GrantRelaxesNothingStructural(t *testing.T) {
	t.Parallel()

	t.Run("asset mismatch still rejects", func(t *testing.T) {
		t.Parallel()

		balance := exceptionBalance(grantedAlias, true, true, true)
		balance.AssetCode = "EUR"

		err := ValidateBalancesRules(context.Background(), Transaction{},
			sourceResponses(grantedAlias, false), []*Balance{balance}, bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey))

		require.Error(t, err)
		assert.Equal(t, pkgConstant.ErrAssetCodeNotFound.Error(), codeFromError(err),
			"a grant must not relax the asset match")
	})

	t.Run("external account on a pending source still rejects", func(t *testing.T) {
		t.Parallel()

		balance := exceptionBalance(grantedAlias, true, true, true)
		balance.AccountType = libConstant.ExternalAccountType

		err := ValidateBalancesRules(context.Background(), Transaction{},
			sourceResponses(grantedAlias, true), []*Balance{balance}, bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey))

		require.Error(t, err)
		assert.Equal(t, pkgConstant.ErrOnHoldExternalAccount.Error(), codeFromError(err),
			"a grant must not relax the pending/external rule")
	})

	t.Run("balance count mismatch still rejects", func(t *testing.T) {
		t.Parallel()

		err := ValidateBalancesRules(context.Background(), Transaction{},
			sourceResponses(grantedAlias, false), nil, bindingFor(t, grantedAlias, pkgConstant.DefaultBalanceKey))

		require.Error(t, err)
		assert.Equal(t, pkgConstant.ErrAccountIneligibility.Error(), codeFromError(err),
			"a grant must not relax the eligibility count check")
	})
}

// TestValidateBalancesRules_NoGrantIsByteForByteLegacy is the RF-03 guard: with no
// grant presented, an unblocked balance that passes its denies produces exactly
// the outcome it did before the field existed.
func TestValidateBalancesRules_NoGrantIsByteForByteLegacy(t *testing.T) {
	t.Parallel()

	balances := []*Balance{exceptionBalance(grantedAlias, false, true, true)}

	require.NoError(t, ValidateBalancesRules(context.Background(), Transaction{},
		sourceResponses(grantedAlias, false), balances, nil))
}
