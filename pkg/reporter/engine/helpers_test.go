// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// accountsSnapshot is a discovered schema for the qualified accounts table used
// across the filter-translation unit tests.
func accountsSnapshot(qualified string, fields ...string) fetcher.SchemaSnapshot {
	return fetcher.SchemaSnapshot{
		ConfigName: "ledger",
		Tables:     []fetcher.TableSnapshot{{Name: qualified, Fields: fields}},
	}
}

func TestDecodeMaybeJSON(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]any{"k": "v"}, decodeMaybeJSON([]byte(`{"k":"v"}`)))
	assert.Equal(t, []any{1.0, 2.0}, decodeMaybeJSON([]byte(`[1,2]`)))
	assert.Equal(t, "hello", decodeMaybeJSON([]byte(`"hello"`)))
	// Non-JSON bytes fall back to the raw value.
	raw := []byte("not json")
	assert.Equal(t, raw, decodeMaybeJSON(raw))
	// Non-byte values pass through untouched.
	assert.Equal(t, 42, decodeMaybeJSON(42))
}

func TestSplitQualified(t *testing.T) {
	t.Parallel()

	schema, table := splitQualified("public.accounts")
	assert.Equal(t, "public", schema)
	assert.Equal(t, "accounts", table)

	schema, table = splitQualified("accounts")
	assert.Empty(t, schema)
	assert.Equal(t, "accounts", table)
}

func TestQuoteIdentifier_EscapesQuotes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `"weird""name"`, quoteIdentifier(`weird"name`))
}

func TestBuildPostgresSelect_QualifiedAndUnqualified(t *testing.T) {
	t.Parallel()

	q, _, err := buildPostgresSelect("public.accounts", []string{"id"}, nil, fetcher.SchemaSnapshot{})
	require.NoError(t, err)
	assert.Contains(t, q, `FROM "public"."accounts"`)

	q, _, err = buildPostgresSelect("accounts", []string{"*"}, nil, fetcher.SchemaSnapshot{})
	require.NoError(t, err)
	assert.Contains(t, q, `FROM "accounts"`)
	assert.Contains(t, q, "SELECT *")
}

func TestConvertBSONValue(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)

	doc := bson.M{
		"nested": bson.M{"inner": "v"},
		"list":   bson.A{"a", bson.M{"x": 1}},
		"date":   bson.NewDateTimeFromTime(now),
		"plain":  "string",
	}

	out := convertBSON(doc)

	assert.Equal(t, map[string]any{"inner": "v"}, out["nested"])
	assert.Equal(t, []any{"a", map[string]any{"x": 1}}, out["list"])
	assert.Equal(t, now.UTC(), out["date"].(time.Time).UTC())
	assert.Equal(t, "string", out["plain"])
}

func TestConvertBSONValue_DType(t *testing.T) {
	t.Parallel()

	out := convertBSONValue(bson.D{{Key: "k", Value: "v"}})
	assert.Equal(t, map[string]any{"k": "v"}, out)
}

func TestConvertBSONValue_BinaryUUID(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	out := convertBSONValue(bson.Binary{Subtype: 0x04, Data: id[:]})
	assert.Equal(t, id.String(), out)
}

func TestConvertBSONValue_BinaryNonUUIDToHex(t *testing.T) {
	t.Parallel()

	data := []byte{0xde, 0xad, 0xbe, 0xef}

	out := convertBSONValue(bson.Binary{Subtype: 0x00, Data: data})
	assert.Equal(t, hex.EncodeToString(data), out)
}

func TestConvertBSONValue_ObjectID(t *testing.T) {
	t.Parallel()

	oid := bson.NewObjectID()

	out := convertBSONValue(oid)
	assert.Equal(t, oid.Hex(), out)
}

func TestConvertBSONValue_DateTime(t *testing.T) {
	t.Parallel()

	ts := time.UnixMilli(1_700_000_000_000).UTC()

	out := convertBSONValue(bson.NewDateTimeFromTime(ts))
	assert.Equal(t, ts, out.(time.Time).UTC())
}

