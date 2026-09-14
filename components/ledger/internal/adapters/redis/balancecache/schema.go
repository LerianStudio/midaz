// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

const (
	// TTL is the balance cache lifetime, independent of execution receipt retention.
	TTL = cachepolicy.BalanceTTL
	// HashTag keeps balance mutations in the existing transaction hash slot.
	HashTag = cachepolicy.HashTag
	// DeletionMarkerSuffix identifies the guard adjacent to a balance cache key.
	DeletionMarkerSuffix = cachepolicy.DeletionMarkerSuffix
)

// Format explicitly selects the fields emitted by a writer.
type Format uint8

const (
	// FormatDual writes both field representations for compatible readers.
	FormatDual Format = iota + 1
	// FormatNewOnly writes only the version-two field representation.
	FormatNewOnly
)

var fieldNames = []string{
	"ID", "AccountID", "AccountType", "AssetCode", "Alias", "Key", "Direction", "BalanceScope",
	"Available", "OnHold", "OverdraftUsed", "Version", "AllowSending", "AllowReceiving", "Blocked",
	"AllowOverdraft", "OverdraftLimitEnabled", "OverdraftLimit",
}

func lowerName(name string) string {
	if name == "ID" {
		return "id"
	}

	if name == "AccountID" {
		return "accountId"
	}

	return strings.ToLower(name[:1]) + name[1:]
}

func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("cached balance must be a JSON object")
	}

	fields := make(map[string]json.RawMessage, len(fieldNames)*2+1)

	for decoder.More() {
		fieldToken, tokenErr := decoder.Token()
		if tokenErr != nil {
			return nil, fmt.Errorf("read cached balance field: %w", tokenErr)
		}

		name, ok := fieldToken.(string)
		if !ok {
			return nil, errors.New("invalid cached balance field name")
		}

		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("duplicate cached balance field %s", name)
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode cached balance field %s: %w", name, err)
		}

		fields[name] = value
	}

	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("close cached balance object: %w", err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected trailing cached balance content")
	}

	return fields, nil
}

func authoritativeField(fields map[string]json.RawMessage, name string) (json.RawMessage, bool, bool) {
	if raw, exists := fields[name]; exists {
		return raw, true, true
	}

	raw, exists := fields[lowerName(name)]

	return raw, exists, false
}

func dualTextField(name string, raw json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	var value string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
		return nil, nil, fmt.Errorf("invalid cached balance field %s", name)
	}

	return raw, raw, nil
}

func dualFlagField(name string, raw json.RawMessage, numeric bool) (json.RawMessage, json.RawMessage, error) {
	var enabled bool

	if numeric {
		switch string(raw) {
		case "0":
			enabled = false
		case "1":
			enabled = true
		default:
			return nil, nil, fmt.Errorf("invalid cached balance field %s", name)
		}
	} else {
		switch string(raw) {
		case "false":
			enabled = false
		case "true":
			enabled = true
		default:
			return nil, nil, fmt.Errorf("invalid cached balance field %s", name)
		}
	}

	legacy := json.RawMessage("0")
	modern := json.RawMessage("false")

	if enabled {
		legacy = json.RawMessage("1")
		modern = json.RawMessage("true")
	}

	return legacy, modern, nil
}

func dualMoneyField(name string, raw json.RawMessage, legacyRepresentation bool) (json.RawMessage, json.RawMessage, error) {
	value, validRepresentation := moneyText(raw, legacyRepresentation)
	if !validRepresentation {
		return nil, nil, fmt.Errorf("invalid cached balance field %s", name)
	}

	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cached balance field %s", name)
	}

	canonical := parsed.String()
	if !legacyRepresentation && canonical != value {
		return nil, nil, fmt.Errorf("invalid cached balance field %s", name)
	}

	modern, err := json.Marshal(canonical)
	if err != nil {
		return nil, nil, fmt.Errorf("encode cached balance field %s: %w", name, err)
	}

	if legacyRepresentation {
		return raw, modern, nil
	}

	return modern, modern, nil
}

func dualVersionField(raw json.RawMessage, numeric bool) (json.RawMessage, json.RawMessage, error) {
	text := string(raw)

	if !numeric {
		var value string
		if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
			return nil, nil, errors.New("invalid cached balance field Version")
		}

		text = value
	}

	version, err := strconv.ParseInt(text, 10, 64)
	if err != nil || version < 0 || strconv.FormatInt(version, 10) != text {
		return nil, nil, errors.New("invalid cached balance field Version")
	}

	legacy := json.RawMessage(strconv.FormatInt(version, 10))

	modern, err := json.Marshal(strconv.FormatInt(version, 10))
	if err != nil {
		return nil, nil, fmt.Errorf("encode cached balance field Version: %w", err)
	}

	return legacy, modern, nil
}
