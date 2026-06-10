// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package plugincrm

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"testing"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
)

const (
	testHashKey = "test-hash-secret-key"
	// testEncryptKey is a hex-encoded 32-byte (AES-256) key.
	testEncryptKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
)

func testLogger() log.Logger { return log.NewNop() }

// expectedHash computes the HMAC-SHA256 hex hash the way lib-commons crypto does,
// so the filter-mapping test can assert exact hashed values without depending on
// the library internals.
func expectedHash(t *testing.T, plaintext string) string {
	t.Helper()

	h := hmac.New(sha256.New, []byte(testHashKey))
	h.Write([]byte(plaintext))

	return hex.EncodeToString(h.Sum(nil))
}

func TestIs(t *testing.T) {
	assert.True(t, Is("plugin_crm"))
	assert.False(t, Is("onboarding"))
	assert.False(t, Is(""))
}

func TestIsQueryableCollection(t *testing.T) {
	assert.False(t, IsQueryableCollection("organization"))
	assert.True(t, IsQueryableCollection("holders"))
}

func TestTransformFilters_NilAndUnconfigured(t *testing.T) {
	t.Run("nil filter returns nil with no error", func(t *testing.T) {
		out, err := TransformFilters(nil, testHashKey, testLogger())
		require.NoError(t, err)
		assert.Nil(t, out)
	})

	t.Run("empty hash key fails closed", func(t *testing.T) {
		out, err := TransformFilters(map[string]model.FilterCondition{
			"document": {Equals: []any{"123"}},
		}, "", testLogger())

		require.Error(t, err)
		assert.Nil(t, out)

		var preErr pkgErr.FailedPreconditionError
		require.True(t, errors.As(err, &preErr))
		assert.Equal(t, cnErr.ErrCodeCRMHashKeyNotConfigured.Error(), preErr.Code)
	})
}

func TestTransformFilters_MapsFieldsAndHashesValues(t *testing.T) {
	in := map[string]model.FilterCondition{
		// Mapped, encrypted top-level field -> search.document, hashed value.
		"document": {Equals: []any{"12345678900"}},
		// Mapped nested field -> search.contact_primary_email, hashed In values.
		"contact.primary_email": {In: []any{"a@example.com", "b@example.com"}},
		// Unmapped field passes through untouched (and not hashed).
		"status": {Equals: []any{"ACTIVE"}},
	}

	out, err := TransformFilters(in, testHashKey, testLogger())
	require.NoError(t, err)

	// Field renames happened; original keys are gone.
	_, hasDocument := out["document"]
	assert.False(t, hasDocument, "mapped source field must be renamed away")

	searchDoc, ok := out["search.document"]
	require.True(t, ok, "document must map to search.document")
	require.Len(t, searchDoc.Equals, 1)
	assert.Equal(t, expectedHash(t, "12345678900"), searchDoc.Equals[0])

	searchEmail, ok := out["search.contact_primary_email"]
	require.True(t, ok)
	require.Len(t, searchEmail.In, 2)
	assert.Equal(t, expectedHash(t, "a@example.com"), searchEmail.In[0])
	assert.Equal(t, expectedHash(t, "b@example.com"), searchEmail.In[1])

	// Unmapped field kept verbatim, NOT hashed.
	status, ok := out["status"]
	require.True(t, ok)
	require.Len(t, status.Equals, 1)
	assert.Equal(t, "ACTIVE", status.Equals[0])
}

func TestTransformFilters_NonStringValuesUntouched(t *testing.T) {
	in := map[string]model.FilterCondition{
		// name is mapped -> search.name; numeric + empty-string values must pass
		// through unchanged (only non-empty strings are hashed).
		"name": {In: []any{42, "", "real"}},
	}

	out, err := TransformFilters(in, testHashKey, testLogger())
	require.NoError(t, err)

	got, ok := out["search.name"]
	require.True(t, ok)
	require.Len(t, got.In, 3)
	assert.Equal(t, 42, got.In[0])
	assert.Equal(t, "", got.In[1])
	assert.Equal(t, expectedHash(t, "real"), got.In[2])
}

