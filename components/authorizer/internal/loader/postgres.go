// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

const (
	queryBalancesBase = `
SELECT
  id,
  organization_id,
  ledger_id,
  account_id,
  alias,
  key,
  asset_code,
  available,
  on_hold,
  version,
  account_type,
  allow_sending,
  allow_receiving
FROM balance
WHERE deleted_at IS NULL`

	// queryBalancesStreamingBase is the keyset-paginated variant used by
	// LoadBalancesStreaming. Adds updated_at and orders by (updated_at, id)
	// so the (updated_at, id) cursor can advance monotonically under
	// concurrent writes. The id tiebreaker handles rows that share an
	// updated_at timestamp (race conditions at the ms boundary).
	queryBalancesStreamingBase = `
SELECT
  id,
  organization_id,
  ledger_id,
  account_id,
  alias,
  key,
  asset_code,
  available,
  on_hold,
  version,
  account_type,
  allow_sending,
  allow_receiving,
  COALESCE(updated_at, created_at)
FROM balance
WHERE deleted_at IS NULL`

	// initialArgsCap is the initial capacity for query arguments.
	initialArgsCap = 2

	// initialBalancesCap is the initial capacity for the loaded balances slice.
	initialBalancesCap = 1024
)

// Streaming/retry tuning defaults. Keyset pagination preserves a stable cursor
// under concurrent writes so initial cold-start loads do not pin ~50 GB of
// working memory or trip statement_timeout on a single unbounded scan.
const (
	// DefaultStreamingBatchSize is the default number of balance rows fetched
	// per keyset-paginated query page. 10_000 balances ~= ~3 MiB of raw rows
	// which keeps pgx scratch buffers small while amortizing round-trip cost.
	DefaultStreamingBatchSize = 10_000

	// DefaultStreamingWorkers controls how many UpsertBalances consumers run
	// concurrently off the streaming channel. UpsertBalances takes per-shard
	// write locks, so more workers = more parallelism across shards.
	DefaultStreamingWorkers = 4

	// DefaultStreamingChanSize is the buffered channel capacity between the
	// producer (DB rows) and consumers (engine upserts). Sized to one full
	// batch ahead so producer can continue while consumer drains.
	DefaultStreamingChanSize = DefaultStreamingBatchSize

	// DefaultStreamingMaxRetries caps exponential backoff attempts for
	// transient PG errors (too_many_connections=53300, query_canceled=57014).
	DefaultStreamingMaxRetries = 3

	// retryBackoffInitial is the starting backoff between transient retries.
	retryBackoffInitial = 100 * time.Millisecond

	// retryBackoffMax caps the exponential backoff ceiling so a pathological
	// 53300 storm does not stall bootstrap indefinitely. The loader gives up
	// after DefaultStreamingMaxRetries cycles regardless.
	retryBackoffMax = 2 * time.Second

	// PG error codes treated as transient (retryable with backoff).
	pgCodeUndefinedTable     = "42P01" // UNDEFINED_TABLE — fail-closed (NOT retried)
	pgCodeTooManyConnections = "53300" // too_many_connections
	pgCodeQueryCanceled      = "57014" // query_canceled (typically statement_timeout)
)

// ErrBalanceTableMissing is returned when the underlying `balance` table does
// not exist. Previously the loader silently returned (nil, nil) on 42P01 which
// meant the authorizer bootstrapped with zero balances against a missing
// schema — a fail-open behavior that created an undetectable outage window
// during deploys with migration drift. We now fail-closed so bootstrap
// terminates loudly and Kubernetes/operator alerting fires on a crash loop
// rather than a silently-empty engine.
var ErrBalanceTableMissing = errors.New("postgres loader: balance table does not exist")

// PostgresLoader loads balance state from PostgreSQL into the in-memory authorizer.
type PostgresLoader struct {
	pool   *pgxpool.Pool
	router *shard.Router
	// envName is captured at construction time so LoadBalancesStreaming can
	// honor the same non-production allow-list used by enforceDSNSSLMode.
	envName string
}

