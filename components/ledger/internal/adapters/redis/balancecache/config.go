// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

// Configuration keys and the configuration copy in the balance blob.
//
// The configuration of a balance (AllowSending, AllowReceiving, AllowOverdraft,
// OverdraftLimitEnabled, OverdraftLimit, BalanceScope) and of an account (Blocked)
// lives in keys of its own, apart from the live balance blob whose money only the
// engine writes. This file fixes the contract every reader and writer shares:
//
// Names. utils.BalanceConfigKey(org, ledger, "alias#key") and
// utils.AccountConfigKey(org, ledger, accountID), prefixed with the tenant by
// tmvalkey.GetKeyContext in multi-tenant mode, in the {transactions} slot of the
// balance blobs. Lua never builds a name: it receives the keys through KEYS.
//
// Values. Canonical JSON written only by Go, from a PostgreSQL row:
//
//	balance: {"balanceId":"<uuid>","configVersion":"<n>","allowSending":<bool>,
//	          "allowReceiving":<bool>,"allowOverdraft":<bool>,"overdraftLimitEnabled":<bool>,
//	          "overdraftLimit":"<canonical decimal>","balanceScope":"transactional"|"internal"}
//	account: {"accountId":"<uuid>","configVersion":"<n>","blocked":<bool>}
//
// configVersion is the row's config_version as a canonical decimal string, so a Lua
// number never rounds it; the balance and the account versions are independent
// counters and are never compared with each other. Readers ignore unknown fields.
//
// Identity. A balance value describes the balance whose ID it carries. For any other
// balance it is absent: a balance deleted and recreated under the same alias#key is a
// different balance with its own version sequence. A write for another ID replaces
// the value whatever the versions.
//
// Writes. Only Go writes a key, with the whole configuration row and its version (a
// PATCH with the row its UPDATE returned, a seed with a primary read). A write lands
// when the key is absent, holds another identity or an older version; it then lives
// cachepolicy.ConfigKeyTTL from that write. The same version with the same content
// changes nothing, not even the expiry. The same version with different content is
// an inconsistency and is refused. An older version is not written, and the caller
// gets the stored, newer value back. Readers never touch the expiry.
//
// Blob copy (compatibility bridge for releases that read the configuration from the
// blob; removable once no rollback target reads it there). The blob keeps a copy of
// both configuration families plus their versions in two fields of its own,
// BlobBalanceConfigVersionField and BlobAccountConfigVersionField (decimal strings;
// absent reads as 0, which previous releases neither read nor drop). A copy replaces
// one whole family and its version together when the blob's version of that family
// is lower, or equal while the family's content differs (a blob that diverged before
// versions existed starts at version 0 like its row); it never writes over a higher
// version and never touches money, identity or the other family.
//
// Versions in shared types. accounting.BalanceSnapshot and mmodel.Balance carry
// ConfigVersion and AccountConfigVersion, and mmodel.Account carries ConfigVersion,
// all tagged json:"-": those types are serialized into completion plans, completion
// records, write-behind messages and API responses that the previous release decodes
// with unknown fields refused. Versions travel only in the engine request, in these
// keys and in the blob fields above.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const (
	// BlobBalanceConfigVersionField names the blob field holding the version of the
	// balance configuration copied into the blob.
	BlobBalanceConfigVersionField = "balanceConfigVersion"
	// BlobAccountConfigVersionField names the blob field holding the version of the
	// account's blocked flag copied into the blob.
	BlobAccountConfigVersionField = "accountConfigVersion"
)

// ErrBlobOfAnotherBalance reports a blob whose identity is not the one the
// configuration describes. Copying would hand one balance's configuration to
// another, so the copy is refused.
var ErrBlobOfAnotherBalance = errors.New("cached balance belongs to another balance or account")

