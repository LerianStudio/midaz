// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// AccountBlockExceptionGrant is a single-use account-block exception as the Go
// pre-validation reads it out of the cache. It carries the identifier the caller
// presented plus the two values the grant is bound to: the source account alias
// it authorizes and the exact amount of the debit it authorizes.
//
// It is the RAW cache read and authorizes nothing on its own. What it authorizes
// is decided by ResolveAccountBlockExceptionBinding, which ties it to the
// specific balances of one logical debit — see AccountBlockExceptionBinding.
type AccountBlockExceptionGrant struct {
	// ID is the identifier the transaction body presented.
	ID uuid.UUID
	// Alias is the source account alias the grant authorizes, as minted.
	Alias string
	// Amount is the canonical decimal string of the authorized debit, as minted.
	Amount string
}

// AccountBlockExceptionBinding is a presented grant tied to the ONE logical debit
// of a transaction it authorizes.
//
// "One logical debit" is a SET of balances, not a single one: the primary debit
// leg on the granted alias, plus the overdraft companion legs the system derived
// from that leg when the debit overdraws the balance. The companion is another
// balance of the SAME account, so a blocked account marks it blocked too — a
// binding that named only the primary would be rejected on the companion and the
// grant would be unusable on every transaction that draws overdraft.
//
// What the binding authorizes is scoped to those balances by their FULL identity
// (alias AND balance key), never by alias alone. Two balances of one account are
// two different permission surfaces: the per-balance sending/receiving flags are
// set independently, and the atomic script does not re-check them. Relieving them
// per alias would let a grant minted for one segment's debit release an unrelated
// segment's restriction inside the same transaction.
//
// The binding is derived ONCE, in the balance step, and the same value reaches
// both the Go pre-validation and the atomic script — so the balances Go stops
// fast-failing are exactly the balances the script bypasses the block on.
type AccountBlockExceptionBinding struct {
	// ID is the presented identifier, which the script validates and deletes.
	ID uuid.UUID
	// Alias is the granted source account alias.
	Alias string
	// Amount is the canonical decimal of the PRIMARY debited leg — the full
	// authorized debit. The overdraft companion carries only the overdrawn
	// portion of it and is never the value the grant is matched against.
	Amount string

	// authorized identifies the balances the binding covers by "alias#balanceKey".
	authorized map[string]struct{}
	// internalKeys are the unprefixed balance internal keys of the same set, in
	// the order they appear in the batch, for the script's bypass list.
	internalKeys []string
}

// Authorizes reports whether the binding covers the balance identified by its
// alias and balance key.
//
// A nil receiver means "no grant presented" and authorizes nothing, so callers
// need no separate nil guard. An empty balance key is read as the default key,
// matching how the rest of the package resolves a balance's identity.
func (b *AccountBlockExceptionBinding) Authorizes(alias, balanceKey string) bool {
	if b == nil {
		return false
	}

	_, ok := b.authorized[AliasKey(alias, balanceKey)]

	return ok
}

// InternalKeys returns the unprefixed balance internal keys the binding covers.
// A nil receiver returns nil, so the no-grant path allocates nothing.
func (b *AccountBlockExceptionBinding) InternalKeys() []string {
	if b == nil {
		return nil
	}

	return b.internalKeys
}

// AccountBlockExceptionLeg is what ResolveAccountBlockExceptionBinding needs to
// know about one balance operation of the batch.
//
// It is deliberately NOT mmodel.BalanceOperation. The model package imports this
// one (Balance.ToTransactionBalance), so this package cannot import the model
// back; the caller projects each operation onto this shape instead. The rule
// itself — what a grant authorizes — stays here, next to the validation that
// applies it, rather than being restated in the adapter.
type AccountBlockExceptionLeg struct {
	// Alias is the balance's plain account alias.
	Alias string
	// BalanceKey is the balance's own key ("default", "overdraft", a segment).
	BalanceKey string
	// EntryKey is the operation's composite entry key, whose positional index
	// prefix is what ties a derived companion leg to the primary it came from.
	EntryKey string
	// Direction is the operation's accounting direction.
	Direction string
	// Amount is the operation's canonical decimal amount.
	Amount string
	// InternalKey is the operation's unprefixed balance internal key.
	InternalKey string
}

