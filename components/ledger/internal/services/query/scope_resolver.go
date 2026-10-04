// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"slices"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// MaxScopeAliases is the largest number of distinct aliases one
// AccountIDsByAlias call resolves.
const MaxScopeAliases = 100

// ErrScopeTransactionAccountsUnavailable reports a transaction that exists but
// names no account yet: no operation row is projected and its body carries no
// leg. The caller cannot tell which accounts it touches and must not guess.
var ErrScopeTransactionAccountsUnavailable = errors.New("transaction exists but names no account yet")

// ScopeAliasBatchTooLargeError refuses a question with more than Max distinct aliases.
type ScopeAliasBatchTooLargeError struct {
	Count int
	Max   int
}

func (e ScopeAliasBatchTooLargeError) Error() string {
	return fmt.Sprintf("%d distinct aliases requested, at most %d are resolved at once", e.Count, e.Max)
}

// ScopeAliasAmbiguousError reports an alias held by more than one live account
// of the scope. The alias is not resolved to either.
type ScopeAliasAmbiguousError struct {
	Alias      string
	AccountIDs []uuid.UUID
}

func (e ScopeAliasAmbiguousError) Error() string {
	return fmt.Sprintf("alias %q is held by %d live accounts", e.Alias, len(e.AccountIDs))
}

// AliasResolution answers an alias question. Every distinct alias asked lands in
// exactly one of the two fields: AccountIDs when a live account of the scope
// holds it, NotFound (in the order first asked) when none does.
type AliasResolution struct {
	AccountIDs map[string]uuid.UUID
	NotFound   []string
}

// ScopeResolver translates the indirect references a request carries into the
// account ids they stand for, inside one organization and ledger. Absence is
// reported apart from failure: a not-found answer comes back with a nil error,
// so the caller can refuse it naming the reference.
type ScopeResolver interface {
	// AccountIDsByAlias resolves up to MaxScopeAliases distinct aliases in one
	// read. Aliases compare exactly; the external account of an asset is the
	// alias "@external/<ASSET>".
	AccountIDsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) (*AliasResolution, error)
	// AccountIDsOfTransaction returns the distinct accounts a transaction
	// touches, on both sides, whether it is settled, pending or reverted.
	AccountIDsOfTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) ([]uuid.UUID, bool, error)
	// AccountIDOfBalance returns the account that owns a balance.
	AccountIDOfBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (uuid.UUID, bool, error)
	// LedgerIDsOfHolders returns, per holder, the ledgers of the organization it
	// owns a live account in, the rule the holder list is confined by. A holder
	// without one is absent from the answer.
	LedgerIDsOfHolders(ctx context.Context, organizationID uuid.UUID, holderIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
}

var _ ScopeResolver = (*UseCase)(nil)

// AccountIDsByAlias implements ScopeResolver.
func (uc *UseCase) AccountIDsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) (*AliasResolution, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.scope_resolver.account_ids_by_alias")
	defer span.End()

	distinct := distinctAliases(aliases)

	span.SetAttributes(attribute.Int("app.scope.aliases", len(distinct)))

	if len(distinct) > MaxScopeAliases {
		err := ScopeAliasBatchTooLargeError{Count: len(distinct), Max: MaxScopeAliases}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Alias batch too large", err)

		return nil, err
	}

	resolution := &AliasResolution{AccountIDs: make(map[string]uuid.UUID, len(distinct)), NotFound: []string{}}

	if len(distinct) == 0 {
		return resolution, nil
	}

	held, err := uc.liveAccountsByAlias(ctx, organizationID, ledgerID, nonEmpty(distinct))
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve aliases", err)

		return nil, err
	}

	for _, alias := range distinct {
		ids := held[alias]

		switch len(ids) {
		case 0:
			resolution.NotFound = append(resolution.NotFound, alias)
		case 1:
			resolution.AccountIDs[alias] = ids[0]
		default:
			err := ScopeAliasAmbiguousError{Alias: alias, AccountIDs: ids}

			libOpentelemetry.HandleSpanError(span, "Alias held by more than one live account", err)

			return nil, err
		}
	}

	span.SetAttributes(attribute.Int("app.scope.aliases_not_found", len(resolution.NotFound)))

	return resolution, nil
}

