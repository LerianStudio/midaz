//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Fixture classes.
const (
	classOrdinary = "ordinary"
	classHot      = "hot"
	classFee      = "fee"
	classMultikey = "multikey"
	keySecondary  = "secondary"

	benchTenantID = "bench"
	// Above 8000/s × 100 × (60 s warm-up + 1800 s soak + 30 s drain) = 1.512e9: no seeded account runs dry.
	benchFunds int64 = 10_000_000_000
)

// The ordinary balances come first, then these. Multikey's two balances share one account, so only the
// first extraControls carry a control of their own.
var seedExtras = [...]struct{ class, key string }{
	{classHot, constant.DefaultBalanceKey},
	{classFee, constant.DefaultBalanceKey},
	{classMultikey, constant.DefaultBalanceKey},
	{classMultikey, keySecondary},
}

const extraControls = 3

var (
	benchNS       = uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://github.com/LerianStudio/midaz/tests/bench/engine"))
	benchOrgID    = uuid.NewSHA1(benchNS, []byte("organization"))
	benchLedgerID = uuid.NewSHA1(benchNS, []byte("ledger"))
	benchSeedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

func accountID(class string, i int) uuid.UUID {
	return uuid.NewSHA1(benchNS, fmt.Appendf(nil, "account/%s/%d", class, i))
}

func balanceID(class string, i int, key string) uuid.UUID {
	return uuid.NewSHA1(benchNS, fmt.Appendf(nil, "balance/%s/%d/%s", class, i, key))
}

func benchAlias(class string, i int) string {
	return fmt.Sprintf("@bench_%s_%07d", class, i)
}

// seedRow maps row j of a seed with n ordinary balances to its fixture.
func seedRow(n, j int) (class string, i int, key string) {
	if j < n {
		return classOrdinary, j, constant.DefaultBalanceKey
	}

	return seedExtras[j-n].class, 0, seedExtras[j-n].key
}

// seedFixtures streams n ordinary balances plus the extras, their controls and an ownerless fence into the
// freshly bootstrapped schema on the verified primary: one transaction, committed through standby1.
func seedFixtures(ctx context.Context, env *benchEnv, n int) error {
	stack, err := openVerifiedStack(ctx, env)
	if err != nil {
		return err
	}
	defer stack.close(ctx)

	// A NULL deleted_at keeps balances live; NULL settings leaves overdraft unconfigured.
	balanceColumns := []string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "asset_code", "available", "on_hold", "version",
		"account_type", "allow_sending", "allow_receiving", "created_at", "updated_at", "key", "direction", "overdraft_used",
	}
	balances := pgx.CopyFromSlice(n+len(seedExtras), func(j int) ([]any, error) {
		class, i, key := seedRow(n, j)

		return []any{
			balanceID(class, i, key), benchOrgID, benchLedgerID, accountID(class, i), benchAlias(class, i), "BRL",
			benchFunds, 0, 0, "deposit", true, true, benchSeedTime, benchSeedTime, key, constant.DirectionCredit, 0,
		}, nil
	})
	controls := pgx.CopyFromSlice(n+extraControls, func(j int) ([]any, error) {
		class, i, _ := seedRow(n, j)

		return []any{benchOrgID, benchLedgerID, accountID(class, i), false, benchFunds, 0}, nil
	})

	err = pgx.BeginFunc(ctx, stack.primary, func(tx pgx.Tx) error {
		// Hypothesis under test: page room for HOT updates of both rows every debit rewrites.
		if _, err := tx.Exec(ctx, `ALTER TABLE balance SET (fillfactor = 70); ALTER TABLE account_control SET (fillfactor = 70)`); err != nil {
			return err
		}

		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"balance"}, balanceColumns, balances); err != nil {
			return fmt.Errorf("copy balance: %w", err)
		}

		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"account_control"},
			[]string{"organization_id", "ledger_id", "account_id", "blocked", "limit_remaining", "version"}, controls); err != nil {
			return fmt.Errorf("copy account_control: %w", err)
		}

		// A fixed past lease, already expired by the database clock: the first writer takes over.
		_, err := tx.Exec(ctx, `INSERT INTO engine_writer_fence (tenant_id, owner_id, epoch, lease_until) VALUES ($1, '', 0, $2)`,
			benchTenantID, benchSeedTime)

		return err
	})
	if err != nil {
		return fmt.Errorf("seed fixtures: %w", err)
	}

	// VACUUM refuses a transaction block. It settles hint bits and statistics before any measurement reads the rows.
	if _, err := stack.primary.Exec(ctx, `VACUUM (ANALYZE) balance, account_control`); err != nil {
		return fmt.Errorf("vacuum seeded tables: %w", err)
	}

	return nil
}

// timed measures fn on the process clock; only bench measurements read it, never the fixtures.
func timed(fn func() error) (time.Duration, error) {
	start := time.Now()
	err := fn()

	return time.Since(start), err
}