// PoolConfig holds connection pool tuning parameters for the PostgreSQL loader.
type PoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration

	// EnvName is consulted by the SSL enforcement guard in
	// NewPostgresLoaderWithConfig. Non-production env names bypass the
	// sslmode=disable rejection.
	EnvName string

	// StatementTimeout, when > 0, is applied via the standard `statement_timeout`
	// session setting on every pool connection. This bounds worst-case query
	// duration during cold-start scans so a degraded PG replica cannot hang
	// bootstrap past the readiness gate timeout.
	StatementTimeout time.Duration

	// StreamingBatchSize overrides DefaultStreamingBatchSize when > 0.
	StreamingBatchSize int

	// StreamingWorkers overrides DefaultStreamingWorkers when > 0.
	StreamingWorkers int

	// StreamingMaxRetries overrides DefaultStreamingMaxRetries when > 0.
	StreamingMaxRetries int
}

// NewPostgresLoader creates a PostgresLoader with default pool configuration.
func NewPostgresLoader(ctx context.Context, dsn string, router *shard.Router) (*PostgresLoader, error) {
	return NewPostgresLoaderWithConfig(ctx, dsn, router, PoolConfig{})
}

// NewPostgresLoaderWithConfig creates a PostgresLoader with custom pool configuration.
// It enforces the same sslmode=disable rejection policy that the transaction
// and authorizer config-layer helpers apply to env-var-assembled DSNs, so raw
// DSNs flowing through this path cannot bypass SSL posture.
func NewPostgresLoaderWithConfig(ctx context.Context, dsn string, router *shard.Router, poolConfig PoolConfig) (*PostgresLoader, error) {
	if router == nil {
		router = shard.NewRouter(shard.DefaultShardCount)
	}

	// Defense-in-depth SSL enforcement — reject sslmode=disable before we
	// dial, even if the DSN came from a non-config caller (tests,
	// direct embedders). buildPostgresDSN() already rejects this earlier
	// in the authorizer bootstrap, but this closes the raw-DSN back door
	// flagged by the D1 audit.
	if err := enforceDSNSSLMode(dsn, poolConfig.EnvName); err != nil {
		return nil, err
	}

	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	applyPoolConfig(parsed, poolConfig)

	if poolConfig.StatementTimeout > 0 {
		if parsed.ConnConfig.RuntimeParams == nil {
			parsed.ConnConfig.RuntimeParams = map[string]string{}
		}

		parsed.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(poolConfig.StatementTimeout.Milliseconds(), 10)
	}

	pool, err := pgxpool.NewWithConfig(ctx, parsed)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	// Explicit startup ping (D7). pgxpool.NewWithConfig is lazy and does
	// not establish a physical connection until the first query; a bad
	// DSN, unreachable host, or SSL handshake failure would otherwise
	// surface on the first load — after the readiness gate already
	// flipped green. We bound the check at 30s so a slow DNS/TLS dance
	// cannot stall bootstrap indefinitely.
	const pingTimeout = 30 * time.Second

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if pingErr := pool.Ping(pingCtx); pingErr != nil {
		pool.Close()

		return nil, fmt.Errorf("ping postgres pool: %w", pingErr)
	}

	return &PostgresLoader{pool: pool, router: router, envName: poolConfig.EnvName}, nil
}

// Close releases the underlying connection pool.
func (l *PostgresLoader) Close() {
	if l != nil && l.pool != nil {
		l.pool.Close()
	}
}

// LoadBalances fetches balance rows from PostgreSQL and converts them to engine.Balance values.
//
// Behavior change vs. previous implementation (D1 audit finding #5):
//
//   - 42P01 undefined_table → ErrBalanceTableMissing (was silently (nil, nil)).
//     The previous fail-open behavior caused the authorizer to bootstrap with
//     zero balances when the `balance` table was missing due to a migration
//     gap, making schema drift invisible until the first customer request.
//     We now fail-closed so bootstrap crashes loudly.
//
//   - 53300 too_many_connections / 57014 query_canceled → retried with
//     exponential backoff (cap: DefaultStreamingMaxRetries). These are the
//     transient states that occur during onboarding pool saturation (cross-
//     service exhaustion) and statement_timeout expiry on a slow replica.
//
// LoadBalances is retained for small datasets and test fixtures; production
// bootstrap should prefer LoadBalancesStreaming which streams keyset-paginated
// batches into worker-driven UpsertBalances calls.
func (l *PostgresLoader) LoadBalances(ctx context.Context, organizationID, ledgerID string, shardIDs []int32) ([]*engine.Balance, error) {
	if l == nil || l.pool == nil {
		return nil, constant.ErrPostgresLoaderNotInit
	}

	query, args := buildBalanceQuery(organizationID, ledgerID)

	var balances []*engine.Balance

	err := retryTransient(ctx, DefaultStreamingMaxRetries, func(ctx context.Context) error {
		rows, qErr := l.pool.Query(ctx, query, args...)
		if qErr != nil {
			return classifyQueryError(qErr)
		}
		defer rows.Close()

		shardFilter := buildShardFilter(shardIDs)
		collected := make([]*engine.Balance, 0, initialBalancesCap)

		for rows.Next() {
			balance, scanErr := scanBalance(rows, l.router, shardFilter)
			if scanErr != nil {
				return scanErr
			}

			if balance != nil {
				collected = append(collected, balance)
			}
		}

		if rowsErr := rows.Err(); rowsErr != nil {
			return classifyQueryError(rowsErr)
		}

		balances = collected

		return nil
	})
	if err != nil {
		return nil, err
	}

	return balances, nil
}