type balanceConfigValue struct {
	BalanceID             string `json:"balanceId"`
	ConfigVersion         string `json:"configVersion"`
	AllowSending          *bool  `json:"allowSending"`
	AllowReceiving        *bool  `json:"allowReceiving"`
	AllowOverdraft        *bool  `json:"allowOverdraft"`
	OverdraftLimitEnabled *bool  `json:"overdraftLimitEnabled"`
	OverdraftLimit        string `json:"overdraftLimit"`
	BalanceScope          string `json:"balanceScope"`
}

type accountConfigValue struct {
	AccountID     string `json:"accountId"`
	ConfigVersion string `json:"configVersion"`
	Blocked       *bool  `json:"blocked"`
}

// BalanceConfigFromBalance takes the configuration of a balance row read from or
// returned by PostgreSQL. Absent settings are the defaults every reader applies
// to a legacy row: transactional scope, no overdraft, a zero limit.
func BalanceConfigFromBalance(balance *mmodel.Balance) (accounting.BalanceConfig, error) {
	if balance == nil {
		return accounting.BalanceConfig{}, errors.New("balance configuration: nil balance")
	}

	balanceID, err := uuid.Parse(balance.ID)
	if err != nil || balanceID == uuid.Nil {
		return accounting.BalanceConfig{}, fmt.Errorf("balance configuration: invalid balance ID %q", balance.ID)
	}

	config := accounting.BalanceConfig{
		BalanceID:      balanceID,
		Version:        balance.ConfigVersion,
		AllowSending:   balance.AllowSending,
		AllowReceiving: balance.AllowReceiving,
		OverdraftLimit: decimal.Zero,
		BalanceScope:   mmodel.BalanceScopeTransactional,
	}

	if settings := balance.Settings; settings != nil {
		config.AllowOverdraft = settings.AllowOverdraft
		config.OverdraftLimitEnabled = settings.OverdraftLimitEnabled

		if settings.BalanceScope != "" {
			config.BalanceScope = settings.BalanceScope
		}

		if settings.OverdraftLimit != nil {
			limit, parseErr := decimal.NewFromString(*settings.OverdraftLimit)
			if parseErr != nil {
				return accounting.BalanceConfig{}, fmt.Errorf("balance configuration: invalid overdraft limit %q on balance %s: %w", *settings.OverdraftLimit, balance.ID, parseErr)
			}

			config.OverdraftLimit = limit
		}
	}

	if err := validateBalanceConfig(config); err != nil {
		return accounting.BalanceConfig{}, err
	}

	return config, nil
}

// AccountConfigFromAccount takes the configuration of an account row read from or
// returned by PostgreSQL. An absent blocked flag is an account that is not blocked.
func AccountConfigFromAccount(account *mmodel.Account) (accounting.AccountConfig, error) {
	if account == nil {
		return accounting.AccountConfig{}, errors.New("account configuration: nil account")
	}

	accountID, err := uuid.Parse(account.ID)
	if err != nil || accountID == uuid.Nil {
		return accounting.AccountConfig{}, fmt.Errorf("account configuration: invalid account ID %q", account.ID)
	}

	config := accounting.AccountConfig{AccountID: accountID, Version: account.ConfigVersion}
	if account.Blocked != nil {
		config.Blocked = *account.Blocked
	}

	if config.Version < 0 {
		return accounting.AccountConfig{}, errors.New("account configuration: negative version")
	}

	return config, nil
}

func validateBalanceConfig(config accounting.BalanceConfig) error {
	if config.BalanceID == uuid.Nil {
		return errors.New("balance configuration: missing balance ID")
	}

	if config.Version < 0 {
		return errors.New("balance configuration: negative version")
	}

	if config.BalanceScope != mmodel.BalanceScopeTransactional && config.BalanceScope != mmodel.BalanceScopeInternal {
		return fmt.Errorf("balance configuration: invalid scope %q", config.BalanceScope)
	}

	return nil
}

