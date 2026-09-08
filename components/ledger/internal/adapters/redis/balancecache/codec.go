// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package balancecache defines lossless balance cache representations.
package balancecache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
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

// SettingsPatch contains the settings values that a cache PATCH writer owns.
// Callers resolve domain defaults before constructing this value; the codec is
// responsible only for applying it to the wire representation.
type SettingsPatch struct {
	AllowOverdraft        bool
	OverdraftLimitEnabled bool
	OverdraftLimit        string
	BalanceScope          string
}

// NoncanonicalLimitError requires an explicit conditional repair of a live limit.
// Decode never returns a usable snapshot alongside this error.
type NoncanonicalLimitError struct {
	Raw       string
	Canonical string
}

func (e *NoncanonicalLimitError) Error() string {
	return "noncanonical cached overdraft limit"
}

var fieldNames = []string{
	"ID", "AccountID", "AccountType", "AssetCode", "Alias", "Key", "Direction", "BalanceScope",
	"Available", "OnHold", "OverdraftUsed", "Version", "AllowSending", "AllowReceiving",
	"Blocked", "AllowOverdraft", "OverdraftLimitEnabled", "OverdraftLimit",
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

		legacyRepresentation := uppercase || legacyLowerShape

		var legacy, modern json.RawMessage

		switch name {
		case "AllowSending", "AllowReceiving", "Blocked":
			legacy, modern, err = dualFlagField(name, source, legacyRepresentation)
		case "Available", "OnHold", "OverdraftUsed":
			legacy, modern, err = dualMoneyField(name, source, legacyRepresentation)
		case "Version":
			legacy, modern, err = dualVersionField(source, legacyRepresentation)
		default:
			legacy, modern, err = dualTextField(name, source)
		}

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
	snapshot *engine.BalanceSnapshot,
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
	snapshot engine.BalanceSnapshot,
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

func snapshotDualField(snapshot engine.BalanceSnapshot, name string) (json.RawMessage, json.RawMessage, error) {
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
		Version: r.version(), AllowSending: r.flag("AllowSending", false), AllowReceiving: r.flag("AllowReceiving", false), Blocked: r.flag("Blocked", true),
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

func validateSnapshot(snapshot *engine.BalanceSnapshot, allowMissingAlias bool) error {
	if snapshot.ID == uuid.Nil || snapshot.AccountID == uuid.Nil || snapshot.AccountType == "" || snapshot.AssetCode == "" {
		return errors.New("incomplete balance cache identity")
	}

	if snapshot.Version < 0 || snapshot.OnHold.IsNegative() || snapshot.OverdraftUsed.IsNegative() || snapshot.OverdraftLimit.IsNegative() {
		return errors.New("invalid balance cache numeric state")
	}

	if snapshot.Direction != "" && snapshot.Direction != "credit" && snapshot.Direction != "debit" {
		return errors.New("invalid balance cache direction")
	}

	if snapshot.BalanceScope == "" {
		snapshot.BalanceScope = "transactional"
	}

	if snapshot.BalanceScope != "transactional" && snapshot.BalanceScope != "internal" {
		return errors.New("invalid balance cache scope")
	}

	return normalizeLogicalIdentity(snapshot, allowMissingAlias)
}

func normalizeLogicalIdentity(snapshot *engine.BalanceSnapshot, allowMissingAlias bool) error {
	if snapshot.Key == "" {
		snapshot.Key = "default"
	}

	if strings.Contains(snapshot.Key, "#") {
		return errors.New("invalid balance cache key")
	}

	if allowMissingAlias && snapshot.Alias == "" {
		return nil
	}

	alias := snapshot.Alias
	if parts := strings.Split(alias, "#"); len(parts) == 2 && parts[1] == snapshot.Key {
		alias = parts[0]
	}

	if alias == "" || strings.Contains(alias, "#") {
		return errors.New("inconsistent balance alias and key")
	}

	ref := alias + "#" + snapshot.Key
	if snapshot.BalanceRef != "" && snapshot.BalanceRef != ref {
		return errors.New("inconsistent balance reference")
	}

	snapshot.Alias, snapshot.BalanceRef = alias, ref

	return nil
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
		"allowSending": snapshot.AllowSending, "allowReceiving": snapshot.AllowReceiving,
		"blocked":        snapshot.Blocked,
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
