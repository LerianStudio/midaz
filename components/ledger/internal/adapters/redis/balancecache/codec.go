// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package balancecache defines lossless balance cache representations.
package balancecache

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
)

// NoncanonicalLimitError requires an explicit conditional repair of a live limit.
// Decode never returns a usable snapshot alongside this error.
type NoncanonicalLimitError struct {
	Raw       string
	Canonical string
}

func (e *NoncanonicalLimitError) Error() string {
	return "noncanonical cached overdraft limit"
}

type fieldReader struct {
	fields                 map[string]json.RawMessage
	err                    error
	allowNoncanonicalLimit bool
	legacyReadShape        bool
}

func (r *fieldReader) value(name string) (json.RawMessage, bool, bool) {
	if raw, exists := r.fields[name]; exists {
		return raw, true, true
	}

	raw, exists := r.fields[lowerName(name)]

	return raw, exists, false
}

func (r *fieldReader) invalid(name string) {
	if r.err == nil {
		r.err = fmt.Errorf("invalid cached balance field %s", name)
	}
}

func (r *fieldReader) text(name, fallback string, optional bool) string {
	raw, exists, _ := r.value(name)
	if !exists && optional {
		return fallback
	}

	var value string
	if !exists || len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
		r.invalid(name)
	}

	return value
}

func (r *fieldReader) flag(name string, optional bool) bool {
	raw, exists, legacy := r.value(name)
	if !exists && optional {
		return false
	}

	if legacy || r.legacyReadShape {
		if string(raw) == "0" || string(raw) == "1" {
			return string(raw) == "1"
		}
	} else if string(raw) == "true" || string(raw) == "false" {
		return string(raw) == "true"
	}

	r.invalid(name)

	return false
}

func (r *fieldReader) money(name string, optional bool) decimal.Decimal {
	raw, exists, uppercase := r.value(name)
	if !exists && optional {
		return decimal.Zero
	}

	legacyRepresentation := r.acceptsLegacyMoneyRepresentation(name, uppercase)

	value, validRepresentation := moneyText(raw, legacyRepresentation)
	if !exists || !validRepresentation {
		r.invalid(name)
		return decimal.Zero
	}

	if r.legacyReadShape && !uppercase && (name == "OverdraftUsed" || name == "OverdraftLimit") && value == "" {
		return decimal.Zero
	}

	parsed, err := decimal.NewFromString(value)
	if err != nil {
		r.invalid(name)
		return decimal.Zero
	}

	if parsed.String() != value {
		if legacyRepresentation || name == "OverdraftLimit" && r.allowNoncanonicalLimit {
			return parsed
		}

		if name == "OverdraftLimit" && r.err == nil {
			r.err = &NoncanonicalLimitError{Raw: value, Canonical: parsed.String()}
		} else {
			r.invalid(name)
		}
	}

	return parsed
}

func (r *fieldReader) acceptsLegacyMoneyRepresentation(name string, uppercase bool) bool {
	if !r.allowNoncanonicalLimit || !uppercase && !r.legacyReadShape {
		return false
	}

	return name == "Available" || name == "OnHold" || name == "OverdraftUsed"
}

func moneyText(raw json.RawMessage, allowNumber bool) (string, bool) {
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return "", false
		}

		return value, true
	}

	if allowNumber && isJSONNumber(raw) {
		return string(raw), true
	}

	return "", false
}

func isJSONNumber(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}

	return raw[0] == '-' || raw[0] >= '0' && raw[0] <= '9'
}

func (r *fieldReader) version() int64 {
	raw, exists, legacy := r.value("Version")
	if !exists {
		r.invalid("Version")
		return 0
	}

	text := string(raw)
	if !legacy && !r.legacyReadShape {
		text = r.text("Version", "", false)
	}

	version, err := strconv.ParseInt(text, 10, 64)
	if err != nil || version < 0 || strconv.FormatInt(version, 10) != text {
		r.invalid("Version")
	}

	return version
}

// Decode chooses legacy fields whenever present, independent of SchemaVersion.
// A malformed authoritative value is never replaced by a lower-camel fallback.
// Unknown fields are ignored; callers rewriting live blobs must preserve their
// extension fields separately rather than treating this projection as raw JSON.
// An old cold seed may omit Alias. It decodes with empty Alias and BalanceRef;
// callers must match ID, AccountID, Key and asset against trusted request scope
// before completing that logical identity. New-only blobs require an alias.
func Decode(raw []byte) (engine.BalanceSnapshot, error) {
	return decode(raw, false)
}

// DecodeForRead decodes a cache entry for read-only projection. Legacy money
// representations and a valid but noncanonical OverdraftLimit are normalized
// in the returned snapshot; raw is never modified. Mutating cache paths must
// use strict Decode plus conditional repair rather than this projection.
func DecodeForRead(raw []byte) (engine.BalanceSnapshot, error) {
	return decode(raw, true)
}

