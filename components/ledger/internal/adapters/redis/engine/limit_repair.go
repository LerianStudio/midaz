// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

//go:embed scripts/normalize_balance_limits.lua
var normalizeBalanceLimitsScriptRaw string

var normalizeBalanceLimitsScript = redis.NewScript(normalizeBalanceLimitsScriptRaw)

const (
	repairCASMismatch  int64 = 0
	repairCASSuccess   int64 = 1
	repairCASMissing   int64 = -1
	repairCASWrongType int64 = -2
)

func repairBalanceLimits(ctx context.Context, client *redis.Client, keys []string, request accounting.Execution) error {
	if err := ctx.Err(); err != nil {
		return technical("context_canceled", false, err)
	}

	resolved, err := resolveAdapterKeys(ctx, request)
	if err != nil {
		return technical("invalid_scope", false, err)
	}

	aliases := make(map[string]string, len(request.Balances))
	for _, balance := range request.Balances {
		aliases[resolved.Balances[balance.BalanceRef].Balance] = balance.Alias
	}

	values, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		return technical("normalization_read_failed", false, fmt.Errorf("read cached balances for limit normalization: %w", err))
	}

	args := make([]any, 0, len(keys)*2)

	for i, value := range values {
		raw, ok := value.(string)
		if !ok {
			if value == nil {
				return technical("normalization_balance_missing", false, fmt.Errorf("cached balance %s disappeared before limit normalization", keys[i]))
			}

			return technical("normalization_invalid_balance", false, fmt.Errorf("cached balance %s is not a string", keys[i]))
		}

		replacement, repairErr := balancecache.NormalizeLimitDual([]byte(raw), aliases[keys[i]])
		if repairErr != nil {
			return technical("normalization_invalid_balance", false, fmt.Errorf("validate cached balance %s for limit normalization: %w", keys[i], repairErr))
		}

		args = append(args, raw, string(replacement))
	}

	if err := ctx.Err(); err != nil {
		return technical("context_canceled", false, err)
	}

	evalSHAArgs := make([]any, 0, 3+len(keys)+len(args))

	evalSHAArgs = append(evalSHAArgs, "evalsha", normalizeBalanceLimitsScript.Hash(), len(keys))
	for _, key := range keys {
		evalSHAArgs = append(evalSHAArgs, key)
	}

	evalSHAArgs = append(evalSHAArgs, args...)

	response, err := processNoRetry(ctx, client, evalSHAArgs...).Result()
	if isNoScript(err) {
		evalArgs := make([]any, 0, 3+len(keys)+len(args))

		evalArgs = append(evalArgs, "eval", normalizeBalanceLimitsScriptRaw, len(keys))
		for _, key := range keys {
			evalArgs = append(evalArgs, key)
		}

		evalArgs = append(evalArgs, args...)
		response, err = processNoRetry(ctx, client, evalArgs...).Result()
	}

	if err != nil {
		return technical("normalization_repair_failed", true, fmt.Errorf("conditionally normalize cached balance limits: %w", err))
	}

	status, ok := response.(int64)
	if !ok {
		return technical("normalization_repair_failed", true, errors.New("unexpected balance limit normalization response"))
	}

	switch status {
	case repairCASSuccess, repairCASMismatch:
		return nil
	case repairCASMissing:
		return technical("normalization_balance_missing", false, errors.New("cached balance disappeared during limit normalization"))
	case repairCASWrongType:
		return technical("normalization_invalid_balance", false, errors.New("cached balance changed type during limit normalization"))
	default:
		return technical("normalization_repair_failed", true, errors.New("invalid balance limit normalization status"))
	}
}
