// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// validateUpdateSettings runs the BalanceSettings contract check before any
// repository interaction. Failing closed here keeps corrupt payloads out of
// PostgreSQL and guarantees no partial writes on validation failure.
//
// It also rejects any attempt to set BalanceScope="internal" via PATCH:
// internal scope is reserved for system-managed balances (auto-created
// overdraft balances) and MUST NOT be settable through the public API.
func validateUpdateSettings(ctx context.Context, logger libLog.Logger, span trace.Span, settings *mmodel.BalanceSettings) error {
	settings.Normalize()

	if err := settings.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid balance settings payload", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected invalid balance settings", libLog.Err(err))

		return pkg.ValidateBusinessError(constant.ErrInvalidBalanceSettings, constant.EntityBalance)
	}

	if settings.BalanceScope == mmodel.BalanceScopeInternal {
		err := pkg.ValidateBusinessError(constant.ErrInvalidBalanceSettings, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserved balance scope", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected reserved balance scope on update", libLog.String("scope", settings.BalanceScope))

		return err
	}

	return nil
}

// enforceOverdraftTransition protects balances that carry outstanding
// overdraft usage from being reconfigured into an impossible limit state.
// Disabling AllowOverdraft is allowed even when OverdraftUsed > 0: the flag
// controls future debit-side overdraft usage, while incoming credits must still
// be able to repay the existing debt.
//
// The only rejected transition is reducing OverdraftLimit below OverdraftUsed,
// which would leave the balance permanently over-limit. This rule is enforced
// before the repository Update is invoked.
func enforceOverdraftTransition(ctx context.Context, logger libLog.Logger, span trace.Span, current *mmodel.Balance, next *mmodel.BalanceSettings) error {
	if current == nil || next == nil {
		return nil
	}

	usage := current.OverdraftUsed
	if usage.IsZero() {
		return nil
	}

	if next.OverdraftLimitEnabled && next.OverdraftLimit != nil {
		limit, perr := decimal.NewFromString(*next.OverdraftLimit)
		if perr != nil {
			// Should not happen — Validate() already parsed it. Treat as
			// invalid payload to stay fail-closed.
			wrapped := pkg.ValidateBusinessError(constant.ErrInvalidBalanceSettings, constant.EntityBalance)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Overdraft limit is not a valid decimal", perr)
			logger.Log(ctx, libLog.LevelWarn, "Invalid overdraft limit after validation", libLog.Err(perr))

			return wrapped
		}

		if limit.LessThan(usage) {
			err := pkg.ValidateBusinessError(constant.ErrOverdraftLimitBelowUsage, constant.EntityBalance)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Overdraft limit below current usage", err)
			logger.Log(ctx, libLog.LevelWarn, "Rejected overdraft limit below current usage")

			return err
		}
	}

	return nil
}

