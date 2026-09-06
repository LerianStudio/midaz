// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
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

type jsonValueSpan struct {
	start int
	end   int
}

func repairBalanceLimits(ctx context.Context, client *redis.Client, keys []string) error {
	if err := ctx.Err(); err != nil {
		return technical("context_canceled", false, err)
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

		replacement, repairErr := normalizedLimitBlob([]byte(raw))
		if repairErr != nil {
			return technical("normalization_invalid_balance", false, fmt.Errorf("validate cached balance %s for limit normalization: %w", keys[i], repairErr))
		}

		args = append(args, raw, string(replacement))
	}

	if err := ctx.Err(); err != nil {
		return technical("context_canceled", false, err)
	}

	response, err := normalizeBalanceLimitsScript.EvalSha(ctx, client, keys, args...).Result()
	if isNoScript(err) {
		response, err = normalizeBalanceLimitsScript.Eval(ctx, client, keys, args...).Result()
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

func normalizedLimitBlob(raw []byte) ([]byte, error) {
	_, err := balancecache.Decode(raw)
	if err == nil {
		return raw, nil
	}

	var noncanonical *balancecache.NoncanonicalLimitError
	if !errors.As(err, &noncanonical) {
		return nil, err
	}

	parsed, err := decimal.NewFromString(noncanonical.Raw)
	if err != nil || parsed.String() != noncanonical.Canonical {
		return nil, errors.New("invalid noncanonical overdraft limit")
	}

	span, err := authoritativeLimitSpan(raw)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(noncanonical.Canonical)
	if err != nil {
		return nil, fmt.Errorf("encode canonical overdraft limit: %w", err)
	}

	replacement := make([]byte, 0, len(raw)-span.end+span.start+len(encoded))
	replacement = append(replacement, raw[:span.start]...)
	replacement = append(replacement, encoded...)
	replacement = append(replacement, raw[span.end:]...)

	if _, err := balancecache.Decode(replacement); err != nil {
		return nil, fmt.Errorf("validate normalized cached balance: %w", err)
	}

	return replacement, nil
}

func authoritativeLimitSpan(raw []byte) (jsonValueSpan, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return jsonValueSpan{}, errors.New("cached balance must be a JSON object")
	}

	var legacy, current *jsonValueSpan

	for decoder.More() {
		fieldToken, err := decoder.Token()
		if err != nil {
			return jsonValueSpan{}, fmt.Errorf("read cached balance field: %w", err)
		}

		name, ok := fieldToken.(string)
		if !ok {
			return jsonValueSpan{}, errors.New("invalid cached balance field name")
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return jsonValueSpan{}, fmt.Errorf("decode cached balance field %s: %w", name, err)
		}

		end := int(decoder.InputOffset())
		start := end - len(value)

		if start < 0 || !bytes.Equal(raw[start:end], value) {
			return jsonValueSpan{}, fmt.Errorf("locate cached balance field %s", name)
		}

		span := &jsonValueSpan{start: start, end: end}

		switch name {
		case "OverdraftLimit":
			if legacy != nil {
				return jsonValueSpan{}, errors.New("duplicate cached balance field OverdraftLimit")
			}

			legacy = span
		case "overdraftLimit":
			if current != nil {
				return jsonValueSpan{}, errors.New("duplicate cached balance field overdraftLimit")
			}

			current = span
		}
	}

	if _, err := decoder.Token(); err != nil {
		return jsonValueSpan{}, fmt.Errorf("close cached balance object: %w", err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return jsonValueSpan{}, errors.New("unexpected trailing cached balance content")
	}

	if legacy != nil {
		return *legacy, nil
	}

	if current != nil {
		return *current, nil
	}

	return jsonValueSpan{}, errors.New("cached balance has no overdraft limit")
}