// EncodeBalanceConfig writes the canonical value of a balance configuration key.
// Equal configurations always encode to the same bytes.
func EncodeBalanceConfig(config accounting.BalanceConfig) ([]byte, error) {
	if err := validateBalanceConfig(config); err != nil {
		return nil, err
	}

	return json.Marshal(balanceConfigValue{
		BalanceID:             config.BalanceID.String(),
		ConfigVersion:         strconv.FormatInt(config.Version, 10),
		AllowSending:          &config.AllowSending,
		AllowReceiving:        &config.AllowReceiving,
		AllowOverdraft:        &config.AllowOverdraft,
		OverdraftLimitEnabled: &config.OverdraftLimitEnabled,
		OverdraftLimit:        config.OverdraftLimit.String(),
		BalanceScope:          config.BalanceScope,
	})
}

// DecodeBalanceConfig reads a balance configuration key value. Every field of the
// contract is required; unknown fields are ignored.
func DecodeBalanceConfig(raw []byte) (accounting.BalanceConfig, error) {
	var value balanceConfigValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return accounting.BalanceConfig{}, fmt.Errorf("decode balance configuration: %w", err)
	}

	balanceID, err := uuid.Parse(value.BalanceID)
	if err != nil {
		return accounting.BalanceConfig{}, errors.New("decode balance configuration: invalid balance ID")
	}

	version, err := parseConfigVersion(value.ConfigVersion)
	if err != nil {
		return accounting.BalanceConfig{}, fmt.Errorf("decode balance configuration: %w", err)
	}

	if value.AllowSending == nil || value.AllowReceiving == nil || value.AllowOverdraft == nil || value.OverdraftLimitEnabled == nil {
		return accounting.BalanceConfig{}, errors.New("decode balance configuration: missing flag")
	}

	limit, err := decimal.NewFromString(value.OverdraftLimit)
	if err != nil || limit.String() != value.OverdraftLimit {
		return accounting.BalanceConfig{}, errors.New("decode balance configuration: invalid overdraft limit")
	}

	config := accounting.BalanceConfig{
		BalanceID:             balanceID,
		Version:               version,
		AllowSending:          *value.AllowSending,
		AllowReceiving:        *value.AllowReceiving,
		AllowOverdraft:        *value.AllowOverdraft,
		OverdraftLimitEnabled: *value.OverdraftLimitEnabled,
		OverdraftLimit:        limit,
		BalanceScope:          value.BalanceScope,
	}

	if err := validateBalanceConfig(config); err != nil {
		return accounting.BalanceConfig{}, err
	}

	return config, nil
}

// DecodeBalanceConfigFor reads a balance configuration key value for the balance
// with the given ID. A value of another balance is reported as absent (found is
// false), never as an error: it is what a balance recreated under the same
// alias#key finds until its own configuration replaces it.
func DecodeBalanceConfigFor(raw []byte, balanceID uuid.UUID) (config accounting.BalanceConfig, found bool, err error) {
	if len(raw) == 0 {
		return accounting.BalanceConfig{}, false, nil
	}

	config, err = DecodeBalanceConfig(raw)
	if err != nil {
		return accounting.BalanceConfig{}, false, err
	}

	if !config.Describes(balanceID) {
		return accounting.BalanceConfig{}, false, nil
	}

	return config, true, nil
}

// EncodeAccountConfig writes the canonical value of an account configuration key.
func EncodeAccountConfig(config accounting.AccountConfig) ([]byte, error) {
	if config.AccountID == uuid.Nil {
		return nil, errors.New("account configuration: missing account ID")
	}

	if config.Version < 0 {
		return nil, errors.New("account configuration: negative version")
	}

	return json.Marshal(accountConfigValue{
		AccountID:     config.AccountID.String(),
		ConfigVersion: strconv.FormatInt(config.Version, 10),
		Blocked:       &config.Blocked,
	})
}