// ResolveAccountBlockExceptionBinding ties a presented grant to the one logical
// debit of the batch it authorizes, or refuses to guess.
//
// The PRIMARY leg is the batch's debit-direction operation on the granted alias
// that is not a system-derived overdraft companion. There must be EXACTLY ONE:
//
//   - none means the grant was minted for an account this transaction does not
//     debit, and
//   - more than one means the caller submitted several debits out of that
//     account and the grant — which authorizes one exact amount — cannot name
//     which of them it covers.
//
// Both refuse with ErrAccountBlockExceptionInvalid. Refusing here, before the
// atomic script runs, is what keeps the identifier alive: only the script
// consumes, so a refused bind leaves the grant available for the transaction it
// was actually minted for.
//
// The DERIVED legs are the overdraft companions of that primary. A companion is
// recognized by three facts together, all of them properties the enrichment layer
// establishes: it carries the reserved "overdraft" balance key (a key the public
// create-balance route refuses, so no client owns one), it debits the same account
// alias, and its entry key carries the SAME positional index prefix as the
// primary — the enrichment copies that prefix from the leg it derived the
// companion from.
//
// An overdraft-keyed debit on the granted alias that does NOT carry the primary's
// index is not a leg the system derived from it. It is a debit the CALLER placed,
// which gets its own positional index, and it is counted as an extra debit so the
// bind fails closed. Letting it through unbound would be worse than rejecting: on
// an unblocked account the transaction would post, consuming the grant, with that
// second debit never authorized by anything.
func ResolveAccountBlockExceptionBinding(grant *AccountBlockExceptionGrant, legs []AccountBlockExceptionLeg) (*AccountBlockExceptionBinding, error) {
	if grant == nil {
		return nil, nil
	}

	granted := make([]AccountBlockExceptionLeg, 0, len(legs))

	primaries := 0

	var primary AccountBlockExceptionLeg

	for _, leg := range legs {
		if leg.Alias != grant.Alias || leg.Direction != constant.DirectionDebit {
			continue
		}

		granted = append(granted, leg)

		if leg.BalanceKey == constant.OverdraftBalanceKey {
			continue
		}

		primaries++
		primary = leg
	}

	if primaries != 1 {
		return nil, invalidAccountBlockException()
	}

	primaryIndex := entryKeyIndexPrefix(primary.EntryKey)

	binding := &AccountBlockExceptionBinding{
		ID:           grant.ID,
		Alias:        grant.Alias,
		Amount:       primary.Amount,
		authorized:   map[string]struct{}{AliasKey(primary.Alias, primary.BalanceKey): {}},
		internalKeys: []string{primary.InternalKey},
	}

	for _, leg := range granted {
		if leg.BalanceKey != constant.OverdraftBalanceKey {
			continue
		}

		if entryKeyIndexPrefix(leg.EntryKey) != primaryIndex {
			return nil, invalidAccountBlockException()
		}

		identity := AliasKey(leg.Alias, leg.BalanceKey)
		if _, seen := binding.authorized[identity]; seen {
			continue
		}

		binding.authorized[identity] = struct{}{}
		binding.internalKeys = append(binding.internalKeys, leg.InternalKey)
	}

	return binding, nil
}

// invalidAccountBlockException is the single rejection every failed bind returns,
// so the caller cannot tell "minted for another account" from "cannot name which
// debit" — both mean the presented identifier does not authorize this transaction,
// and neither consumes it.
func invalidAccountBlockException() error {
	return pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionInvalid, constant.EntityTransaction)
}

// entryKeyIndexPrefix returns the leading "<index>#" of a composite entry key, or
// the empty string when it carries none. It mirrors the enrichment layer's own
// prefix derivation, which is what makes a derived companion recognizable as
// belonging to one specific primary leg rather than to the alias at large.
func entryKeyIndexPrefix(entryKey string) string {
	for i := 0; i < len(entryKey); i++ {
		if entryKey[i] == AliasSeparator {
			return entryKey[:i+1]
		}
	}

	return ""
}