func TestTransformFilters_AllOperatorsHashed(t *testing.T) {
	in := map[string]model.FilterCondition{
		// document is mapped -> search.document; every operator carries a single
		// string value so each per-operator hashing branch is exercised.
		"document": {
			Equals:         []any{"eq"},
			GreaterThan:    []any{"gt"},
			GreaterOrEqual: []any{"gte"},
			LessThan:       []any{"lt"},
			LessOrEqual:    []any{"lte"},
			Between:        []any{"b0", "b1"},
			In:             []any{"in"},
			NotIn:          []any{"nin"},
		},
	}

	out, err := TransformFilters(in, testHashKey, testLogger())
	require.NoError(t, err)

	got, ok := out["search.document"]
	require.True(t, ok)
	assert.Equal(t, expectedHash(t, "eq"), got.Equals[0])
	assert.Equal(t, expectedHash(t, "gt"), got.GreaterThan[0])
	assert.Equal(t, expectedHash(t, "gte"), got.GreaterOrEqual[0])
	assert.Equal(t, expectedHash(t, "lt"), got.LessThan[0])
	assert.Equal(t, expectedHash(t, "lte"), got.LessOrEqual[0])
	assert.Equal(t, expectedHash(t, "b0"), got.Between[0])
	assert.Equal(t, expectedHash(t, "b1"), got.Between[1])
	assert.Equal(t, expectedHash(t, "in"), got.In[0])
	assert.Equal(t, expectedHash(t, "nin"), got.NotIn[0])
}

func TestDecryptRecords_DecryptFailurePropagates(t *testing.T) {
	// A top-level encrypted field carrying a value that is not valid base64
	// ciphertext must surface as a record-decryption precondition error, not a
	// silent pass-through.
	records := []map[string]any{{"document": "not-valid-base64-ciphertext!!!"}}

	_, err := DecryptRecords(records, []string{"document"}, testHashKey, testEncryptKey, testLogger())
	require.Error(t, err)

	var preErr pkgErr.FailedPreconditionError
	require.True(t, errors.As(err, &preErr))
	assert.Equal(t, cnErr.ErrCodeRecordDecryptionFailed.Error(), preErr.Code)
}

func TestDecryptRecords_NoDecryptableFieldsShortCircuits(t *testing.T) {
	records := []map[string]any{{"status": "ACTIVE"}}

	// Only flat, non-encrypted, non-dotted fields requested -> no decryption,
	// input returned untouched even with empty keys.
	out, err := DecryptRecords(records, []string{"status", "id"}, "", "", testLogger())
	require.NoError(t, err)
	assert.Equal(t, records, out)
}

func TestDecryptRecords_FailsClosedOnMissingKeys(t *testing.T) {
	records := []map[string]any{{"document": "x"}}

	t.Run("missing encrypt key", func(t *testing.T) {
		_, err := DecryptRecords(records, []string{"document"}, testHashKey, "", testLogger())
		require.Error(t, err)

		var preErr pkgErr.FailedPreconditionError
		require.True(t, errors.As(err, &preErr))
		assert.Equal(t, cnErr.ErrCodeCRMEncryptKeyNotConfigured.Error(), preErr.Code)
	})

	t.Run("missing hash key", func(t *testing.T) {
		_, err := DecryptRecords(records, []string{"document"}, "", testEncryptKey, testLogger())
		require.Error(t, err)

		var preErr pkgErr.FailedPreconditionError
		require.True(t, errors.As(err, &preErr))
		assert.Equal(t, cnErr.ErrCodeCRMHashKeyNotConfigured.Error(), preErr.Code)
	})
}