// DecodeAccountConfig reads an account configuration key value. Every field of the
// contract is required; unknown fields are ignored.
func DecodeAccountConfig(raw []byte) (accounting.AccountConfig, error) {
	var value accountConfigValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return accounting.AccountConfig{}, fmt.Errorf("decode account configuration: %w", err)
	}

	accountID, err := uuid.Parse(value.AccountID)
	if err != nil || accountID == uuid.Nil {
		return accounting.AccountConfig{}, errors.New("decode account configuration: invalid account ID")
	}

	version, err := parseConfigVersion(value.ConfigVersion)
	if err != nil {
		return accounting.AccountConfig{}, fmt.Errorf("decode account configuration: %w", err)
	}

	if value.Blocked == nil {
		return accounting.AccountConfig{}, errors.New("decode account configuration: missing blocked flag")
	}

	return accounting.AccountConfig{AccountID: accountID, Version: version, Blocked: *value.Blocked}, nil
}

// parseConfigVersion accepts only the canonical decimal form of a non-negative
// version, the form the conditional write compares as text.
func parseConfigVersion(text string) (int64, error) {
	version, err := strconv.ParseInt(text, 10, 64)
	if err != nil || version < 0 || strconv.FormatInt(version, 10) != text {
		return 0, fmt.Errorf("invalid configuration version %q", text)
	}

	return version, nil
}

// CopyBalanceConfig returns the blob with the balance configuration family replaced
// by config and BlobBalanceConfigVersionField set to config.Version.
//
// current is true, and nothing is returned to write, when the blob holds a newer
// copy, or a copy of that version that already reads as config: a copy never moves
// the blob back, and repeating a configuration never writes. A copy of that version
// with other content is replaced. The blob must belong to config.BalanceID.
//
// Every other field passes through byte for byte: money (Available, OnHold,
// OverdraftUsed, Version, in whatever legacy, dual or new-only representation it
// is), identity, the account family and unknown fields are never decoded into
// numbers nor re-encoded. Configuration fields are written in the representations
// the blob already uses, so a legacy, dual or new-only blob keeps its shape.
func CopyBalanceConfig(raw []byte, config accounting.BalanceConfig) (replacement []byte, current bool, err error) {
	if err := validateBalanceConfig(config); err != nil {
		return nil, false, err
	}

	fields, err := decodeCopyTarget(raw)
	if err != nil {
		return nil, false, err
	}

	blobID, err := blobUUID(fields, "ID")
	if err != nil {
		return nil, false, err
	}

	if blobID != config.BalanceID {
		return nil, false, ErrBlobOfAnotherBalance
	}

	copied, err := blobConfigVersion(fields, BlobBalanceConfigVersionField)
	if err != nil {
		return nil, false, err
	}

	if copied > config.Version || copied == config.Version && blobHoldsBalanceConfig(fields, config) {
		return nil, true, nil
	}

	limitJSON, err := json.Marshal(config.OverdraftLimit.String())
	if err != nil {
		return nil, false, fmt.Errorf("encode overdraft limit copy: %w", err)
	}

	scopeJSON, err := json.Marshal(config.BalanceScope)
	if err != nil {
		return nil, false, fmt.Errorf("encode balance scope copy: %w", err)
	}

	setBlobFlag(fields, "AllowSending", config.AllowSending)
	setBlobFlag(fields, "AllowReceiving", config.AllowReceiving)
	setBlobFlag(fields, "AllowOverdraft", config.AllowOverdraft)
	setBlobFlag(fields, "OverdraftLimitEnabled", config.OverdraftLimitEnabled)
	setBlobText(fields, "OverdraftLimit", limitJSON)
	setBlobText(fields, "BalanceScope", scopeJSON)
	fields[BlobBalanceConfigVersionField] = configVersionJSON(config.Version)

	return encodeRawObject(fields), false, nil
}