// AccountIDsOfTransaction implements ScopeResolver.
//
// The persisted record answers first. A transaction it does not hold may still
// be one a commit, cancel or revert can act on, because the write path answers
// the caller before projecting: the engine index and then the write-behind
// entry are consulted, the same sources the lifecycle writes load from.
//
// The accounts of a transaction never change once it is recorded, so whichever
// source answers, the set is the same; the order only decides which read is
// paid for.
func (uc *UseCase) AccountIDsOfTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) ([]uuid.UUID, bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.scope_resolver.account_ids_of_transaction")
	defer span.End()

	refs, found, err := uc.transactionAccountRefs(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to load transaction account references", err)

		return nil, false, err
	}

	if !found {
		span.SetAttributes(attribute.Bool("app.scope.transaction_found", false))

		return nil, false, nil
	}

	ids, err := uc.accountIDsOfRefs(ctx, organizationID, ledgerID, refs)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve transaction accounts", err)

		return nil, false, err
	}

	span.SetAttributes(attribute.Int("app.scope.transaction_accounts", len(ids)))

	return ids, true, nil
}

// AccountIDOfBalance implements ScopeResolver.
func (uc *UseCase) AccountIDOfBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (uuid.UUID, bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.scope_resolver.account_id_of_balance")
	defer span.End()

	found, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if isEntityNotFound(err) {
		return uuid.Nil, false, nil
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find balance", err)

		return uuid.Nil, false, err
	}

	accountID, err := uuid.Parse(found.AccountID)
	if err != nil {
		err = fmt.Errorf("balance %s carries account id %q that is not a uuid: %w", balanceID, found.AccountID, err)

		libOpentelemetry.HandleSpanError(span, "Failed to parse balance account id", err)

		return uuid.Nil, false, err
	}

	return accountID, true, nil
}

// LedgerIDsOfHolders implements ScopeResolver.
func (uc *UseCase) LedgerIDsOfHolders(ctx context.Context, organizationID uuid.UUID, holderIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.scope_resolver.ledger_ids_of_holders")
	defer span.End()

	distinct := make([]uuid.UUID, 0, len(holderIDs))
	for _, id := range holderIDs {
		if !slices.Contains(distinct, id) {
			distinct = append(distinct, id)
		}
	}

	span.SetAttributes(attribute.Int("app.scope.holders", len(distinct)))

	if len(distinct) == 0 {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}

	ledgers, err := uc.AccountRepo.ListLedgerIDsOfHolders(ctx, organizationID, distinct)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve holder ledgers", err)

		return nil, err
	}

	return ledgers, nil
}

// transactionAccountRefs loads the account references of a transaction from the
// first source that holds it.
func (uc *UseCase) transactionAccountRefs(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.AccountRefs, bool, error) {
	refs, err := uc.TransactionRepo.ListAccountRefsByTransaction(ctx, organizationID, ledgerID, transactionID)
	if err == nil {
		return refs, true, nil
	}

	if !isEntityNotFound(err) {
		return nil, false, err
	}

	if uc.CanResolveEngineWriteBehind() {
		resolved, err := uc.ResolveEngineWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
		if err != nil && !isEntityNotFound(err) && !errors.Is(err, redis.ErrEngineWriteBehindNotFound) {
			return nil, false, err
		}

		if err == nil && resolved != nil && resolved.Transaction != nil && resolved.Transaction.ID != "" {
			indexed, indexedErr := refsOfTransaction(resolved.Transaction)

			return indexed, indexedErr == nil, indexedErr
		}
	}

	if uc.TransactionRedisRepo == nil {
		return nil, false, nil
	}

	cached, err := uc.GetWriteBehindTransaction(ctx, organizationID, ledgerID, transactionID)
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	if cached == nil || cached.ID == "" {
		return nil, false, nil
	}

	refs, err = refsOfTransaction(cached)

	return refs, err == nil, err
}