// buildShardFilter converts shardIDs into a lookup map for O(1) membership tests.
func buildShardFilter(shardIDs []int32) map[int32]struct{} {
	if len(shardIDs) == 0 {
		return nil
	}

	shardFilter := make(map[int32]struct{}, len(shardIDs))
	for _, shardID := range shardIDs {
		shardFilter[shardID] = struct{}{}
	}

	return shardFilter
}

// classifyQueryError maps raw pgconn errors to either a typed sentinel
// (for fail-closed handling by callers) or a wrapped transient error with
// the original err preserved via %w. Callers use errors.Is(err,
// ErrBalanceTableMissing) to distinguish the fatal missing-table case and
// isTransientPGError(err) to decide whether to retry.
func classifyQueryError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgCodeUndefinedTable:
			return fmt.Errorf("%w: %s", ErrBalanceTableMissing, pgErr.Message)
		case pgCodeTooManyConnections, pgCodeQueryCanceled:
			// Wrap with a transient marker so retryTransient sees it and the
			// underlying pgErr is still matchable via errors.As.
			return &transientError{err: fmt.Errorf("query balances (transient pg %s): %w", pgErr.Code, err)}
		}
	}

	return fmt.Errorf("query balances: %w", err)
}

// transientError marks an error as transient/retryable.
type transientError struct {
	err error
}

func (t *transientError) Error() string { return t.err.Error() }
func (t *transientError) Unwrap() error { return t.err }

// isTransientPGError returns true when err is wrapped in transientError OR
// when the underlying pgconn error code is in the retryable set. The double
// check handles cases where callers wrap transient errors with extra context.
func isTransientPGError(err error) bool {
	if err == nil {
		return false
	}

	var t *transientError
	if errors.As(err, &t) {
		return true
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgCodeTooManyConnections || pgErr.Code == pgCodeQueryCanceled
	}

	return false
}

// retryTransient runs op until it succeeds or until maxAttempts is reached.
// Only errors classified as transient (via isTransientPGError) trigger a
// retry; all other errors — including ErrBalanceTableMissing — are returned
// immediately. Uses a capped exponential backoff so a sustained 53300 storm
// does not extend cold-start latency beyond (maxAttempts * retryBackoffMax).
func retryTransient(ctx context.Context, maxAttempts int, op func(context.Context) error) error {
	if maxAttempts <= 0 {
		maxAttempts = DefaultStreamingMaxRetries
	}

	backoff := retryBackoffInitial

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w", err)
		}

		err := op(ctx)
		if err == nil {
			return nil
		}

		if !isTransientPGError(err) {
			return err
		}

		lastErr = err

		if attempt == maxAttempts-1 {
			break
		}

		// Sleep with context-aware cancellation so a shutting-down process
		// does not get stuck retrying through its grace period.
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w", ctx.Err())
		case <-timer.C:
		}

		backoff *= 2
		if backoff > retryBackoffMax {
			backoff = retryBackoffMax
		}
	}

	return fmt.Errorf("transient pg errors exhausted after %d attempts: %w", maxAttempts, lastErr)
}