func TestDecryptRecords_RoundTrip(t *testing.T) {
	crypto := &libCrypto.Crypto{EncryptSecretKey: testEncryptKey, Logger: testLogger()}
	require.NoError(t, crypto.InitializeCipher())

	enc := func(plain string) string {
		c, err := crypto.Encrypt(&plain)
		require.NoError(t, err)

		return *c
	}

	// A record exercising a top-level encrypted field, every nested encrypted
	// object, and the related_parties array path.
	records := []map[string]any{
		{
			"document": enc("12345678900"),
			"name":     enc("Jane Doe"),
			"status":   "ACTIVE", // not encrypted, must pass through
			"contact": map[string]any{
				"primary_email": enc("jane@example.com"),
				"mobile_phone":  enc("+5511999990000"),
			},
			"banking_details": map[string]any{
				"account": enc("00012345"),
				"iban":    enc("BR1500000000000010932840814P2"),
			},
			"legal_person": map[string]any{
				"representative": map[string]any{
					"name":     enc("Rep Name"),
					"document": enc("99988877766"),
					"email":    enc("rep@example.com"),
				},
			},
			"natural_person": map[string]any{
				"mother_name": enc("Mother Name"),
				"father_name": enc("Father Name"),
			},
			"regulatory_fields": map[string]any{
				"participant_document": enc("55544433322"),
			},
			"related_parties": []any{
				map[string]any{"document": enc("11122233344")},
				map[string]any{"document": enc("55566677788")},
			},
		},
	}

	out, err := DecryptRecords(records, []string{"document", "name", "contact.primary_email"}, testHashKey, testEncryptKey, testLogger())
	require.NoError(t, err)
	require.Len(t, out, 1)

	rec := out[0]
	assert.Equal(t, "12345678900", rec["document"])
	assert.Equal(t, "Jane Doe", rec["name"])
	assert.Equal(t, "ACTIVE", rec["status"])

	contact := rec["contact"].(map[string]any)
	assert.Equal(t, "jane@example.com", contact["primary_email"])
	assert.Equal(t, "+5511999990000", contact["mobile_phone"])

	banking := rec["banking_details"].(map[string]any)
	assert.Equal(t, "00012345", banking["account"])
	assert.Equal(t, "BR1500000000000010932840814P2", banking["iban"])

	rep := rec["legal_person"].(map[string]any)["representative"].(map[string]any)
	assert.Equal(t, "Rep Name", rep["name"])
	assert.Equal(t, "99988877766", rep["document"])
	assert.Equal(t, "rep@example.com", rep["email"])

	natural := rec["natural_person"].(map[string]any)
	assert.Equal(t, "Mother Name", natural["mother_name"])
	assert.Equal(t, "Father Name", natural["father_name"])

	reg := rec["regulatory_fields"].(map[string]any)
	assert.Equal(t, "55544433322", reg["participant_document"])

	parties := rec["related_parties"].([]any)
	require.Len(t, parties, 2)
	assert.Equal(t, "11122233344", parties[0].(map[string]any)["document"])
	assert.Equal(t, "55566677788", parties[1].(map[string]any)["document"])
}

func TestDecryptRecords_DottedFieldTriggersDecryptionOfTopLevel(t *testing.T) {
	crypto := &libCrypto.Crypto{EncryptSecretKey: testEncryptKey, Logger: testLogger()}
	require.NoError(t, crypto.InitializeCipher())

	plain := "secret-doc"
	enc, err := crypto.Encrypt(&plain)
	require.NoError(t, err)

	records := []map[string]any{{"document": *enc}}

	// Only a dotted field is requested; needsDecryption returns true, and the
	// top-level encrypted "document" still gets decrypted.
	out, err := DecryptRecords(records, []string{"contact.primary_email"}, testHashKey, testEncryptKey, testLogger())
	require.NoError(t, err)
	assert.Equal(t, "secret-doc", out[0]["document"])
}

// fakeQuerier is a CollectionQuerier backed by in-memory maps, returning
// collections in a deliberately unsorted order to prove the fan-out sorts. It
// records the filters it was handed per physical collection so a test can assert
// the transformed advanced filters reach every org query.
type fakeQuerier struct {
	names      []string
	rows       map[string][]map[string]any
	err        error
	gotFilters map[string]map[string]model.FilterCondition
}

func (f *fakeQuerier) ListCollectionNames(_ context.Context) ([]string, error) {
	return f.names, f.err
}

