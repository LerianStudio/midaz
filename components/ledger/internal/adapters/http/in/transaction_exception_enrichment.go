// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/LerianStudio/lib-observability/v2/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// exceptionComponent is the low-cardinality metric/span component label for the
// account-exception enrichment. It never carries a free integrator identifier.
const exceptionComponent = "ledger"

// exception evaluation result labels (bounded enum — never a free identifier).
const (
	exceptionResultGranted    = "granted"
	exceptionResultNoMatch    = "no_match"
	exceptionResultStoreError = "store_error"
)

// activeExceptionsLoader abstracts the read of an account's live exception set.
// It mirrors the signature of query.UseCase.GetActiveAccountExceptions so the
// enrichment can be exercised without the full query use case (same injectable
// pattern as companionBalanceLoader for the overdraft enrichment).
type activeExceptionsLoader func(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.AccountException, error)

// blockedAccountsResolver abstracts the pre-read of the blocked-accounts index.
// It mirrors the signature of RedisRepository.ResolveBlockedAccounts, whose
// contract it inherits whole: the answer is authoritative or it is an error —
// there is no third outcome the caller has to interpret.
type blockedAccountsResolver func(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) ([]uuid.UUID, error)

// exceptionGrantCandidate is one would-be-deny transaction side that may be
// rescued by a matching account exception. It holds the live map reference and
// the exact map key so the grant is written back read-modify-write.
type exceptionGrantCandidate struct {
	amounts map[string]mtransaction.Amount // validate.From or validate.To
	key     string                         // the concrete map key (concat form)
	balance *mmodel.Balance
}

// enrichAccountExceptionGrants is the transaction-time producer of account-block
// exception grants (RF-06/07/09/4B). It runs BETWEEN rejectInternalScopeBalances
// and buildBalanceOperations in executeCreateTransaction, mutating the matched
// side's Amount on validate.From/To so the downstream validators
// (ValidateBalancesRules) transpass the 0502/0024 gates for that entry only.
//
// It returns the appliedExceptionID (the FIRST grant in the deterministic
// sources->destinations order) for persistence on the transaction record, or nil
// when no grant applies.
//
// Cost-zero skips (ADR-007): an empty operationalTypeCode returns nil with NO I/O
// (the common path stays untouched — the loader is never called); a transaction
// with no would-be-deny side likewise returns nil with no loader call.
//
// Fail-closed (ADR-007/TRD §6): a loader error emits
// account_exception_evaluations_total{result="store_error"}, logs at Error,
// attributes the span, and grants nothing for that side — the transaction
// proceeds and the validators deny it with 0502/0024. The store's unavailability
// never produces a 500, never fails open, and never aborts the flow.
func enrichAccountExceptionGrants(
	ctx context.Context,
	loader activeExceptionsLoader,
	metricsFactory *metrics.MetricsFactory,
	organizationID, ledgerID uuid.UUID,
	operationalTypeCode string,
	validate *mtransaction.Responses,
	balances []*mmodel.Balance,
	blockedAccountIDs map[string]struct{},
) *string {
	// Cost-zero skip #1: no operational type code => common path untouched, no I/O.
	if operationalTypeCode == "" || validate == nil {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "enrich.account_exception")
	defer span.End()

	candidates := collectWouldBeDenyCandidates(validate, balances, blockedAccountIDs)

	span.SetAttributes(attribute.Int("app.exception.would_be_deny_sides", len(candidates)))

	// Cost-zero skip #2: nothing would be denied => no loader call.
	if len(candidates) == 0 {
		return nil
	}

	start := time.Now()
	defer utils.RecordAccountExceptionEvaluationDuration(ctx, metricsFactory, logger, exceptionComponent, start)

	// One decision instant for the whole enrichment (RF-4B/RF-09 determinism).
	now := time.Now().UTC()

	// Lazy, deduplicated loads: one loader call per distinct AccountID. The
	// outcome (including an error) is cached so a second side on the same account
	// never issues a second read.
	type loadResult struct {
		exceptions []*mmodel.AccountException
		err        error
	}

	loaded := make(map[string]loadResult)

	loadFor := func(accountID string) loadResult {
		if r, ok := loaded[accountID]; ok {
			return r
		}

		parsed, parseErr := uuid.Parse(accountID)
		if parseErr != nil {
			r := loadResult{err: parseErr}
			loaded[accountID] = r

			return r
		}

		exceptions, err := loader(ctx, organizationID, ledgerID, parsed)
		r := loadResult{exceptions: exceptions, err: err}
		loaded[accountID] = r

		return r
	}

	var (
		appliedExceptionID *string
		grantedIDs         []string
	)

	for _, c := range candidates {
		res := loadFor(c.balance.AccountID)
		if res.err != nil {
			// Fail-closed: no grant for this side; validators deny 0502/0024.
			libOpentelemetry.HandleSpanError(span, "Failed to load account exceptions for enrichment", res.err)
			logger.Log(ctx, libLog.LevelError, "Failed to load account exceptions; failing closed (no grant)",
				libLog.String("alias", c.balance.Alias),
				libLog.Err(res.err))

			utils.RecordAccountExceptionEvaluation(ctx, metricsFactory, logger, exceptionComponent, exceptionResultStoreError)

			continue
		}

		matched := matchAccountException(res.exceptions, operationalTypeCode, c.balance.Key, now)
		if matched == nil {
			utils.RecordAccountExceptionEvaluation(ctx, metricsFactory, logger, exceptionComponent, exceptionResultNoMatch)

			continue
		}

		// Grant: read-modify-write on the matched side's Amount (map value type).
		amt := c.amounts[c.key]
		amt.BlockBypassGranted = true
		amt.GrantedExceptionID = matched.ID
		c.amounts[c.key] = amt

		grantedIDs = append(grantedIDs, matched.ID)

		if appliedExceptionID == nil {
			id := matched.ID
			appliedExceptionID = &id
		}

		utils.RecordAccountExceptionEvaluation(ctx, metricsFactory, logger, exceptionComponent, exceptionResultGranted)
	}

	span.SetAttributes(
		attribute.Int("app.exception.accounts_evaluated", len(loaded)),
		attribute.Int("app.exception.granted_count", len(grantedIDs)),
		attribute.StringSlice("app.exception.granted_ids", grantedIDs),
	)

	return appliedExceptionID
}