func TestConvertBSONValue_Nil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, convertBSONValue(nil))
}

func TestFiltersForDatasource(t *testing.T) {
	t.Parallel()

	// No filters at all yields nil, no error.
	got, err := filtersForDatasource("ledger", nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	// A filter for another datasource is ignored for this one.
	got, err = filtersForDatasource("ledger", map[string]any{
		"other": datasourceFilters{"accounts": {"status": {Equals: []any{"ACTIVE"}}}},
	})
	require.NoError(t, err)
	assert.Nil(t, got)

	// A well-shaped filter for this datasource is returned.
	want := datasourceFilters{"public.accounts": {"status": {Equals: []any{"ACTIVE"}}}}
	got, err = filtersForDatasource("ledger", map[string]any{"ledger": want})
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// A present-but-wrong-shaped filter fails closed with CategoryValidation,
	// rather than silently extracting the entire table.
	_, err = filtersForDatasource("ledger", map[string]any{
		"ledger": map[string]any{"accounts": map[string]any{"status": "ACTIVE"}},
	})
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestDatasourceFilters_TableFiltersMultiFormatMatch(t *testing.T) {
	t.Parallel()

	cond := map[string]model.FilterCondition{"status": {Equals: []any{"ACTIVE"}}}

	// Exact qualified key matches.
	qualified := datasourceFilters{"public.accounts": cond}
	assert.Equal(t, cond, qualified.tableFilters("public.accounts"))

	// Bare table key matches a qualified request.
	bare := datasourceFilters{"accounts": cond}
	assert.Equal(t, cond, bare.tableFilters("public.accounts"))

	// No match yields nil.
	assert.Nil(t, qualified.tableFilters("public.balances"))
	assert.Nil(t, datasourceFilters(nil).tableFilters("public.accounts"))
}

func TestApplyPostgresFilters_EqualitySingleAndMulti(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("public.accounts", "status")

	q, args, err := buildPostgresSelect("public.accounts", []string{"id"},
		map[string]model.FilterCondition{"status": {Equals: []any{"ACTIVE"}}}, schema)
	require.NoError(t, err)
	assert.Contains(t, q, "WHERE")
	assert.Contains(t, q, `status = $1`)
	assert.Equal(t, []any{"ACTIVE"}, args)

	q, args, err = buildPostgresSelect("public.accounts", []string{"id"},
		map[string]model.FilterCondition{"status": {Equals: []any{"ACTIVE", "PENDING"}}}, schema)
	require.NoError(t, err)
	assert.Contains(t, q, `status IN ($1,$2)`)
	assert.Equal(t, []any{"ACTIVE", "PENDING"}, args)
}

func TestApplyPostgresFilters_RangeOperators(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("public.accounts", "balance")

	q, args, err := buildPostgresSelect("public.accounts", []string{"id"},
		map[string]model.FilterCondition{"balance": {GreaterThan: []any{100}, LessOrEqual: []any{1000}}}, schema)
	require.NoError(t, err)
	assert.Contains(t, q, `balance > $`)
	assert.Contains(t, q, `balance <= $`)
	assert.ElementsMatch(t, []any{100, 1000}, args)
}

func TestApplyPostgresFilters_BetweenDateExpansion(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("public.accounts", "created_at")

	q, args, err := buildPostgresSelect("public.accounts", []string{"id"},
		map[string]model.FilterCondition{"created_at": {Between: []any{"2025-06-01", "2025-06-30"}}}, schema)
	require.NoError(t, err)
	assert.Contains(t, q, `created_at >= $`)
	assert.Contains(t, q, `created_at <= $`)
	// The date-only upper bound is expanded to end-of-day so the range is
	// inclusive of the whole end day.
	assert.Contains(t, args, "2025-06-01")
	assert.Contains(t, args, "2025-06-30T23:59:59.999Z")
}

func TestApplyPostgresFilters_UnknownFieldErrors(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("public.accounts", "status")

	_, _, err := buildPostgresSelect("public.accounts", []string{"id"},
		map[string]model.FilterCondition{"ghost": {Equals: []any{"x"}}}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestBuildMongoFilter_EqualitySingleAndMulti(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("holders", "status")

	got, err := buildMongoFilter("holders", map[string]model.FilterCondition{"status": {Equals: []any{"ACTIVE"}}}, schema)
	require.NoError(t, err)
	assert.Equal(t, bson.M{"status": map[string]any{"$eq": "ACTIVE"}}, got)

	got, err = buildMongoFilter("holders", map[string]model.FilterCondition{"status": {Equals: []any{"ACTIVE", "PENDING"}}}, schema)
	require.NoError(t, err)
	assert.Equal(t, bson.M{"status": map[string]any{"$in": []any{"ACTIVE", "PENDING"}}}, got)
}

func TestBuildMongoFilter_RangeAndBetween(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("holders", "balance", "created_at")

	got, err := buildMongoFilter("holders", map[string]model.FilterCondition{
		"balance":    {GreaterThan: []any{100}, LessOrEqual: []any{1000}},
		"created_at": {Between: []any{"2025-06-01", "2025-06-30"}},
	}, schema)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"$gt": 100, "$lte": 1000}, got["balance"])
	// Mongo Between maps to $gte/$lte with no end-of-day expansion (mirrors the
	// legacy mongo path).
	assert.Equal(t, map[string]any{"$gte": "2025-06-01", "$lte": "2025-06-30"}, got["created_at"])
}

func TestBuildMongoFilter_UnknownFieldErrors(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("holders", "status")

	_, err := buildMongoFilter("holders", map[string]model.FilterCondition{"ghost": {Equals: []any{"x"}}}, schema)
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestApplyPostgresFilters_MalformedBetweenRejected(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("public.accounts", "created_at")

	// A Between with one value (or three) must fail closed rather than being
	// silently dropped, which would leave created_at unfiltered.
	for _, between := range [][]any{{"2025-06-01"}, {"2025-06-01", "2025-06-15", "2025-06-30"}} {
		_, _, err := buildPostgresSelect("public.accounts", []string{"id"},
			map[string]model.FilterCondition{"created_at": {Between: between}}, schema)
		require.Error(t, err)

		var engineErr *fetcher.EngineError
		require.ErrorAs(t, err, &engineErr)
		assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
	}
}

func TestApplyPostgresFilters_MultiValueSingleOpRejected(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("public.accounts", "balance")

	_, _, err := buildPostgresSelect("public.accounts", []string{"id"},
		map[string]model.FilterCondition{"balance": {GreaterThan: []any{100, 9999}}}, schema)
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestApplyPostgresFilters_BetweenNonDateStartNoExpansion(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("public.accounts", "created_at")

	// A Between whose start is not a date string must NOT expand the upper bound,
	// matching the legacy builder that gated expansion on both ends being dates.
	q, args, err := buildPostgresSelect("public.accounts", []string{"id"},
		map[string]model.FilterCondition{"created_at": {Between: []any{42, "2025-06-30"}}}, schema)
	require.NoError(t, err)
	assert.Contains(t, q, `created_at <= $`)
	assert.Contains(t, args, "2025-06-30")
	assert.NotContains(t, args, "2025-06-30T23:59:59.999Z")
}

func TestBuildMongoFilter_MalformedBetweenRejected(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("holders", "created_at")

	_, err := buildMongoFilter("holders",
		map[string]model.FilterCondition{"created_at": {Between: []any{"2025-06-01"}}}, schema)
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestBuildMongoFilter_MultiValueSingleOpRejected(t *testing.T) {
	t.Parallel()

	schema := accountsSnapshot("holders", "balance")

	_, err := buildMongoFilter("holders",
		map[string]model.FilterCondition{"balance": {GreaterThan: []any{100, 200}}}, schema)
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestBuildMongoFilter_EmptyCollectionSkipsFieldValidation(t *testing.T) {
	t.Parallel()

	// An existing-but-empty collection samples no fields. The filter must still
	// apply (mirroring the legacy count==0 short-circuit), not be rejected as an
	// unknown field.
	emptyCollection := fetcher.SchemaSnapshot{
		ConfigName: "crm",
		Tables:     []fetcher.TableSnapshot{{Name: "holders", Fields: nil}},
	}

	got, err := buildMongoFilter("holders",
		map[string]model.FilterCondition{"status": {Equals: []any{"ACTIVE"}}}, emptyCollection)
	require.NoError(t, err)
	assert.Equal(t, bson.M{"status": map[string]any{"$eq": "ACTIVE"}}, got)
}

func TestBuildMongoFilter_MissingCollectionStillRejects(t *testing.T) {
	t.Parallel()

	// A collection absent from the snapshot does not exist; its filter fields are
	// still rejected (distinct from the empty-collection short-circuit).
	schema := accountsSnapshot("other", "status")

	_, err := buildMongoFilter("holders",
		map[string]model.FilterCondition{"status": {Equals: []any{"ACTIVE"}}}, schema)
	require.Error(t, err)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryValidation, engineErr.Category)
}

func TestIsDateFieldAndString(t *testing.T) {
	t.Parallel()

	assert.True(t, isDateField("created_at"))
	assert.True(t, isDateField("timestamp"))
	assert.False(t, isDateField("balance"))

	assert.True(t, isDateString("2025-06-01"))
	assert.True(t, isDateString("2025-06-01T12:00:00Z"))
	assert.False(t, isDateString("not-a-date"))
	assert.False(t, isDateString(42))
}

func TestFilterNestedFields_DropsChildOfSelectedParent(t *testing.T) {
	t.Parallel()

	out := filterNestedFields([]string{"contact", "contact.email", "name"})
	assert.Equal(t, []string{"contact", "name"}, out)
}

func TestMongoProjection_Star(t *testing.T) {
	t.Parallel()

	// A "*" selection sets no projection (builder remains default).
	opts := mongoProjection([]string{"*"})
	assert.NotNil(t, opts)
}

func TestSchemaOverride_AnySlice(t *testing.T) {
	t.Parallel()

	desc := fetcher.ConnectionDescriptor{
		HostAttributes: map[string]any{
			hostAttrSchemas: []any{"public", "audit", 123},
		},
	}

	assert.Equal(t, []string{"public", "audit"}, schemaOverrideFromDescriptor(desc))
}

func TestSchemaOverride_AbsentReturnsNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, schemaOverrideFromDescriptor(fetcher.ConnectionDescriptor{}))
}

func TestSingleTenantResolver_ResolveMongoError(t *testing.T) {
	t.Parallel()

	ds := &fakeSingleTenantDatasources{mongoErr: errors.New("down")}
	r := NewSingleTenantResolver(ds)

	_, err := r.ResolveMongo(context.Background(), "", "crm")
	require.Error(t, err)
}

func TestMultiTenantResolver_ResolveMongoForwardsTenant(t *testing.T) {
	t.Parallel()

	mgr := &fakeMongoManager{err: errors.New("no db")}
	r := NewMultiTenantResolver(&fakePGManager{}, mgr, nil)

	_, err := r.ResolveMongo(context.Background(), "tenant-q", "crm")
	require.Error(t, err)
	assert.Equal(t, "tenant-q", mgr.gotTenant)

	var engineErr *fetcher.EngineError
	require.ErrorAs(t, err, &engineErr)
	assert.Equal(t, fetcher.CategoryUnavailable, engineErr.Category)
}

func TestClassifyQueryError_TimeoutAndDefault(t *testing.T) {
	t.Parallel()

	timeoutErr := classifyQueryError(context.Background(), "q", context.DeadlineExceeded)
	var engineErr *fetcher.EngineError
	require.ErrorAs(t, timeoutErr, &engineErr)
	assert.Equal(t, fetcher.CategoryTimeout, engineErr.Category)

	other := classifyQueryError(context.Background(), "q", errors.New("boom"))
	require.ErrorAs(t, other, &engineErr)
	assert.Equal(t, fetcher.CategoryUnavailable, engineErr.Category)
}