func (f *fakeQuerier) QueryCollection(_ context.Context, physical string, _ []string, filters map[string]model.FilterCondition) ([]map[string]any, error) {
	if f.gotFilters == nil {
		f.gotFilters = make(map[string]map[string]model.FilterCondition)
	}

	f.gotFilters[physical] = filters

	return f.rows[physical], nil
}

func TestFanOutOrgCollections_MergesSortedWithOrgInjection(t *testing.T) {
	q := &fakeQuerier{
		// Unsorted on purpose; orgB before orgA; an unrelated collection that
		// must NOT match the holders_ prefix.
		names: []string{"holders_orgB", "accounts_orgA", "holders_orgA"},
		rows: map[string][]map[string]any{
			"holders_orgA": {{"document": "A1"}, {"document": "A2"}},
			"holders_orgB": {{"document": "B1"}},
		},
	}

	out, err := FanOutOrgCollections(context.Background(), q, "holders", []string{"document"}, nil)
	require.NoError(t, err)
	require.Len(t, out, 3)

	// Sorted by physical collection name: holders_orgA rows first, then orgB.
	assert.Equal(t, "A1", out[0]["document"])
	assert.Equal(t, "orgA", out[0]["organization_id"])
	assert.Equal(t, "A2", out[1]["document"])
	assert.Equal(t, "orgA", out[1]["organization_id"])
	assert.Equal(t, "B1", out[2]["document"])
	assert.Equal(t, "orgB", out[2]["organization_id"])
}

func TestFanOutOrgCollections_NoMatchesYieldsEmpty(t *testing.T) {
	q := &fakeQuerier{names: []string{"accounts_orgA", "other"}}

	out, err := FanOutOrgCollections(context.Background(), q, "holders", []string{"document"}, nil)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestFanOutOrgCollections_ListErrorWraps(t *testing.T) {
	sentinel := errors.New("mongo down")
	q := &fakeQuerier{err: sentinel}

	_, err := FanOutOrgCollections(context.Background(), q, "holders", nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sentinel))
}

// TestFanOutOrgCollections_AppliesFiltersToEveryOrgCollection locks the
// regression fix: the transformed advanced filters must reach EACH physical org
// collection. Without this, a filtered plugin_crm report silently returns every
// org collection's full row set.
func TestFanOutOrgCollections_AppliesFiltersToEveryOrgCollection(t *testing.T) {
	q := &fakeQuerier{
		names: []string{"holders_orgB", "holders_orgA"},
		rows: map[string][]map[string]any{
			"holders_orgA": {{"document": "A1"}},
			"holders_orgB": {{"document": "B1"}},
		},
	}

	filters := map[string]model.FilterCondition{
		"search.document": {Equals: []any{"hashed-value"}},
	}

	_, err := FanOutOrgCollections(context.Background(), q, "holders", []string{"document"}, filters)
	require.NoError(t, err)

	// Every matched physical collection received the exact transformed filters.
	require.Len(t, q.gotFilters, 2)
	assert.Equal(t, filters, q.gotFilters["holders_orgA"])
	assert.Equal(t, filters, q.gotFilters["holders_orgB"])
}

// TestAdvancedFilterMappings_Stable locks the field-mapping table so a change to
// the plugin's stored search index contract is a deliberate, reviewed edit.
func TestAdvancedFilterMappings_Stable(t *testing.T) {
	want := map[string]string{
		"document":                               "search.document",
		"name":                                   "search.name",
		"banking_details.account":                "search.banking_details_account",
		"banking_details.iban":                   "search.banking_details_iban",
		"contact.primary_email":                  "search.contact_primary_email",
		"contact.secondary_email":                "search.contact_secondary_email",
		"contact.mobile_phone":                   "search.contact_mobile_phone",
		"contact.other_phone":                    "search.contact_other_phone",
		"regulatory_fields.participant_document": "search.regulatory_fields_participant_document",
		"related_parties.document":               "search.related_party_documents",
	}

	assert.Equal(t, want, advancedFilterFieldMappings)

	keys := make([]string, 0, len(advancedFilterFieldMappings))
	for k := range advancedFilterFieldMappings {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	assert.Len(t, keys, 10)
}