// collectWouldBeDenyCandidates walks validate.Aliases in the deterministic
// sources->destinations order and returns every side that would be denied by the
// phase-1 validators (blocked account, or a status flag that forbids the
// direction). The alias<->balance and alias<->map-key matching is identical to
// the validators' (`key == balance.ID || SplitAliasWithKey(key) == AliasKey(alias, key)`),
// so a grant produced here lands exactly where the validator reads it. Map order
// is never used — only validate.Aliases — so the traversal is deterministic.
//
// blockedAccountIDs is the answer of the blocked-accounts index, and it counts
// as a would-be-deny reason on its own. It is empty on the create path, whose
// block signal is still the balance projection; the commit path fills it from
// the pre-read. Reading BOTH sources is what lets the projection be deleted in
// phase 2 without this collector changing again.
func collectWouldBeDenyCandidates(validate *mtransaction.Responses, balances []*mmodel.Balance, blockedAccountIDs map[string]struct{}) []exceptionGrantCandidate {
	candidates := make([]exceptionGrantCandidate, 0)

	for _, alias := range validate.Aliases {
		balance := matchBalanceForAlias(balances, alias)
		if balance == nil {
			continue
		}

		_, indexedAsBlocked := blockedAccountIDs[balance.AccountID]
		blocked := balance.AccountBlocked || indexedAsBlocked

		// Source (debit) side: blocked or sending disallowed.
		if key, ok := matchAmountKey(validate.From, balance); ok {
			if blocked || !balance.AllowSending {
				candidates = append(candidates, exceptionGrantCandidate{amounts: validate.From, key: key, balance: balance})
			}
		}

		// Destination (credit) side: blocked or receiving disallowed.
		if key, ok := matchAmountKey(validate.To, balance); ok {
			if blocked || !balance.AllowReceiving {
				candidates = append(candidates, exceptionGrantCandidate{amounts: validate.To, key: key, balance: balance})
			}
		}
	}

	return candidates
}

