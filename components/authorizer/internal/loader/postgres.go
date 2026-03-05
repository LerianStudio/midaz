// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

	// initialArgsCap is the initial capacity for query arguments.
	initialArgsCap = 2

	// initialBalancesCap is the initial capacity for the loaded balances slice.
	initialBalancesCap = 1024
)

// PostgresLoader loads balance state from PostgreSQL into the in-memory authorizer.
type PostgresLoader struct {
	pool   *pgxpool.Pool
	router *shard.Router
}

// PoolConfig holds connection pool tuning parameters for the PostgreSQL loader.
type PoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
}

// NewPostgresLoader creates a PostgresLoader with default pool configuration.
func NewPostgresLoader(ctx context.Context, dsn string, router *shard.Router) (*PostgresLoader, error) {
	return NewPostgresLoaderWithConfig(ctx, dsn, router, PoolConfig{})
}

// NewPostgresLoaderWithConfig creates a PostgresLoader with custom pool configuration.
func NewPostgresLoaderWithConfig(ctx context.Context, dsn string, router *shard.Router, poolConfig PoolConfig) (*PostgresLoader, error) {
	if router == nil {
		router = shard.NewRouter(shard.DefaultShardCount)
	}

	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	applyPoolConfig(parsed, poolConfig)

	pool, err := pgxpool.NewWithConfig(ctx, parsed)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	return &PostgresLoader{pool: pool, router: router}, nil
}

// Close releases the underlying connection pool.
func (l *PostgresLoader) Close() {
	if l != nil && l.pool != nil {
		l.pool.Close()
	}
}

// LoadBalances fetches balance rows from PostgreSQL and converts them to engine.Balance values.
func (l *PostgresLoader) LoadBalances(ctx context.Context, organizationID, ledgerID string, shardIDs []int32) ([]*engine.Balance, error) {
	if l == nil || l.pool == nil {
		return nil, constant.ErrPostgresLoaderNotInit
	}

	query, args := buildBalanceQuery(organizationID, ledgerID)

	rows, err := l.pool.Query(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			return nil, nil
		}

		return nil, fmt.Errorf("query balances: %w", err)
	}
	defer rows.Close()

	shardFilter := make(map[int32]struct{}, len(shardIDs))
	for _, shardID := range shardIDs {
		shardFilter[shardID] = struct{}{}
	}

	balances := make([]*engine.Balance, 0, initialBalancesCap)

	for rows.Next() {
		balance, scanErr := scanBalance(rows, l.router, shardFilter)
		if scanErr != nil {
			return nil, scanErr
		}

		if balance != nil {
			balances = append(balances, balance)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate balance rows: %w", err)
	}

	return balances, nil
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
		id             string
		org            string
		ledger         string
		accountID      string
		alias          string
		balanceKey     string
		assetCode      string
		availableRaw   string
		onHoldRaw      string
		version        int64
		accountType    string
		allowSending   bool
		allowReceiving bool
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

	return buildBalance(id, org, ledger, accountID, alias, balanceKey, assetCode, availableRaw, onHoldRaw, version, accountType, allowSending, allowReceiving)
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