// refsOfTransaction reads the account references off a transaction loaded from
// a cache, which carries its operations and body together.
func refsOfTransaction(tran *transaction.Transaction) (*transaction.AccountRefs, error) {
	refs := &transaction.AccountRefs{Body: tran.Body, AccountIDs: make([]uuid.UUID, 0, len(tran.Operations))}

	for _, op := range tran.Operations {
		if op == nil || op.AccountID == "" {
			continue
		}

		id, err := uuid.Parse(op.AccountID)
		if err != nil {
			return nil, fmt.Errorf("operation %s carries account id %q that is not a uuid: %w", op.ID, op.AccountID, err)
		}

		refs.AccountIDs = append(refs.AccountIDs, id)
	}

	return refs, nil
}

// accountIDsOfRefs unites the accounts of the operation rows with the accounts
// the body legs name. A leg alias that no live account holds any longer names
// nothing the transaction can still move, so it adds no account; an alias held
// by more than one live account adds all of them.
func (uc *UseCase) accountIDsOfRefs(ctx context.Context, organizationID, ledgerID uuid.UUID, refs *transaction.AccountRefs) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]struct{}, len(refs.AccountIDs))
	ids := make([]uuid.UUID, 0, len(refs.AccountIDs))

	add := func(id uuid.UUID) {
		if _, dup := seen[id]; dup {
			return
		}

		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	for _, id := range refs.AccountIDs {
		add(id)
	}

	if aliases := bodyLegAliases(refs.Body); len(aliases) > 0 {
		held, err := uc.liveAccountsByAlias(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			return nil, err
		}

		for _, alias := range aliases {
			for _, id := range held[alias] {
				add(id)
			}
		}
	}

	if len(ids) == 0 {
		return nil, ErrScopeTransactionAccountsUnavailable
	}

	return ids, nil
}

// liveAccountsByAlias reads the live accounts holding any of the aliases in one
// statement and groups their ids by alias.
func (uc *UseCase) liveAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) (map[string][]uuid.UUID, error) {
	if len(aliases) == 0 {
		return map[string][]uuid.UUID{}, nil
	}

	accounts, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)
	if err != nil && !errors.Is(err, services.ErrDatabaseItemNotFound) {
		return nil, err
	}

	held := make(map[string][]uuid.UUID, len(accounts))

	for _, acc := range accounts {
		if acc == nil || acc.Alias == nil {
			continue
		}

		id, err := accountUUID(acc)
		if err != nil {
			return nil, err
		}

		if !slices.Contains(held[*acc.Alias], id) {
			held[*acc.Alias] = append(held[*acc.Alias], id)
		}
	}

	return held, nil
}

func accountUUID(acc *mmodel.Account) (uuid.UUID, error) {
	id, err := uuid.Parse(acc.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("account id %q is not a uuid: %w", acc.ID, err)
	}

	return id, nil
}

// bodyLegAliases returns the distinct bare aliases of every leg of a submitted
// body, both sides, in submitted order.
func bodyLegAliases(body mtransaction.Transaction) []string {
	legs := make([]string, 0, len(body.Send.Source.From)+len(body.Send.Distribute.To))

	for _, entry := range body.Send.Source.From {
		legs = append(legs, mtransaction.BareAlias(entry.AccountAlias))
	}

	for _, entry := range body.Send.Distribute.To {
		legs = append(legs, mtransaction.BareAlias(entry.AccountAlias))
	}

	return nonEmpty(distinctAliases(legs))
}

// distinctAliases drops repeated aliases, keeping first-seen order. An empty
// alias is kept: no account holds it, and the caller reports it as not found.
func distinctAliases(aliases []string) []string {
	seen := make(map[string]struct{}, len(aliases))
	out := make([]string, 0, len(aliases))

	for _, alias := range aliases {
		if _, dup := seen[alias]; dup {
			continue
		}

		seen[alias] = struct{}{}
		out = append(out, alias)
	}

	return out
}

// nonEmpty drops the empty alias, which no account can hold and no read needs to ask.
func nonEmpty(aliases []string) []string {
	return slices.DeleteFunc(slices.Clone(aliases), func(alias string) bool { return alias == "" })
}

func isEntityNotFound(err error) bool {
	var notFound pkg.EntityNotFoundError

	return errors.As(err, &notFound)
}
