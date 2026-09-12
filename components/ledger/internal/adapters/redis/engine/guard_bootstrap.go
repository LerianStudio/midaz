// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

// EnsureTransactionGuard conditionally seeds a transaction guard without
// changing an existing lifecycle token. It does not mutate balances, receipts,
// recovery records, or key expiry.
func (a *Adapter) EnsureTransactionGuard(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, nextToken string) error {
	if err := ctx.Err(); err != nil {
		return technical("context_canceled", false, err)
	}

	if organizationID == uuid.Nil || ledgerID == uuid.Nil || transactionID == uuid.Nil || nextToken == "" || !utf8.ValidString(nextToken) || len(nextToken) > a.limits.MaxPreparedBytes {
		return technical("invalid_request", false, errors.New("transaction guard bootstrap requires nonzero scope UUIDs and a valid token"))
	}

	key, err := resolveGuardKey(ctx, organizationID, ledgerID)
	if err != nil {
		return technical("invalid_request", false, err)
	}

	shared, err := a.provider.GetClient(ctx)
	if err != nil {
		return technical("connection_unavailable", false, err)
	}

	client, ok := shared.(*redis.Client)
	if !ok || client == nil {
		return technical("unsupported_transport", false, errors.New("transaction guard bootstrap requires a standalone or Sentinel Redis client"))
	}

	if err := ctx.Err(); err != nil {
		return technical("context_canceled", false, err)
	}

	if _, err := processNoRetry(ctx, client, "hsetnx", key, transactionID.String(), nextToken).Bool(); err != nil {
		return technical("transport", true, fmt.Errorf("ensure transaction guard: %w", err))
	}

	return nil
}

func resolveGuardKey(ctx context.Context, organizationID, ledgerID uuid.UUID) (string, error) {
	scope := organizationID.String() + ":" + ledgerID.String()

	return tmvalkey.GetKeyContext(ctx, "engine:"+cachepolicy.HashTag+":guards:"+scope)
}