// ensureOverdraftBalance returns the system-managed overdraft companion of the
// account whenever nextSettings asks for AllowOverdraft, creating it when it does
// not exist yet. It runs BEFORE the parent's settings are persisted, whatever the
// current read says about the parent: another PATCH may switch the overdraft off
// between that read and this PATCH's UPDATE, so the companion — and the payload of
// balance.config_changed{overdraft_enabled} — must be ready for the UPDATE that
// actually switches it on. A failure here keeps the parent untouched.
//
// created reports whether this call materialized the companion. Callers that
// announce the companion's creation use it; the PATCH announces the overdraft
// switching on instead, which only the UPDATE that made the transition knows.
//
// The auto-created balance is:
//   - key        = "overdraft"
//   - direction  = "debit"
//   - scope      = "internal" (protected from direct operations)
//   - alias      = same as the parent balance
//   - asset code = same as the parent balance
//
// Concurrency model:
//
// Two concurrent requests enabling the overdraft on the same account would each
// observe "no overdraft balance" in the initial lookup and race to Create.
// Application-level locking cannot close this window cleanly across replicas, so
// the invariant is enforced at the storage layer: a partial UNIQUE index on
// (organization_id, ledger_id, account_id, asset_code, key) rejects the second
// insert with SQLSTATE 23505. The loser reads the winner's companion back from
// the PRIMARY — a lagging replica would not have it yet — and returns it with
// created=false. Any other Create failure (connectivity, constraint violations we
// did not trigger, etc.) is propagated unchanged.
func (uc *UseCase) ensureOverdraftBalance(ctx context.Context, logger libLog.Logger, span trace.Span, organizationID, ledgerID uuid.UUID, current *mmodel.Balance, nextSettings *mmodel.BalanceSettings) (companion *mmodel.Balance, created bool, err error) {
	if current == nil || nextSettings == nil || !nextSettings.AllowOverdraft {
		return nil, false, nil
	}

	// An empty AccountID can only occur in tests that seed a minimal
	// balance fixture. We fall back to uuid.Nil rather than failing the
	// update.
	var accountID uuid.UUID

	if current.AccountID != "" {
		parsed, perr := uuid.Parse(current.AccountID)
		if perr != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid account id on current balance", perr)
			logger.Log(ctx, libLog.LevelError, "Invalid account id on current balance", libLog.String("accountID", current.AccountID), libLog.Err(perr))

			return nil, false, perr
		}

		accountID = parsed
	}

	existing, ferr := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.OverdraftBalanceKey)
	if ferr != nil {
		// The repository signals "not found" by returning an
		// EntityNotFoundError (see FindByAccountIDAndKey in
		// balance.postgresql.go — sql.ErrNoRows is mapped to
		// pkg.ValidateBusinessError(constant.ErrEntityNotFound, ...)).
		// Only propagate real infrastructure errors; a not-found result
		// is the expected trigger for the auto-creation path below.
		var notFound pkg.EntityNotFoundError
		if !errors.As(ferr, &notFound) {
			recordCommandError(ctx, span, logger, "Failed to check for existing overdraft balance", ferr)

			return nil, false, ferr
		}

		existing = nil
	}

	if existing != nil {
		return existing, false, nil
	}

	// The companion balance participates as BOTH a source (DEBIT grows the
	// liability when the credit balance overdraws) and a destination (CREDIT
	// shrinks the liability when the overdraft is repaid). Both flags are
	// therefore true. Direct user access is blocked by the scope guard at
	// SendTransactionToRedisQueue (BalanceScope=internal → 0168), so leaving
	// AllowSending=true does NOT expose the companion to client-initiated
	// transactions — only the system enrichment engine can route operations
	// onto it.
	balanceUUID, uuidErr := libCommons.GenerateUUIDv7()
	if uuidErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate UUID for overdraft balance", uuidErr)
		logger.Log(ctx, libLog.LevelError, "Failed to generate UUID for overdraft balance", libLog.Err(uuidErr))

		return nil, false, uuidErr
	}

	now := time.Now()

	overdraftBalance := &mmodel.Balance{
		ID:             balanceUUID.String(),
		OrganizationID: current.OrganizationID,
		LedgerID:       current.LedgerID,
		AccountID:      current.AccountID,
		Alias:          current.Alias,
		Key:            constant.OverdraftBalanceKey,
		AssetCode:      current.AssetCode,
		AccountType:    current.AccountType,
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      constant.DirectionDebit,
		OverdraftUsed:  decimal.Zero,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	persisted, cerr := uc.BalanceRepo.Create(ctx, overdraftBalance)
	if cerr != nil {
		// Benign race: a concurrent request won the Create. Read its companion
		// back so we never silently swallow a 23505 triggered by an unrelated
		// index.
		if isUniqueViolation(cerr) {
			winner, reloadErr := uc.findOverdraftCompanionOnPrimary(ctx, organizationID, ledgerID, current)
			if reloadErr == nil {
				logger.Log(ctx, libLog.LevelDebug, "Overdraft balance created concurrently by peer request, treating as idempotent success", libLog.String("account_id", current.AccountID))

				return winner, false, nil
			}
			// Fall through and surface the original Create error if the
			// reload failed or found nothing — that means the 23505 did not
			// come from our target tuple.
		}

		recordCommandError(ctx, span, logger, "Failed to auto-create overdraft balance", cerr)

		return nil, false, cerr
	}

	logger.Log(ctx, libLog.LevelDebug, "Auto-created overdraft balance", libLog.String("account_id", current.AccountID))

	return persisted, true, nil
}

// findOverdraftCompanionOnPrimary reads the overdraft companion of the parent's
// account from the PRIMARY. The companion shares the parent's alias under the
// overdraft key; the account check keeps the read on the parent's own account.
func (uc *UseCase) findOverdraftCompanionOnPrimary(ctx context.Context, organizationID, ledgerID uuid.UUID, parent *mmodel.Balance) (*mmodel.Balance, error) {
	balances, err := uc.BalanceRepo.ListByAliasesWithKeysOnPrimary(ctx, organizationID, ledgerID, []string{parent.Alias + "#" + constant.OverdraftBalanceKey})
	if err != nil {
		return nil, err
	}

	for _, candidate := range balances {
		if candidate != nil && candidate.AccountID == parent.AccountID {
			return candidate, nil
		}
	}

	return nil, errOverdraftCompanionNotOnPrimary
}

var errOverdraftCompanionNotOnPrimary = errors.New("overdraft companion not found on the primary")

// isUniqueViolation reports whether err is a PostgreSQL unique_violation
// (SQLSTATE 23505). Used by ensureOverdraftBalance to distinguish a benign
// concurrent-insert race from real Create failures. Matches the pattern
// already used in create_balance_transaction_operations_async.go.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode
}
