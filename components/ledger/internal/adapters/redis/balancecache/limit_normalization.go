// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// NormalizeLimitDual conditionally converts a cached balance to the complete
// dual representation while repairing a noncanonical authoritative overdraft
// limit. A canonical authoritative limit is a true no-op. Uppercase fields are
// authoritative whenever present, and unknown fields are retained.
//
// trustedAlias completes only an absent legacy alias. It is never used to
// replace a present value.
func NormalizeLimitDual(raw []byte, trustedAlias string) ([]byte, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}

	decodeRaw, qualifiedKey, err := limitRepairValidationView(raw, fields)
	if err != nil {
		return nil, err
	}

	snapshot, err := DecodeForRead(decodeRaw)
	if err != nil {
		return nil, err
	}

	limitRaw, limitExists, _ := authoritativeField(fields, "OverdraftLimit")
	if !limitExists {
		return raw, nil
	}

	limitText, valid := moneyText(limitRaw, false)
	if !valid {
		return nil, errors.New("invalid cached balance field OverdraftLimit")
	}

	canonicalLimit := snapshot.OverdraftLimit.String()
	if limitText == canonicalLimit {
		return raw, nil
	}

	if err := completeLimitRepairAlias(fields, &snapshot, trustedAlias, qualifiedKey); err != nil {
		return nil, err
	}

	legacyLowerShape := fields["SchemaVersion"] == nil

	for _, name := range fieldNames {
		source, exists, uppercase := authoritativeField(fields, name)
		delete(fields, name)
		delete(fields, lowerName(name))

		legacy, modern, err := limitRepairDualField(
			snapshot, name, source, exists, uppercase, legacyLowerShape, canonicalLimit, qualifiedKey,
		)
		if err != nil {
			return nil, err
		}

		fields[name] = legacy
		fields[lowerName(name)] = modern
	}

	fields["SchemaVersion"] = json.RawMessage("2")

	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode cached balance: %w", err)
	}

	return encoded, nil
}

type qualifiedLimitRepairKey struct {
	alias         string
	domainKey     string
	originalAlias string
	originalKey   string
}

func limitRepairValidationView(
	raw []byte,
	fields map[string]json.RawMessage,
) ([]byte, *qualifiedLimitRepairKey, error) {
	keyRaw, keyExists, uppercase := authoritativeField(fields, "Key")
	if !keyExists {
		return raw, nil, nil
	}

	qualifiedKey, qualified, err := parseQualifiedLimitRepairKey(keyRaw)
	if err != nil {
		return nil, nil, err
	}

	if !qualified {
		return raw, nil, nil
	}

	if !uppercase && fields["SchemaVersion"] != nil {
		return nil, nil, errors.New("qualified key is not permitted in new-only cached balance")
	}

	qualifiedKey.originalAlias, err = validateQualifiedLimitRepairAlias(fields, qualifiedKey)
	if err != nil {
		return nil, nil, err
	}

	decodeRaw, err := encodeLimitRepairValidationView(fields, qualifiedKey.domainKey)
	if err != nil {
		return nil, nil, err
	}

	return decodeRaw, qualifiedKey, nil
}

func parseQualifiedLimitRepairKey(raw json.RawMessage) (*qualifiedLimitRepairKey, bool, error) {
	var key string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &key) != nil {
		return nil, false, errors.New("invalid cached balance field Key")
	}

	parts := strings.Split(key, "#")
	if len(parts) == 1 {
		return nil, false, nil
	}

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, false, errors.New("invalid qualified cached balance key")
	}

	return &qualifiedLimitRepairKey{
		alias: parts[0], domainKey: parts[1], originalKey: key,
	}, true, nil
}

func validateQualifiedLimitRepairAlias(
	fields map[string]json.RawMessage,
	qualifiedKey *qualifiedLimitRepairKey,
) (string, error) {
	aliasRaw, aliasExists, _ := authoritativeField(fields, "Alias")
	if !aliasExists {
		return "", nil
	}

	var alias string
	if len(aliasRaw) == 0 || aliasRaw[0] != '"' || json.Unmarshal(aliasRaw, &alias) != nil {
		return "", errors.New("invalid cached balance field Alias")
	}

	if alias != qualifiedKey.alias && alias != qualifiedKey.originalKey {
		return "", errors.New("cached balance alias does not match qualified key")
	}

	return alias, nil
}

