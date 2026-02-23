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

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

const queryBalancesBase = `
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

// PostgresLoader loads balance state from PostgreSQL into the in-memory authorizer.
type PostgresLoader struct {
	pool   *pgxpool.Pool
	router *shard.Router
}

type PoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
}

func NewPostgresLoader(ctx context.Context, dsn string, router *shard.Router) (*PostgresLoader, error) {
	return NewPostgresLoaderWithConfig(ctx, dsn, router, PoolConfig{})
}

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

func (l *PostgresLoader) Close() {
	if l != nil && l.pool != nil {
		l.pool.Close()
	}
}

func (l *PostgresLoader) LoadBalances(ctx context.Context, organizationID, ledgerID string, shardIDs []int32) ([]*engine.Balance, error) {
	if l == nil || l.pool == nil {
		return nil, fmt.Errorf("postgres loader is not initialized")
	}

	query := queryBalancesBase
	args := make([]any, 0, 2)

	if organizationID != "" {
		args = append(args, organizationID)
		query += " AND organization_id = $" + strconv.Itoa(len(args))
	}

	if ledgerID != "" {
		args = append(args, ledgerID)
		query += " AND ledger_id = $" + strconv.Itoa(len(args))
	}

	rows, err := l.pool.Query(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			return nil, nil
		}

		return nil, err
	}
	defer rows.Close()

	shardFilter := make(map[int32]struct{}, len(shardIDs))
	for _, shardID := range shardIDs {
		shardFilter[shardID] = struct{}{}
	}

	balances := make([]*engine.Balance, 0, 1024)

	for rows.Next() {
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
			return nil, err
		}

		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		resolvedShardID := int32(l.router.ResolveBalance(alias, balanceKey))
		if len(shardFilter) > 0 {
			if _, ok := shardFilter[resolvedShardID]; !ok {
				continue
			}
		}

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
			return nil, err
		}

		onHoldInt, err := pkgTransaction.ScaleToInt(onHoldDecimal, scale)
		if err != nil {
			return nil, err
		}

		if version < 0 {
			version = 0
		}

		balances = append(balances, &engine.Balance{
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
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return balances, nil
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