// LoadBalancesStreaming streams balance rows from PostgreSQL through a keyset
// paginated query and dispatches them to consumer goroutines that call
// consume (typically engine.UpsertBalances). Replaces the unbounded single
// pool.Query() with bounded-memory cursor pagination (D1 audit finding #3).
//
// Ordering: the query orders by (updated_at, id) so the cursor is stable under
// concurrent writes. The updated_at filter uses the >= start bound to include
// rows touched at exactly snapshotTime (useful when wiring to a snapshot +
// delta bootstrap pattern later).
//
// Concurrency: one producer goroutine drains batches from PG; N consumer
// goroutines run consume in parallel. consume MUST be safe for concurrent
// invocation — engine.UpsertBalances is, because it takes per-shard locks.
//
// Back-pressure: the balance channel is buffered at DefaultStreamingChanSize
// so the producer can prefetch one full batch ahead of the slowest consumer.
//
// The returned count is the total number of rows streamed (not just those
// that passed the shard filter; filtered rows are accounted as skipped and
// excluded from the count). On error, in-flight consumers are canceled and
// the count reflects only successfully-consumed batches.
func (l *PostgresLoader) LoadBalancesStreaming(
	ctx context.Context,
	organizationID, ledgerID string,
	shardIDs []int32,
	updatedAtStart time.Time,
	consume func(batch []*engine.Balance) error,
) (int64, error) {
	if l == nil || l.pool == nil {
		return 0, constant.ErrPostgresLoaderNotInit
	}

	if consume == nil {
		return 0, fmt.Errorf("%w", constant.ErrLoaderConsumeRequired)
	}

	cfg := streamingTunables{
		batchSize:   DefaultStreamingBatchSize,
		workers:     DefaultStreamingWorkers,
		maxRetries:  DefaultStreamingMaxRetries,
		chanBufSize: DefaultStreamingChanSize,
	}

	shardFilter := buildShardFilter(shardIDs)

	return l.streamBalances(ctx, organizationID, ledgerID, shardFilter, updatedAtStart, cfg, consume)
}

// streamingTunables consolidates streaming parameters so streamBalances has a
// manageable signature. All fields are positive integers at call time —
// defaults are applied by LoadBalancesStreaming before invocation.
type streamingTunables struct {
	batchSize   int
	workers     int
	maxRetries  int
	chanBufSize int
}

func (l *PostgresLoader) streamBalances(
	ctx context.Context,
	organizationID, ledgerID string,
	shardFilter map[int32]struct{},
	updatedAtStart time.Time,
	cfg streamingTunables,
	consume func(batch []*engine.Balance) error,
) (int64, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	batchCh := make(chan []*engine.Balance, cfg.workers)

	var (
		consumeErr   error
		consumeErrMu sync.Mutex
		consumeWG    sync.WaitGroup
	)

	setConsumeErr := func(err error) {
		consumeErrMu.Lock()
		defer consumeErrMu.Unlock()

		if consumeErr == nil {
			consumeErr = err

			cancel()
		}
	}

	for i := 0; i < cfg.workers; i++ {
		consumeWG.Add(1)

		go func() {
			defer consumeWG.Done()

			for batch := range batchCh {
				if err := consume(batch); err != nil {
					setConsumeErr(fmt.Errorf("consume batch: %w", err))
					return
				}
			}
		}()
	}

	produced, produceErr := l.produceBatches(streamCtx, organizationID, ledgerID, shardFilter, updatedAtStart, cfg, batchCh)

	close(batchCh)
	consumeWG.Wait()

	consumeErrMu.Lock()
	cErr := consumeErr
	consumeErrMu.Unlock()

	if cErr != nil {
		return produced, cErr
	}

	if produceErr != nil {
		return produced, produceErr
	}

	return produced, nil
}

func (l *PostgresLoader) produceBatches(
	ctx context.Context,
	organizationID, ledgerID string,
	shardFilter map[int32]struct{},
	updatedAtStart time.Time,
	cfg streamingTunables,
	batchCh chan<- []*engine.Balance,
) (int64, error) {
	var (
		lastUpdatedAt = updatedAtStart
		lastID        string
		total         int64
		firstPage     = true
	)

	for {
		if err := ctx.Err(); err != nil {
			return total, fmt.Errorf("%w", err)
		}

		var page []cursorRow

		err := retryTransient(ctx, cfg.maxRetries, func(ctx context.Context) error {
			p, qErr := l.queryKeysetPage(ctx, organizationID, ledgerID, shardFilter, lastUpdatedAt, lastID, cfg.batchSize, firstPage)
			if qErr != nil {
				return qErr
			}

			page = p

			return nil
		})
		if err != nil {
			return total, err
		}

		firstPage = false

		if len(page) == 0 {
			return total, nil
		}

		// Advance the cursor to the last row of the page; the next query
		// uses strict > comparison on (updated_at, id) to skip what we just saw.
		// Pagination cursor must advance regardless of shard filter — shard
		// filtering is applied during scan (rows outside the filter are
		// returned as sentinel `cursorOnly` entries so the cursor moves
		// without those balances reaching consumers).
		last := page[len(page)-1]
		lastUpdatedAt = last.cursorUpdatedAt
		lastID = last.cursorID

		// Collect non-filtered balances into a fresh slice for consumers.
		// The sentinel-marker balances (shard-filtered) are dropped here.
		filtered := collectBalances(page)
		total += int64(len(filtered))

		if len(filtered) == 0 {
			continue
		}

		select {
		case batchCh <- filtered:
		case <-ctx.Done():
			return total, fmt.Errorf("%w", ctx.Err())
		}
	}
}

