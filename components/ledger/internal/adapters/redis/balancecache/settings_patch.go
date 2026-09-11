// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
)

// SettingsPatch contains the settings values that a cache PATCH writer owns.
// Callers resolve domain defaults before constructing this value; the codec is
// responsible only for applying it to the wire representation.
type SettingsPatch struct {
	AllowOverdraft        bool
	OverdraftLimitEnabled bool
	OverdraftLimit        string
	BalanceScope          string
}

func encodeSettingsPatch(patch SettingsPatch) (map[string][2]json.RawMessage, error) {
	limit, err := decimal.NewFromString(patch.OverdraftLimit)
	if err != nil || limit.String() != patch.OverdraftLimit {
		return nil, errors.New("invalid balance settings overdraft limit")
	}

	limitJSON, err := json.Marshal(patch.OverdraftLimit)
	if err != nil {
		return nil, fmt.Errorf("encode balance settings overdraft limit: %w", err)
	}

	scopeJSON, err := json.Marshal(patch.BalanceScope)
	if err != nil {
		return nil, fmt.Errorf("encode balance settings scope: %w", err)
	}

	allowLegacy, allowModern, _ := dualFlagField("AllowOverdraft", json.RawMessage(strconv.FormatBool(patch.AllowOverdraft)), false)
	limitEnabledLegacy, limitEnabledModern, _ := dualFlagField("OverdraftLimitEnabled", json.RawMessage(strconv.FormatBool(patch.OverdraftLimitEnabled)), false)

	return map[string][2]json.RawMessage{
		"AllowOverdraft":        {allowLegacy, allowModern},
		"OverdraftLimitEnabled": {limitEnabledLegacy, limitEnabledModern},
		"OverdraftLimit":        {limitJSON, limitJSON},
		"BalanceScope":          {scopeJSON, scopeJSON},
	}, nil
}

// PatchSettingsDual converts a legacy, dual, or new-only cache object to the
// dual representation while changing only settings semantics. Uppercase fields
// are authoritative whenever present. Unknown fields and raw legacy monetary
// representations are retained so PATCH cannot round live state or erase
// extensions.
//
// This conversion intentionally does not call Decode or validateSnapshot. Old
// entries may omit Alias or use legacy monetary representations. Partial cache
// objects are tolerated by dualizing only the known fields that are present;
// all four settings fields are always emitted from patch.
func PatchSettingsDual(raw []byte, patch SettingsPatch) ([]byte, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}

	qualifiedKey, err := settingsPatchQualifiedKey(fields)
	if err != nil {
		return nil, err
	}

	settings, err := encodeSettingsPatch(patch)
	if err != nil {
		return nil, err
	}

	// Pre-schema lower-camel DTOs used the same numeric flag/version and
	// permissive money representations as uppercase legacy cache entries.
	legacyLowerShape := fields["SchemaVersion"] == nil

	for _, name := range fieldNames {
		source, exists, uppercase := authoritativeField(fields, name)
		delete(fields, name)
		delete(fields, lowerName(name))

		if setting, patched := settings[name]; patched {
			fields[name] = setting[0]
			fields[lowerName(name)] = setting[1]

			continue
		}

		if !exists {
			continue
		}

		legacy, modern, err := settingsPatchDualField(
			name, source, uppercase, legacyLowerShape, qualifiedKey,
		)
		if err != nil {
			return nil, err
		}

		fields[name] = legacy
		fields[lowerName(name)] = modern
	}

	delete(fields, "allowoverdraft")
	delete(fields, "overdraftlimitenabled")
	delete(fields, "overdraftlimit")
	delete(fields, "balancescope")
	fields["SchemaVersion"] = json.RawMessage("2")

	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode cached balance: %w", err)
	}

	return encoded, nil
}

func settingsPatchQualifiedKey(fields map[string]json.RawMessage) (*qualifiedLimitRepairKey, error) {
	keyRaw, keyExists, _ := authoritativeField(fields, "Key")
	if !keyExists {
		return nil, nil
	}

	qualifiedKey, qualified, err := parseQualifiedLimitRepairKey(keyRaw)
	if err != nil {
		return nil, err
	}

	if !qualified {
		return nil, nil
	}

	qualifiedKey.originalAlias, err = validateQualifiedLimitRepairAlias(fields, qualifiedKey)
	if err != nil {
		return nil, err
	}

	return qualifiedKey, nil
}

func settingsPatchDualField(
	name string,
	source json.RawMessage,
	uppercase, legacyLowerShape bool,
	qualifiedKey *qualifiedLimitRepairKey,
) (json.RawMessage, json.RawMessage, error) {
	legacyRepresentation := uppercase || legacyLowerShape

	switch name {
	case "AllowSending", "AllowReceiving", "Blocked":
		return dualFlagField(name, source, legacyRepresentation)
	case "Available", "OnHold":
		return dualMoneyField(name, source, legacyRepresentation)
	case "OverdraftUsed":
		if legacyLowerShape && !uppercase && string(source) == `""` {
			return quotedDualField(name, "0")
		}

		return dualMoneyField(name, source, legacyRepresentation)
	case "Version":
		return dualVersionField(source, legacyRepresentation)
	case "Key":
		if qualifiedKey != nil {
			_, modern, err := quotedDualField(name, qualifiedKey.domainKey)

			return source, modern, err
		}

		return dualTextField(name, source)
	case "Alias":
		if qualifiedKey != nil && qualifiedKey.originalAlias == qualifiedKey.originalKey {
			_, modern, err := quotedDualField(name, qualifiedKey.alias)

			return source, modern, err
		}

		return dualTextField(name, source)
	default:
		return dualTextField(name, source)
	}
}
