// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger_test

import (
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/LerianStudio/lib-system-facts/contract"
	"github.com/LerianStudio/lib-system-facts/contractcheck/contractchecktest"
	"github.com/LerianStudio/lib-system-facts/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger"
)

const (
	systemFactsDir               = "contracts/system-facts"
	transactionCommittedType     = "midaz.transaction_committed"
	transactionCommittedVersion  = "1.0.0"
	transactionIDReferenceType   = "lerian.midaz.transaction_id"
	transactionCommittedFixtures = "fixtures/midaz.transaction_committed/1.0.0/"
)

// loadSystemFactsCatalog compiles the embedded contracts the way the producer
// does at boot, so every assertion reads what the kit reads.
func loadSystemFactsCatalog(t *testing.T) (*manifest.Manifest, *manifest.Catalog) {
	t.Helper()

	dir, err := fs.Sub(ledger.SystemFactsContracts, systemFactsDir)
	require.NoError(t, err)

	m, err := manifest.Load(dir, ".")
	require.NoError(t, err, "embedded manifest must load")

	catalog, err := m.Compile()
	require.NoError(t, err, "embedded manifest must compile")

	return m, catalog
}

// transactionCommittedSchema is the part of the schema the fixture tests read:
// the closed root key set and the enum each branch field admits.
type transactionCommittedSchema struct {
	Required   []string `json:"required"`
	Properties map[string]struct {
		Enum []string `json:"enum"`
	} `json:"properties"`
}

func loadTransactionCommittedSchema(t *testing.T, m *manifest.Manifest) transactionCommittedSchema {
	t.Helper()

	require.Len(t, m.Facts, 1)

	var schema transactionCommittedSchema

	require.NoError(t, json.Unmarshal(m.Facts[0].Schema, &schema))
	require.NotEmpty(t, schema.Required)

	return schema
}

func readFixture(t *testing.T, name string) map[string]any {
	t.Helper()

	raw, err := fs.ReadFile(ledger.SystemFactsContracts, systemFactsDir+"/"+transactionCommittedFixtures+name)
	require.NoError(t, err, "fixture %s must be embedded", name)

	var payload map[string]any

	require.NoError(t, json.Unmarshal(raw, &payload), "fixture %s must be a JSON object", name)

	return payload
}

func TestSystemFactsContracts_OfflineContractCheck_Passes(t *testing.T) {
	t.Parallel()

	contractchecktest.AssertOffline(t, ledger.SystemFactsContracts, systemFactsDir)
}

// A misspelled x-sensitivity is an ordinary vendor extension: the kit ignores
// it and the service stores the value in clear, so the protected set is pinned
// exactly, class included.
func TestSystemFactsContracts_TransactionCommitted_ProtectsOnlyDescriptionEncrypted(t *testing.T) {
	t.Parallel()

	_, catalog := loadSystemFactsCatalog(t)

	compiled, ok := catalog.Contract(transactionCommittedType, transactionCommittedVersion)
	require.True(t, ok, "catalog must compile %s@%s", transactionCommittedType, transactionCommittedVersion)

	want := []manifest.ProtectedField{
		{
			Pointer:     "/description",
			Path:        []string{"description"},
			Sensitivity: manifest.SensitivityEncrypted,
			Mask:        manifest.DocumentMaskNone,
		},
	}

	assert.Equal(t, want, compiled.ProtectedFields())
}