// cursorRow bundles a scanned balance row with its (updated_at, id) cursor
// coordinates so pagination can advance even when the row is filtered out by
// the shard allow-list.
type cursorRow struct {
	balance         *engine.Balance
	cursorUpdatedAt time.Time
	cursorID        string
}

// collectBalances extracts the non-nil balance pointers from rows into a fresh
// slice, preserving scan order. Rows where balance is nil (shard-filtered)
// are dropped.
func collectBalances(rows []cursorRow) []*engine.Balance {
	out := make([]*engine.Balance, 0, len(rows))

	for _, r := range rows {
		if r.balance != nil {
			out = append(out, r.balance)
		}
	}

	return out
}

// buildBalanceQuery constructs the query and args for loading balances.
func buildBalanceQuery(organizationID, ledgerID string) (string, []any) {
	query := queryBalancesBase
	args := make([]any, 0, initialArgsCap)

	if organizationID != "" {
		args = append(args, organizationID)
		query += " AND organization_id = $" + strconv.Itoa(len(args))
	}

	if ledgerID != "" {
		args = append(args, ledgerID)
		query += " AND ledger_id = $" + strconv.Itoa(len(args))
	}

	return query, args
}

// scanBalance reads a single balance row and returns an engine.Balance.
// Returns nil (without error) when the row's shard is filtered out.
func scanBalance(rows interface {
	Scan(dest ...any) error
}, router *shard.Router, shardFilter map[int32]struct{},
) (*engine.Balance, error) {
	var (
		id         string
		org        string
		ledger     string
		accountID  string
		alias      string
		balanceKey string
		assetCode  string
		// availableRaw and onHoldRaw are scanned as *string so NULL values
		// from the decimal columns do not silently become "" (which then
		// panics/errors in decimal.NewFromString). A NULL is coerced to "0"
		// below, matching the on-disk semantics of a zero-balance row.
		availableRaw *string
		onHoldRaw    *string
		version      int64
		accountType  string
		// allow_sending/allow_receiving are scanned as sql.NullBool because a
		// migration gap (pre-000003/000020) or manual back-fill could leave
		// NULL even though current schema is NOT NULL. A NULL defaults to
		// true — the fail-safe for a policy column: do NOT silently lock an
		// account out of sending/receiving because of unexpected NULL.
		allowSending   sql.NullBool
		allowReceiving sql.NullBool
	)

	if err := rows.Scan(
		&id,
		&org,
		&ledger,
		&accountID,
		&alias,
		&balanceKey,
		&assetCode,
		&availableRaw,
		&onHoldRaw,
		&version,
		&accountType,
		&allowSending,
		&allowReceiving,
	); err != nil {
		return nil, fmt.Errorf("scan balance row: %w", err)
	}

	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	resolvedShardID := router.ResolveBalance(alias, balanceKey)
	if len(shardFilter) > 0 {
		if _, ok := shardFilter[int32(resolvedShardID)]; !ok { //nolint:gosec // shard IDs are small positive ints
			return nil, nil
		}
	}

	return buildBalance(id, org, ledger, accountID, alias, balanceKey, assetCode,
		coalesceDecimalString(availableRaw),
		coalesceDecimalString(onHoldRaw),
		version, accountType,
		coalesceAllowBool(allowSending),
		coalesceAllowBool(allowReceiving),
	)
}

// coalesceDecimalString returns the string value pointed to by p or "0" when
// p is nil (NULL in the underlying column). "0" is the semantically correct
// default for a missing decimal amount and parses cleanly via
// decimal.NewFromString, preventing the previous NULL→""→parse-error panic.
func coalesceDecimalString(p *string) string {
	if p == nil {
		return "0"
	}

	return *p
}