// CopyAccountConfig returns the blob with the account's Blocked flag replaced by
// config.Blocked and BlobAccountConfigVersionField set to config.Version, under the
// same rules as CopyBalanceConfig. The blob must belong to an account balance of
// config.AccountID.
func CopyAccountConfig(raw []byte, config accounting.AccountConfig) (replacement []byte, current bool, err error) {
	if config.AccountID == uuid.Nil || config.Version < 0 {
		return nil, false, errors.New("account configuration copy: invalid configuration")
	}

	fields, err := decodeCopyTarget(raw)
	if err != nil {
		return nil, false, err
	}

	blobAccountID, err := blobUUID(fields, "AccountID")
	if err != nil {
		return nil, false, err
	}

	if blobAccountID != config.AccountID {
		return nil, false, ErrBlobOfAnotherBalance
	}

	copied, err := blobConfigVersion(fields, BlobAccountConfigVersionField)
	if err != nil {
		return nil, false, err
	}

	if copied > config.Version || copied == config.Version && blobFlagHolds(fields, "Blocked", config.Blocked) {
		return nil, true, nil
	}

	setBlobFlag(fields, "Blocked", config.Blocked)
	fields[BlobAccountConfigVersionField] = configVersionJSON(config.Version)

	return encodeRawObject(fields), false, nil
}

// blobHoldsBalanceConfig reports whether every representation the blob carries of
// the balance configuration family already reads as config. A family field the blob
// lacks does not hold it, so the copy writes it.
func blobHoldsBalanceConfig(fields map[string]json.RawMessage, config accounting.BalanceConfig) bool {
	return blobFlagHolds(fields, "AllowSending", config.AllowSending) &&
		blobFlagHolds(fields, "AllowReceiving", config.AllowReceiving) &&
		blobFlagHolds(fields, "AllowOverdraft", config.AllowOverdraft) &&
		blobFlagHolds(fields, "OverdraftLimitEnabled", config.OverdraftLimitEnabled) &&
		blobLimitHolds(fields, "OverdraftLimit", config.OverdraftLimit) &&
		blobTextHolds(fields, "BalanceScope", config.BalanceScope)
}

// blobRepresentations returns the values of every representation of a field the
// blob carries: uppercase, lowercase, or both.
func blobRepresentations(fields map[string]json.RawMessage, name string) []json.RawMessage {
	values := make([]json.RawMessage, 0, 2)

	if raw, exists := fields[name]; exists {
		values = append(values, raw)
	}

	if raw, exists := fields[lowerName(name)]; exists {
		values = append(values, raw)
	}

	return values
}