func encodeLimitRepairValidationView(fields map[string]json.RawMessage, domainKey string) ([]byte, error) {
	encodedDomainKey, err := json.Marshal(domainKey)
	if err != nil {
		return nil, fmt.Errorf("encode cached balance domain key: %w", err)
	}

	validationFields := make(map[string]json.RawMessage, len(fields))
	for name, value := range fields {
		validationFields[name] = value
	}

	if _, exists := validationFields["Key"]; exists {
		validationFields["Key"] = encodedDomainKey
	}

	if _, exists := validationFields["key"]; exists {
		validationFields["key"] = encodedDomainKey
	}

	decodeRaw, err := json.Marshal(validationFields)
	if err != nil {
		return nil, fmt.Errorf("encode normalized cached balance: %w", err)
	}

	return decodeRaw, nil
}

func completeLimitRepairAlias(
	fields map[string]json.RawMessage,
	snapshot *accounting.BalanceSnapshot,
	trustedAlias string,
	qualifiedKey *qualifiedLimitRepairKey,
) error {
	_, aliasExists, _ := authoritativeField(fields, "Alias")
	if aliasExists {
		if snapshot.Alias == "" {
			return errors.New("inconsistent balance alias and key")
		}

		return nil
	}

	snapshot.Alias = trustedAlias
	if err := normalizeLogicalIdentity(snapshot, false); err != nil {
		return err
	}

	if qualifiedKey != nil && snapshot.Alias != qualifiedKey.alias {
		return errors.New("trusted balance alias does not match qualified key")
	}

	return nil
}

func limitRepairDualField(
	snapshot accounting.BalanceSnapshot,
	name string,
	source json.RawMessage,
	exists bool,
	uppercase bool,
	legacyLowerShape bool,
	canonicalLimit string,
	qualifiedKey *qualifiedLimitRepairKey,
) (json.RawMessage, json.RawMessage, error) {
	if !exists {
		return snapshotDualField(snapshot, name)
	}

	legacyRepresentation := uppercase || legacyLowerShape

	switch name {
	case "AllowSending", "AllowReceiving", "Blocked", "AllowOverdraft", "OverdraftLimitEnabled":
		return dualFlagField(name, source, legacyRepresentation)
	case "Available", "OnHold":
		return dualMoneyField(name, source, legacyRepresentation)
	case "OverdraftUsed":
		if legacyLowerShape && !uppercase && string(source) == `""` {
			return snapshotDualField(snapshot, name)
		}

		return dualMoneyField(name, source, legacyRepresentation)
	case "OverdraftLimit":
		return quotedDualField(name, canonicalLimit)
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

func quotedDualField(name, value string) (json.RawMessage, json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, nil, fmt.Errorf("encode cached balance field %s: %w", name, err)
	}

	return encoded, encoded, nil
}

func snapshotDualField(snapshot accounting.BalanceSnapshot, name string) (json.RawMessage, json.RawMessage, error) {
	switch name {
	case "ID":
		return quotedDualField(name, snapshot.ID.String())
	case "AccountID":
		return quotedDualField(name, snapshot.AccountID.String())
	case "AccountType":
		return quotedDualField(name, snapshot.AccountType)
	case "AssetCode":
		return quotedDualField(name, snapshot.AssetCode)
	case "Alias":
		return quotedDualField(name, snapshot.Alias)
	case "Key":
		return quotedDualField(name, snapshot.Key)
	case "Direction":
		return quotedDualField(name, snapshot.Direction)
	case "BalanceScope":
		return quotedDualField(name, snapshot.BalanceScope)
	case "Available":
		return quotedDualField(name, snapshot.Available.String())
	case "OnHold":
		return quotedDualField(name, snapshot.OnHold.String())
	case "OverdraftUsed":
		return quotedDualField(name, snapshot.OverdraftUsed.String())
	case "OverdraftLimit":
		return quotedDualField(name, snapshot.OverdraftLimit.String())
	case "Version":
		return dualVersionField(json.RawMessage(strconv.FormatInt(snapshot.Version, 10)), true)
	case "AllowSending":
		return dualFlagField(name, json.RawMessage(strconv.FormatBool(snapshot.AllowSending)), false)
	case "AllowReceiving":
		return dualFlagField(name, json.RawMessage(strconv.FormatBool(snapshot.AllowReceiving)), false)
	case "Blocked":
		return dualFlagField(name, json.RawMessage(strconv.FormatBool(snapshot.Blocked)), false)
	case "AllowOverdraft":
		return dualFlagField(name, json.RawMessage(strconv.FormatBool(snapshot.AllowOverdraft)), false)
	case "OverdraftLimitEnabled":
		return dualFlagField(name, json.RawMessage(strconv.FormatBool(snapshot.OverdraftLimitEnabled)), false)
	default:
		return nil, nil, fmt.Errorf("unknown cached balance field %s", name)
	}
}