// coalesceAllowBool returns nb.Bool when valid, or true otherwise. true is the
// fail-safe default for the allow_sending/allow_receiving policy flags: a
// NULL (schema drift, manual backfill) must not silently revoke an account's
// ability to transact. Operators can always flip the flag to false explicitly.
func coalesceAllowBool(nb sql.NullBool) bool {
	if !nb.Valid {
		return true
	}

	return nb.Bool
}

// buildBalance converts raw scanned values to an engine.Balance.
func buildBalance(
	id, org, ledger, accountID, alias, balanceKey, assetCode, availableRaw, onHoldRaw string,
	version int64, accountType string, allowSending, allowReceiving bool,
) (*engine.Balance, error) {
	availableDecimal, err := decimal.NewFromString(availableRaw)
	if err != nil {
		return nil, fmt.Errorf("parse available for balance %s: %w", id, err)
	}

	onHoldDecimal, err := decimal.NewFromString(onHoldRaw)
	if err != nil {
		return nil, fmt.Errorf("parse on_hold for balance %s: %w", id, err)
	}

	scale := pkgTransaction.MaxScale(availableDecimal, onHoldDecimal)
	if scale < pkgTransaction.DefaultScale {
		scale = pkgTransaction.DefaultScale
	}

	availableInt, err := pkgTransaction.ScaleToInt(availableDecimal, scale)
	if err != nil {
		return nil, fmt.Errorf("scale available for balance %s: %w", id, err)
	}

	onHoldInt, err := pkgTransaction.ScaleToInt(onHoldDecimal, scale)
	if err != nil {
		return nil, fmt.Errorf("scale on_hold for balance %s: %w", id, err)
	}

	if version < 0 {
		version = 0
	}

	return &engine.Balance{
		ID:             id,
		OrganizationID: org,
		LedgerID:       ledger,
		AccountID:      accountID,
		AccountAlias:   alias,
		BalanceKey:     balanceKey,
		AssetCode:      assetCode,
		Available:      availableInt,
		OnHold:         onHoldInt,
		Scale:          scale,
		Version:        uint64(version),
		AccountType:    accountType,
		IsExternal:     strings.EqualFold(accountType, constant.ExternalAccountType),
		AllowSending:   allowSending,
		AllowReceiving: allowReceiving,
	}, nil
}

func applyPoolConfig(config *pgxpool.Config, poolConfig PoolConfig) {
	if config == nil {
		return
	}

	if poolConfig.MaxConns > 0 {
		config.MaxConns = poolConfig.MaxConns
	}

	if poolConfig.MinConns > 0 {
		config.MinConns = poolConfig.MinConns
	}

	if config.MaxConns > 0 && config.MinConns > config.MaxConns {
		config.MinConns = config.MaxConns
	}

	if poolConfig.MaxConnLifetime > 0 {
		config.MaxConnLifetime = poolConfig.MaxConnLifetime
	}

	if poolConfig.MaxConnIdleTime > 0 {
		config.MaxConnIdleTime = poolConfig.MaxConnIdleTime
	}

	if poolConfig.HealthCheckPeriod > 0 {
		config.HealthCheckPeriod = poolConfig.HealthCheckPeriod
	}

	if poolConfig.ConnectTimeout > 0 {
		config.ConnConfig.ConnectTimeout = poolConfig.ConnectTimeout
	}
}

// queryKeysetPage runs one page of the keyset-paginated streaming query.
// cursorUpdatedAt/cursorID are zero on the first page (firstPage=true) and
// advance across subsequent pages. The query uses strict > semantics on the
// composite cursor: (updated_at, id) > ($cursorUpdatedAt, $cursorID).
//
// NOTE: shardFilter is accepted but NOT applied inside the SQL — filter
// evaluation happens during scan. This keeps the query planner-friendly
// (no IN-list expansion) and keeps pagination cursors valid.
func (l *PostgresLoader) queryKeysetPage(
	ctx context.Context,
	organizationID, ledgerID string,
	shardFilter map[int32]struct{},
	cursorUpdatedAt time.Time,
	cursorID string,
	limit int,
	firstPage bool,
) ([]cursorRow, error) {
	query, args := buildStreamingQuery(organizationID, ledgerID, cursorUpdatedAt, cursorID, limit, firstPage)

	rows, err := l.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, classifyQueryError(err)
	}
	defer rows.Close()

	page := make([]cursorRow, 0, limit)

	for rows.Next() {
		row, scanErr := scanStreamingBalance(rows, l.router, shardFilter)
		if scanErr != nil {
			return nil, scanErr
		}

		page = append(page, row)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, classifyQueryError(rowsErr)
	}

	return page, nil
}