// blobFlagHolds accepts the numeric and the boolean forms in any representation.
func blobFlagHolds(fields map[string]json.RawMessage, name string, want bool) bool {
	values := blobRepresentations(fields, name)
	if len(values) == 0 {
		return false
	}

	for _, raw := range values {
		switch string(raw) {
		case "1", "true":
			if !want {
				return false
			}
		case "0", "false":
			if want {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// blobLimitHolds compares by value, so a limit written with other digits holds.
func blobLimitHolds(fields map[string]json.RawMessage, name string, want decimal.Decimal) bool {
	values := blobRepresentations(fields, name)
	if len(values) == 0 {
		return false
	}

	for _, raw := range values {
		text, valid := moneyText(raw, true)
		if !valid {
			return false
		}

		limit, err := decimal.NewFromString(text)
		if err != nil || !limit.Equal(want) {
			return false
		}
	}

	return true
}

func blobTextHolds(fields map[string]json.RawMessage, name, want string) bool {
	values := blobRepresentations(fields, name)
	if len(values) == 0 {
		return false
	}

	for _, raw := range values {
		var text string
		if json.Unmarshal(raw, &text) != nil || text != want {
			return false
		}
	}

	return true
}

// BlobConfigVersions reads the versions of the two configuration copies a blob
// holds; a blob without a copy of a family reads 0 for it.
func BlobConfigVersions(raw []byte) (balanceVersion, accountVersion int64, err error) {
	fields, err := decodeCopyTarget(raw)
	if err != nil {
		return 0, 0, err
	}

	return blobConfigVersions(fields)
}

func blobConfigVersions(fields map[string]json.RawMessage) (balanceVersion, accountVersion int64, err error) {
	if balanceVersion, err = blobConfigVersion(fields, BlobBalanceConfigVersionField); err != nil {
		return 0, 0, err
	}

	if accountVersion, err = blobConfigVersion(fields, BlobAccountConfigVersionField); err != nil {
		return 0, 0, err
	}

	return balanceVersion, accountVersion, nil
}

func decodeCopyTarget(raw []byte) (map[string]json.RawMessage, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}

	if schema, exists := fields["SchemaVersion"]; exists && string(schema) != "2" {
		return nil, errors.New("unsupported balance cache schema version")
	}

	return fields, nil
}

func blobUUID(fields map[string]json.RawMessage, name string) (uuid.UUID, error) {
	raw, exists, _ := authoritativeField(fields, name)

	var text string
	if !exists || json.Unmarshal(raw, &text) != nil {
		return uuid.Nil, fmt.Errorf("invalid cached balance field %s", name)
	}

	id, err := uuid.Parse(text)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid cached balance field %s", name)
	}

	return id, nil
}

func blobConfigVersion(fields map[string]json.RawMessage, name string) (int64, error) {
	raw, exists := fields[name]
	if !exists {
		return 0, nil
	}

	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, fmt.Errorf("invalid cached balance field %s", name)
	}

	version, err := parseConfigVersion(text)
	if err != nil {
		return 0, fmt.Errorf("invalid cached balance field %s: %w", name, err)
	}

	return version, nil
}

func configVersionJSON(version int64) json.RawMessage {
	return json.RawMessage(strconv.Quote(strconv.FormatInt(version, 10)))
}

// blobFieldForms reports which representations of a field to write: the ones
// the blob already carries for it, or, for a field the blob lacks, the
// representations the blob uses for its identity.
func blobFieldForms(fields map[string]json.RawMessage, name string) (uppercase, lowercase bool) {
	_, uppercase = fields[name]
	_, lowercase = fields[lowerName(name)]

	if uppercase || lowercase {
		return uppercase, lowercase
	}

	_, uppercase = fields["ID"]
	_, lowercase = fields["id"]

	return uppercase, lowercase
}

// setBlobFlag writes a flag in each representation the blob uses: numeric 0/1 in
// the uppercase field and in a pre-schema lowercase field, a JSON boolean in a
// schema-versioned lowercase field.
func setBlobFlag(fields map[string]json.RawMessage, name string, value bool) {
	uppercase, lowercase := blobFieldForms(fields, name)

	numeric := json.RawMessage("0")
	boolean := json.RawMessage("false")

	if value {
		numeric = json.RawMessage("1")
		boolean = json.RawMessage("true")
	}

	if uppercase {
		fields[name] = numeric
	}

	if lowercase {
		if fields["SchemaVersion"] == nil {
			fields[lowerName(name)] = numeric
		} else {
			fields[lowerName(name)] = boolean
		}
	}
}

// setBlobText writes a JSON string field in each representation the blob uses.
func setBlobText(fields map[string]json.RawMessage, name string, value json.RawMessage) {
	uppercase, lowercase := blobFieldForms(fields, name)

	if uppercase {
		fields[name] = value
	}

	if lowercase {
		fields[lowerName(name)] = value
	}
}

// encodeRawObject writes the fields as one JSON object in key order, copying each
// value's bytes exactly as they were read: no value is re-encoded, compacted or
// escaped on the way out.
func encodeRawObject(fields map[string]json.RawMessage) []byte {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}

	sort.Strings(names)

	var out bytes.Buffer

	out.WriteByte('{')

	for i, name := range names {
		if i > 0 {
			out.WriteByte(',')
		}

		// The name was decoded from JSON, so it always encodes back to JSON.
		key, _ := json.Marshal(name) //nolint:errchkjson // a string never fails to encode

		out.Write(key)
		out.WriteByte(':')
		out.Write(fields[name])
	}

	out.WriteByte('}')

	return out.Bytes()
}