func TestSystemFactsContracts_Manifest_DeclaresOneFactWithOneReference(t *testing.T) {
	t.Parallel()

	m, catalog := loadSystemFactsCatalog(t)

	assert.Equal(t, "midaz", catalog.Producer())
	assert.Equal(t, "lerian.midaz.transactions", catalog.Namespace())
	assert.Equal(t, "mazar", catalog.Team())

	require.Len(t, m.Facts, 1, "the catalog declares exactly one fact type")

	fact := m.Facts[0]
	assert.Equal(t, transactionCommittedType, fact.Type)
	assert.Equal(t, transactionCommittedVersion, fact.SchemaVersion)
	assert.Equal(t, []contract.ReferenceDeclaration{{
		Type:        transactionIDReferenceType,
		Cardinality: contract.CardinalityOne,
		Required:    true,
		Correlation: contract.CorrelationExact,
	}}, fact.References)

	// Pix and Fees mirror this type as owner external: any change to scope,
	// sensitivity, correlation or searchable fails their reconcilers.
	require.Len(t, m.ReferenceTypes, 1, "only the type the fact uses is declared")
	assert.Equal(t, manifest.ReferenceType{
		Type:        transactionIDReferenceType,
		Owner:       manifest.OwnerSelf,
		Scope:       []string{"ledgerId", "organizationId"},
		Sensitivity: manifest.SensitivityNone,
		Correlation: contract.CorrelationExact,
		Searchable:  false,
		Description: "Midaz transaction id (UUIDv7) within a ledger",
		Mask:        manifest.DocumentMaskNone,
	}, m.ReferenceTypes[0])
}

// AssertOffline validates each fixture against the schema; this pins that each
// one is a complete payload, so a fixture cannot be trimmed to the optional
// fields it was written to cover.
func TestSystemFactsContracts_TransactionCommittedFixtures_CarryEveryRequiredRootKey(t *testing.T) {
	t.Parallel()

	m, _ := loadSystemFactsCatalog(t)
	schema := loadTransactionCommittedSchema(t, m)

	tests := []struct {
		name    string
		fixture string
	}{
		{name: "inflow direct", fixture: "inflow_direct.json"},
		{name: "outflow direct", fixture: "outflow_direct.json"},
		{name: "outflow commit", fixture: "outflow_commit.json"},
		{name: "inflow revert", fixture: "inflow_revert.json"},
		{name: "internal fee", fixture: "internal_fee.json"},
		{name: "mixed direct", fixture: "mixed_direct.json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := readFixture(t, tc.fixture)

			for _, key := range schema.Required {
				assert.Contains(t, payload, key, "fixture %s lacks required root key %q", tc.fixture, key)
			}
		})
	}
}

// The manifest lists the fixtures contractcheck validates; a renamed or
// dropped file narrows the branch coverage silently unless the set and the
// branches it reaches are pinned.
func TestSystemFactsContracts_TransactionCommittedFixtures_CoverEveryBranch(t *testing.T) {
	t.Parallel()

	m, _ := loadSystemFactsCatalog(t)
	schema := loadTransactionCommittedSchema(t, m)

	wantFixtures := []string{
		transactionCommittedFixtures + "inflow_direct.json",
		transactionCommittedFixtures + "outflow_direct.json",
		transactionCommittedFixtures + "outflow_commit.json",
		transactionCommittedFixtures + "inflow_revert.json",
		transactionCommittedFixtures + "internal_fee.json",
		transactionCommittedFixtures + "mixed_direct.json",
	}
	assert.ElementsMatch(t, wantFixtures, m.Facts[0].Fixtures)

	seenValues := map[string]map[string]bool{"action": {}, "direction": {}}
	seenProperties := map[string]bool{}

	for _, fixture := range m.Facts[0].Fixtures {
		raw, err := fs.ReadFile(ledger.SystemFactsContracts, systemFactsDir+"/"+fixture)
		require.NoError(t, err, fixture)

		var payload map[string]any

		require.NoError(t, json.Unmarshal(raw, &payload), fixture)

		for key, value := range payload {
			seenProperties[key] = true

			if seen, branch := seenValues[key]; branch {
				text, isString := value.(string)
				require.True(t, isString, "fixture %s: %s must be a string", fixture, key)

				seen[text] = true
			}
		}
	}

	for name := range schema.Properties {
		assert.True(t, seenProperties[name], "no fixture carries the property %q", name)
	}

	for field, seen := range seenValues {
		for _, value := range schema.Properties[field].Enum {
			assert.True(t, seen[value], "no fixture carries %s %q", field, value)
		}
	}
}
