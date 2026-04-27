// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"
)

// validateUpdateSettings runs the BalanceSettings contract check before any
// repository interaction. Failing closed here keeps corrupt payloads out of
// PostgreSQL and guarantees no partial writes on validation failure.
//
// It also rejects any attempt to set BalanceScope="internal" via PATCH:
// internal scope is reserved for system-managed balances (auto-created
// overdraft balances) and MUST NOT be settable through the public API.
func validateUpdateSettings(ctx context.Context, logger libLog.Logger, span trace.Span, settings *mmodel.BalanceSettings) error {
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
// overdraft usage. Two transitions are rejected:
//
//  1. disabling AllowOverdraft while OverdraftUsed > 0 — would orphan debt.
//  2. reducing OverdraftLimit below OverdraftUsed — would leave the balance
//     in a permanently over-limit state with no ability to recover.
//
// Both rules are enforced before the repository
// Update is invoked.
func enforceOverdraftTransition(ctx context.Context, logger libLog.Logger, span trace.Span, current *mmodel.Balance, next *mmodel.BalanceSettings) error {
	if current == nil || next == nil {
		return nil
	}

	usage := current.OverdraftUsed
	if usage.IsZero() {
		return nil
	}

	if !next.AllowOverdraft {
		err := pkg.ValidateBusinessError(constant.ErrOverdraftDisableWithUsage, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Cannot disable overdraft while usage is non-zero", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected overdraft disable with active usage")

		return err
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

// ensureOverdraftBalance auto-creates the system-managed overdraft balance
// when AllowOverdraft transitions from false to true. The operation is
// idempotent: if a balance with key="overdraft" already exists for the
// account, no new row is created.
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
// Two concurrent PATCH requests flipping AllowOverdraft from false→true on
// the same account would each observe "no overdraft balance" in the initial
// Find lookup and race to Create. Application-level locking cannot close
// this window cleanly across replicas, so the invariant is enforced at the
// storage layer: a partial UNIQUE index on
// (organization_id, ledger_id, account_id, asset_code, key) rejects the
// second insert with SQLSTATE 23505.
//
// When that happens we treat the race as benign — the peer request already
// created the balance we were about to create — and re-fetch it so the
// caller sees the same state it would have in the single-request case.
// Any other Create failure (connectivity, constraint violations we did not
// trigger, etc.) is propagated unchanged.
func (uc *UseCase) ensureOverdraftBalance(ctx context.Context, logger libLog.Logger, span trace.Span, organizationID, ledgerID uuid.UUID, current *mmodel.Balance, nextSettings *mmodel.BalanceSettings) error {
	// Concurrency model: auto-creation is idempotent. Two concurrent
	// PATCH-enable requests on the same account will both attempt to
	// create the overdraft companion balance. The partial unique index
	// idx_unique_balance_account_key (migration 000032) on
	// (organization_id, ledger_id, account_id, asset_code, key)
	// WHERE deleted_at IS NULL guarantees only one row survives.
	// isUniqueViolation (line ~247) catches the duplicate-key error
	// and treats it as a no-op — the second caller proceeds with the
	// companion that the first caller created.
	if current == nil || nextSettings == nil {
		return nil
	}

	if !nextSettings.AllowOverdraft {
		return nil
	}

	// Only act on the false→true transition. Anything else is a no-op.
	if current.Settings != nil && current.Settings.AllowOverdraft {
		return nil
	}

	// An empty AccountID can only occur in tests that seed a minimal
	// balance fixture. We fall back to uuid.Nil rather than failing the
	// update because the overdraft auto-creation is a best-effort extension
	// of the settings update — the parent balance has already been persisted.
	var accountID uuid.UUID

	if current.AccountID != "" {
		parsed, perr := uuid.Parse(current.AccountID)
		if perr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account id on current balance", perr)
			logger.Log(ctx, libLog.LevelError, "Failed to parse account ID", libLog.String("accountID", current.AccountID), libLog.Err(perr))

			return perr
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
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check for existing overdraft balance", ferr)
			logger.Log(ctx, libLog.LevelError, "Failed to check existing overdraft balance", libLog.Err(ferr))

			return ferr
		}

		existing = nil
	}

	if existing != nil {
		// Idempotent: the overdraft balance already exists.
		return nil
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

		return uuidErr
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

	if _, cerr := uc.BalanceRepo.Create(ctx, overdraftBalance); cerr != nil {
		// Benign race: a concurrent request won the Create. The partial
		// UNIQUE index idx_unique_balance_account_key surfaces this as a
		// PostgreSQL unique_violation (SQLSTATE 23505). Treat it as
		// idempotent success — the peer request materialized the same row
		// we were about to create. Verify with a follow-up Find so we
		// never silently swallow a 23505 triggered by an unrelated index.
		if isUniqueViolation(cerr) {
			reloaded, reloadErr := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.OverdraftBalanceKey)
			if reloadErr == nil && reloaded != nil {
				logger.Log(ctx, libLog.LevelInfo, "Overdraft balance created concurrently by peer request, treating as idempotent success", libLog.String("accountID", current.AccountID))

				return nil
			}
			// Fall through and surface the original Create error if the
			// reload failed or still returned nil — that means the 23505
			// did not come from our target tuple.
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to auto-create overdraft balance", cerr)
		logger.Log(ctx, libLog.LevelError, "Failed to auto-create overdraft balance", libLog.Err(cerr))

		return cerr
	}

	logger.Log(ctx, libLog.LevelInfo, "Auto-created overdraft balance", libLog.String("accountID", current.AccountID))

	return nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique_violation
// (SQLSTATE 23505). Used by ensureOverdraftBalance to distinguish a benign
// concurrent-insert race from real Create failures. Matches the pattern
// already used in create_balance_transaction_operations_async.go.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode
}