// buildStreamingQuery constructs the keyset-paginated query and args.
// The cursor predicate uses the standard row-values comparison
//
//	(updated_at, id) > ($n, $m)
//
// which PostgreSQL evaluates lexicographically and can use a composite index.
// firstPage suppresses the cursor predicate so we fetch the smallest page
// without relying on a sentinel timestamp that some databases reject.
// streamingArgsExtras accounts for the additional query args that streaming
// adds over buildBalanceQuery: cursor_updated_at, cursor_id, limit — at most
// three beyond the filter args.
const streamingArgsExtras = 3

func buildStreamingQuery(organizationID, ledgerID string, cursorUpdatedAt time.Time, cursorID string, limit int, firstPage bool) (string, []any) {
	query := queryBalancesStreamingBase
	args := make([]any, 0, initialArgsCap+streamingArgsExtras)

	if organizationID != "" {
		args = append(args, organizationID)
		query += " AND organization_id = $" + strconv.Itoa(len(args))
	}

	if ledgerID != "" {
		args = append(args, ledgerID)
		query += " AND ledger_id = $" + strconv.Itoa(len(args))
	}

	// On first page, start from the earliest updated_at we care about
	// (zero time => all rows). On subsequent pages, use the strict
	// composite-cursor predicate so we never re-fetch what we already saw.
	if firstPage {
		if !cursorUpdatedAt.IsZero() {
			args = append(args, cursorUpdatedAt)
			query += " AND COALESCE(updated_at, created_at) >= $" + strconv.Itoa(len(args))
		}
	} else {
		args = append(args, cursorUpdatedAt)
		updatedAtIdx := strconv.Itoa(len(args))

		args = append(args, cursorID)
		idIdx := strconv.Itoa(len(args))

		query += " AND (COALESCE(updated_at, created_at), id) > ($" + updatedAtIdx + ", $" + idIdx + ")"
	}

	query += " ORDER BY COALESCE(updated_at, created_at), id"

	args = append(args, limit)
	query += " LIMIT $" + strconv.Itoa(len(args))

	return query, args
}

// scanStreamingBalance reads one balance row (including updated_at cursor
// column) and returns a cursorRow. Shard-filtered rows still populate the
// cursor fields so pagination can advance; their balance pointer is nil.
func scanStreamingBalance(rows interface {
	Scan(dest ...any) error
}, router *shard.Router, shardFilter map[int32]struct{},
) (cursorRow, error) {
	var (
		id         string
		org        string
		ledger     string
		accountID  string
		alias      string
		balanceKey string
		assetCode  string
		// See scanBalance for the rationale behind *string / sql.NullBool
		// handling of the decimal and policy columns.
		availableRaw   *string
		onHoldRaw      *string
		version        int64
		accountType    string
		allowSending   sql.NullBool
		allowReceiving sql.NullBool
		updatedAt      time.Time
	)

	if err := rows.Scan(
		&id,
		&org,
		&ledger,
		&accountID,
		&alias,
		&balanceKey,
		&assetCode,
		&availableRaw,
		&onHoldRaw,
		&version,
		&accountType,
		&allowSending,
		&allowReceiving,
		&updatedAt,
	); err != nil {
		return cursorRow{}, fmt.Errorf("scan streaming balance row: %w", err)
	}

	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	out := cursorRow{cursorUpdatedAt: updatedAt, cursorID: id}

	resolvedShardID := router.ResolveBalance(alias, balanceKey)
	if len(shardFilter) > 0 {
		if _, ok := shardFilter[int32(resolvedShardID)]; !ok { //nolint:gosec // shard IDs are small positive ints
			return out, nil // cursor advances; balance dropped
		}
	}

	balance, err := buildBalance(id, org, ledger, accountID, alias, balanceKey, assetCode,
		coalesceDecimalString(availableRaw),
		coalesceDecimalString(onHoldRaw),
		version, accountType,
		coalesceAllowBool(allowSending),
		coalesceAllowBool(allowReceiving),
	)
	if err != nil {
		return cursorRow{}, err
	}

	out.balance = balance

	return out, nil
}