func detectLegacyReadShape(fields map[string]json.RawMessage, enabled bool) (bool, error) {
	legacy := enabled && fields["SchemaVersion"] == nil && fields["ID"] == nil
	if legacy {
		if _, exists := fields["id"]; !exists {
			return false, errors.New("legacy balance cache requires id")
		}
	}

	return legacy, nil
}

func decode(raw []byte, allowNoncanonicalLimit bool) (engine.BalanceSnapshot, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return engine.BalanceSnapshot{}, err
	}

	if schema, exists := fields["SchemaVersion"]; exists && string(schema) != "2" {
		return engine.BalanceSnapshot{}, errors.New("unsupported balance cache schema version")
	}

	legacyReadShape, err := detectLegacyReadShape(fields, allowNoncanonicalLimit)
	if err != nil {
		return engine.BalanceSnapshot{}, err
	}

	if _, legacy := fields["ID"]; !legacy && !legacyReadShape {
		if _, versioned := fields["SchemaVersion"]; !versioned {
			return engine.BalanceSnapshot{}, errors.New("new balance cache fields require schema version")
		}
	}

	r := fieldReader{fields: fields, allowNoncanonicalLimit: allowNoncanonicalLimit, legacyReadShape: legacyReadShape}
	_, aliasExists, aliasUppercase := r.value("Alias")
	_, legacyIdentity := fields["ID"]
	missingLegacyAlias := (legacyIdentity && !aliasExists) || (legacyReadShape && !aliasUppercase)
	id, idErr := uuid.Parse(r.text("ID", "", false))

	accountID, accountErr := uuid.Parse(r.text("AccountID", "", false))
	if idErr != nil || accountErr != nil || id == uuid.Nil || accountID == uuid.Nil {
		return engine.BalanceSnapshot{}, errors.New("invalid cached balance identity")
	}

	snapshot := engine.BalanceSnapshot{
		ID: id, AccountID: accountID,
		AccountType: r.text("AccountType", "", false), AssetCode: r.text("AssetCode", "", false),
		Alias: r.text("Alias", "", missingLegacyAlias), Key: r.text("Key", "default", true),
		Direction: r.text("Direction", "", true), BalanceScope: r.text("BalanceScope", "transactional", true),
		Available: r.money("Available", false), OnHold: r.money("OnHold", false),
		OverdraftUsed: r.money("OverdraftUsed", true), OverdraftLimit: r.money("OverdraftLimit", true),
		Version: r.version(), AllowSending: r.flag("AllowSending", false), AllowReceiving: r.flag("AllowReceiving", false),
		Blocked:        r.flag("Blocked", true),
		AllowOverdraft: r.flag("AllowOverdraft", true), OverdraftLimitEnabled: r.flag("OverdraftLimitEnabled", true),
	}
	if r.err != nil {
		return engine.BalanceSnapshot{}, r.err
	}

	if err := validateSnapshot(&snapshot, missingLegacyAlias); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	return snapshot, nil
}

// Encode writes both representations from one snapshot, or only new fields.
// New versions are strings so script JSON decoders cannot round int64 values.
func Encode(snapshot engine.BalanceSnapshot, format Format) ([]byte, error) {
	if format != FormatDual && format != FormatNewOnly {
		return nil, errors.New("unsupported balance cache format")
	}

	if err := validateSnapshot(&snapshot, false); err != nil {
		return nil, err
	}

	fields := map[string]any{
		"SchemaVersion": 2,
		"id":            snapshot.ID.String(), "accountId": snapshot.AccountID.String(),
		"accountType": snapshot.AccountType, "assetCode": snapshot.AssetCode,
		"alias": snapshot.Alias, "key": snapshot.Key,
		"direction": snapshot.Direction, "balanceScope": snapshot.BalanceScope,
		"available": snapshot.Available.String(), "onHold": snapshot.OnHold.String(),
		"overdraftUsed": snapshot.OverdraftUsed.String(), "overdraftLimit": snapshot.OverdraftLimit.String(),
		"version":      strconv.FormatInt(snapshot.Version, 10),
		"allowSending": snapshot.AllowSending, "allowReceiving": snapshot.AllowReceiving, "blocked": snapshot.Blocked,
		"allowOverdraft": snapshot.AllowOverdraft, "overdraftLimitEnabled": snapshot.OverdraftLimitEnabled,
	}

	if format == FormatDual {
		for _, name := range fieldNames {
			value := fields[lowerName(name)]
			if flag, ok := value.(bool); ok {
				value = 0
				if flag {
					value = 1
				}
			}

			fields[name] = value
		}

		fields["Version"] = snapshot.Version
	}

	return json.Marshal(fields)
}