// reevaluateAccountExceptionGrants is the commit-time entry of the same
// enrichment the create path runs (REVISED ADR-004: evaluate at every mutation
// of balance, never inherit).
//
// A commit moves money at its own instant, so the exception set that authorizes
// it is the one live at THAT instant: an exception that expired while the
// transaction sat pending must no longer rescue it, and one created after the
// pending must. Inheriting the create-time grant gets both cases wrong.
//
// The trigger is the blocked-accounts index, not the balance projection, and the
// order of the two skips is what keeps it cheap:
//
//  1. no operationalTypeCode => return with NO I/O at all. No exception can
//     apply, so neither the index nor the store is worth a round-trip; if the
//     account is blocked, the atomic script denies the commit by itself.
//  2. nothing blocked => return without touching the exception store. One
//     SMISMEMBER is the entire cost of a commit that carries a code.
//
// An index that cannot be read is returned as an error and MUST fail the commit
// as infrastructure. It is deliberately not degraded into "proceed": that would
// answer an outage with a wrong business verdict — a 0502 for an account that
// held a perfectly valid exception. The exception STORE failing is the opposite
// case and stays fail-closed inside enrichAccountExceptionGrants: it grants
// nothing, emits store_error, and lets the script deny.
func reevaluateAccountExceptionGrants(
	ctx context.Context,
	resolveBlocked blockedAccountsResolver,
	loader activeExceptionsLoader,
	metricsFactory *metrics.MetricsFactory,
	organizationID, ledgerID uuid.UUID,
	operationalTypeCode string,
	validate *mtransaction.Responses,
	balances []*mmodel.Balance,
) (*string, error) {
	// Cost-zero skip #1: nothing an exception could rescue.
	if operationalTypeCode == "" || validate == nil || resolveBlocked == nil {
		return nil, nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "enrich.account_exception.reevaluate")
	defer span.End()

	accountIDs := distinctAccountIDs(balances)

	span.SetAttributes(attribute.Int("app.exception.accounts_probed", len(accountIDs)))

	blockedIDs, err := resolveBlocked(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read the blocked accounts index on commit", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read the blocked accounts index on commit", libLog.Err(err))

		return nil, err
	}

	// Cost-zero skip #2: no involved account is blocked, so no grant can be
	// needed and the exception store stays untouched.
	if len(blockedIDs) == 0 {
		return nil, nil
	}

	blocked := make(map[string]struct{}, len(blockedIDs))
	for _, accountID := range blockedIDs {
		blocked[accountID.String()] = struct{}{}
	}

	span.SetAttributes(attribute.Int("app.exception.blocked_accounts", len(blocked)))

	return enrichAccountExceptionGrants(ctx, loader, metricsFactory, organizationID, ledgerID,
		operationalTypeCode, validate, balances, blocked), nil
}

// distinctAccountIDs returns the parseable account ids of the given balances,
// deduplicated and in first-seen order.
//
// A balance whose AccountID is not a UUID is skipped rather than fatal: the
// enrichment already treats it that way, and one malformed row must not be able
// to fail an otherwise valid commit. It is also never sent to the index, which
// would only be able to answer "not a member" about it.
func distinctAccountIDs(balances []*mmodel.Balance) []uuid.UUID {
	accountIDs := make([]uuid.UUID, 0, len(balances))
	seen := make(map[string]struct{}, len(balances))

	for _, balance := range balances {
		if balance == nil {
			continue
		}

		if _, ok := seen[balance.AccountID]; ok {
			continue
		}

		seen[balance.AccountID] = struct{}{}

		parsed, err := uuid.Parse(balance.AccountID)
		if err != nil {
			continue
		}

		accountIDs = append(accountIDs, parsed)
	}

	return accountIDs
}

// matchBalanceForAlias returns the balance whose alias-key form (or ID) equals
// the bare alias-key carried in validate.Aliases, or nil when none matches.
func matchBalanceForAlias(balances []*mmodel.Balance, alias string) *mmodel.Balance {
	for _, b := range balances {
		if b == nil {
			continue
		}

		if b.ID == alias || mtransaction.AliasKey(b.Alias, b.Key) == alias {
			return b
		}
	}

	return nil
}

// matchAmountKey returns the concrete key of the amount map that matches the
// balance, using the validators' exact correspondence.
func matchAmountKey(amounts map[string]mtransaction.Amount, balance *mmodel.Balance) (string, bool) {
	if amounts == nil {
		return "", false
	}

	balanceAliasKey := mtransaction.AliasKey(balance.Alias, balance.Key)

	for key := range amounts {
		if key == balance.ID || mtransaction.SplitAliasWithKey(key) == balanceAliasKey {
			return key, true
		}
	}

	return "", false
}

// matchAccountException returns the FIRST exception that authorizes the block
// bypass for the given operational type code and balance key at instant `now`,
// or nil when none matches. The loader delivers exceptions oldest-first
// (created_at ASC, Task 2.1.3), so first-match is the oldest matching rule.
//
// Match rule (all conjuncts required):
//   - operationalTypeCode ∈ OperationalTypeCodes (exact equality);
//   - BalanceKey == nil OR *BalanceKey == balanceKey (RF-07 scope);
//   - EffectiveAt == nil OR !now.Before(*EffectiveAt) (inclusive start);
//   - ExpiresAt == nil OR now.Before(*ExpiresAt) (RF-09 exclusive end — an
//     exception that expires exactly at `now` does NOT apply).
func matchAccountException(exceptions []*mmodel.AccountException, operationalTypeCode, balanceKey string, now time.Time) *mmodel.AccountException {
	for _, e := range exceptions {
		if e == nil {
			continue
		}

		if !containsExactCode(e.OperationalTypeCodes, operationalTypeCode) {
			continue
		}

		if e.BalanceKey != nil && *e.BalanceKey != balanceKey {
			continue
		}

		if e.EffectiveAt != nil && now.Before(*e.EffectiveAt) {
			continue
		}

		if e.ExpiresAt != nil && !now.Before(*e.ExpiresAt) {
			continue
		}

		return e
	}

	return nil
}

// containsExactCode reports whether code is present in codes by exact equality.
func containsExactCode(codes []string, code string) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}

	return false
}
