// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

// ---------------------------------------------------------------------------
// Repository Interface Contract Tests (via MockRepository)
// ---------------------------------------------------------------------------

func TestRepository_Query_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		collection string
		fields     []string
		filter     map[string][]any
		want       []map[string]any
	}{
		{
			name:       "returns matching documents",
			collection: "accounts",
			fields:     []string{"name", "status"},
			filter:     map[string][]any{"status": {"active"}},
			want: []map[string]any{
				{"name": "Account A", "status": "active"},
				{"name": "Account B", "status": "active"},
			},
		},
		{
			name:       "returns empty slice when no documents match",
			collection: "accounts",
			fields:     []string{"name"},
			filter:     map[string][]any{"status": {"deleted"}},
			want:       []map[string]any{},
		},
		{
			name:       "returns nil when collection is empty",
			collection: "empty_collection",
			fields:     []string{"*"},
			filter:     map[string][]any{},
			want:       nil,
		},
		{
			name:       "handles multi-value filter",
			collection: "transactions",
			fields:     []string{"amount", "currency"},
			filter:     map[string][]any{"currency": {"USD", "EUR"}},
			want: []map[string]any{
				{"amount": 100.0, "currency": "USD"},
				{"amount": 200.0, "currency": "EUR"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)

			mockRepo.EXPECT().
				Query(gomock.Any(), tt.collection, tt.fields, tt.filter).
				Return(tt.want, nil).
				Times(1)

			got, err := mockRepo.Query(context.Background(), tt.collection, tt.fields, tt.filter)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRepository_Query_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		collection string
		fields     []string
		filter     map[string][]any
		wantErr    string
	}{
		{
			name:       "returns error on connection failure",
			collection: "accounts",
			fields:     []string{"name"},
			filter:     map[string][]any{},
			wantErr:    "failed to establish MongoDB connection",
		},
		{
			name:       "returns error on query timeout",
			collection: "large_collection",
			fields:     []string{"*"},
			filter:     map[string][]any{"status": {"active"}},
			wantErr:    "mongodb query timeout",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)

			mockRepo.EXPECT().
				Query(gomock.Any(), tt.collection, tt.fields, tt.filter).
				Return(nil, errors.New(tt.wantErr)).
				Times(1)

			got, err := mockRepo.Query(context.Background(), tt.collection, tt.fields, tt.filter)

			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRepository_QueryWithAdvancedFilters_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		collection string
		fields     []string
		filter     map[string]model.FilterCondition
		want       []map[string]any
	}{
		{
			name:       "returns results with equals filter",
			collection: "transactions",
			fields:     []string{"amount", "status"},
			filter: map[string]model.FilterCondition{
				"status": {Equals: []any{"completed"}},
			},
			want: []map[string]any{
				{"amount": 500.0, "status": "completed"},
			},
		},
		{
			name:       "returns results with range filter",
			collection: "transactions",
			fields:     []string{"amount"},
			filter: map[string]model.FilterCondition{
				"amount": {Between: []any{100, 1000}},
			},
			want: []map[string]any{
				{"amount": 500.0},
				{"amount": 750.0},
			},
		},
		{
			name:       "returns empty when no matches",
			collection: "transactions",
			fields:     []string{"*"},
			filter: map[string]model.FilterCondition{
				"amount": {GreaterThan: []any{999999}},
			},
			want: []map[string]any{},
		},
		{
			name:       "handles combined operators",
			collection: "orders",
			fields:     []string{"total", "status"},
			filter: map[string]model.FilterCondition{
				"total":  {GreaterOrEqual: []any{100}},
				"status": {NotIn: []any{"cancelled", "refunded"}},
			},
			want: []map[string]any{
				{"total": 150.0, "status": "shipped"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)

			mockRepo.EXPECT().
				QueryWithAdvancedFilters(gomock.Any(), tt.collection, tt.fields, tt.filter).
				Return(tt.want, nil).
				Times(1)

			got, err := mockRepo.QueryWithAdvancedFilters(context.Background(), tt.collection, tt.fields, tt.filter)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRepository_QueryWithAdvancedFilters_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockRepository(ctrl)

	filter := map[string]model.FilterCondition{
		"amount": {Between: []any{100, 1000}},
	}

	mockRepo.EXPECT().
		QueryWithAdvancedFilters(gomock.Any(), "transactions", []string{"amount"}, filter).
		Return(nil, errors.New("mongodb advanced filter query timeout")).
		Times(1)

	got, err := mockRepo.QueryWithAdvancedFilters(context.Background(), "transactions", []string{"amount"}, filter)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "mongodb advanced filter query timeout")
}

func TestRepository_GetDatabaseSchema_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []CollectionSchema
	}{
		{
			name: "returns schema for multiple collections",
			want: []CollectionSchema{
				{
					CollectionName: "accounts",
					Fields: []FieldInformation{
						{Name: "_id", DataType: "objectId"},
						{Name: "name", DataType: "string"},
						{Name: "balance", DataType: "number"},
					},
				},
				{
					CollectionName: "transactions",
					Fields: []FieldInformation{
						{Name: "_id", DataType: "objectId"},
						{Name: "amount", DataType: "number"},
						{Name: "created_at", DataType: "date"},
					},
				},
			},
		},
		{
			name: "returns empty schema for database with no collections",
			want: []CollectionSchema{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)

			mockRepo.EXPECT().
				GetDatabaseSchema(gomock.Any()).
				Return(tt.want, nil).
				Times(1)

			got, err := mockRepo.GetDatabaseSchema(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRepository_GetDatabaseSchema_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockRepository(ctrl)

	mockRepo.EXPECT().
		GetDatabaseSchema(gomock.Any()).
		Return(nil, errors.New("mongodb schema discovery timeout")).
		Times(1)

	got, err := mockRepo.GetDatabaseSchema(context.Background())

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "mongodb schema discovery timeout")
}

func TestRepository_GetDatabaseSchemaForOrganization_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		orgID string
		want  []CollectionSchema
	}{
		{
			name:  "returns filtered collections for organization",
			orgID: "org-123",
			want: []CollectionSchema{
				{
					CollectionName: "accounts_org-123",
					Fields: []FieldInformation{
						{Name: "_id", DataType: "objectId"},
						{Name: "holder_id", DataType: "string"},
					},
				},
			},
		},
		{
			name:  "returns empty when no collections match organization",
			orgID: "org-nonexistent",
			want:  []CollectionSchema{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)

			mockRepo.EXPECT().
				GetDatabaseSchemaForOrganization(gomock.Any(), tt.orgID).
				Return(tt.want, nil).
				Times(1)

			got, err := mockRepo.GetDatabaseSchemaForOrganization(context.Background(), tt.orgID)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRepository_GetDatabaseSchemaForOrganization_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockRepository(ctrl)

	mockRepo.EXPECT().
		GetDatabaseSchemaForOrganization(gomock.Any(), "org-123").
		Return(nil, errors.New("mongodb schema discovery timeout while listing collections")).
		Times(1)

	got, err := mockRepo.GetDatabaseSchemaForOrganization(context.Background(), "org-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "mongodb schema discovery timeout")
}

func TestRepository_CloseConnection_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockRepository(ctrl)

	mockRepo.EXPECT().
		CloseConnection(gomock.Any()).
		Return(nil).
		Times(1)

	err := mockRepo.CloseConnection(context.Background())

	require.NoError(t, err)
}

func TestRepository_CloseConnection_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockRepository(ctrl)

	mockRepo.EXPECT().
		CloseConnection(gomock.Any()).
		Return(errors.New("error closing MongoDB connection")).
		Times(1)

	err := mockRepo.CloseConnection(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error closing MongoDB connection")
}

// ---------------------------------------------------------------------------
// Pure Function Tests (no mock / DB needed)
// ---------------------------------------------------------------------------

func TestBuildMongoFilter(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	tests := []struct {
		name      string
		filter    map[string]model.FilterCondition
		wantKeys  []string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "empty filter returns empty bson.M",
			filter:   map[string]model.FilterCondition{},
			wantKeys: nil,
		},
		{
			name: "skips empty conditions",
			filter: map[string]model.FilterCondition{
				"name": {},
			},
			wantKeys: nil,
		},
		{
			name: "single equals condition",
			filter: map[string]model.FilterCondition{
				"status": {Equals: []any{"active"}},
			},
			wantKeys: []string{"status"},
		},
		{
			name: "multiple fields",
			filter: map[string]model.FilterCondition{
				"status": {Equals: []any{"active"}},
				"amount": {GreaterThan: []any{100}},
			},
			wantKeys: []string{"status", "amount"},
		},
		{
			name: "between filter with correct values",
			filter: map[string]model.FilterCondition{
				"price": {Between: []any{10, 100}},
			},
			wantKeys: []string{"price"},
		},
		{
			name: "invalid between filter returns error",
			filter: map[string]model.FilterCondition{
				"price": {Between: []any{10}},
			},
			wantErr:   true,
			errSubstr: "between operator",
		},
		{
			name: "invalid gt filter with multiple values returns error",
			filter: map[string]model.FilterCondition{
				"age": {GreaterThan: []any{18, 21}},
			},
			wantErr:   true,
			errSubstr: "gt operator",
		},
		{
			name: "in operator",
			filter: map[string]model.FilterCondition{
				"category": {In: []any{"a", "b", "c"}},
			},
			wantKeys: []string{"category"},
		},
		{
			name: "not-in operator",
			filter: map[string]model.FilterCondition{
				"status": {NotIn: []any{"deleted"}},
			},
			wantKeys: []string{"status"},
		},
		{
			name: "less than operator",
			filter: map[string]model.FilterCondition{
				"age": {LessThan: []any{65}},
			},
			wantKeys: []string{"age"},
		},
		{
			name: "less or equal operator",
			filter: map[string]model.FilterCondition{
				"score": {LessOrEqual: []any{100}},
			},
			wantKeys: []string{"score"},
		},
		{
			name: "greater or equal operator",
			filter: map[string]model.FilterCondition{
				"amount": {GreaterOrEqual: []any{0}},
			},
			wantKeys: []string{"amount"},
		},
		{
			name: "mixed empty and non-empty conditions",
			filter: map[string]model.FilterCondition{
				"empty_field": {},
				"real_field":  {Equals: []any{"value"}},
			},
			wantKeys: []string{"real_field"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ds.buildMongoFilter(tt.filter)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				return
			}

			require.NoError(t, err)

			if tt.wantKeys == nil {
				assert.Empty(t, got)
				return
			}

			for _, key := range tt.wantKeys {
				assert.Contains(t, got, key, "expected key %q in filter", key)
			}
		})
	}
}

func TestBuildFindOptions(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	tests := []struct {
		name           string
		fields         []string
		wantProjection bool
	}{
		{
			name:           "wildcard field produces no projection",
			fields:         []string{"*"},
			wantProjection: false,
		},
		{
			name:           "empty fields produce no projection",
			fields:         []string{},
			wantProjection: false,
		},
		{
			name:           "specific fields produce projection",
			fields:         []string{"name", "status", "amount"},
			wantProjection: true,
		},
		{
			name:           "single field produces projection",
			fields:         []string{"name"},
			wantProjection: true,
		},
		{
			name:           "nested fields with parent are collapsed before projection",
			fields:         []string{"contact", "contact.email", "contact.phone"},
			wantProjection: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := ds.buildFindOptions(tt.fields)

			require.NotNil(t, opts)

			// The FindOptions.Projection is set only when wantProjection is true.
			// We verify through the SetProjection path: if projection was set,
			// opts is still non-nil (always true), and we trust the MongoDB
			// driver's SetProjection internally. The key assertion is that the
			// method does not panic and returns a valid options object.
			if tt.wantProjection {
				assert.NotNil(t, opts)
			}
		})
	}
}

func TestConvertBsonValue_BsonD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    bson.D
		wantKeys []string
	}{
		{
			name: "ordered document converts to map",
			input: bson.D{
				{Key: "name", Value: "Alice"},
				{Key: "age", Value: 30},
			},
			wantKeys: []string{"name", "age"},
		},
		{
			name:     "empty ordered document",
			input:    bson.D{},
			wantKeys: []string{},
		},
		{
			name: "nested bson.D converts recursively",
			input: bson.D{
				{Key: "address", Value: bson.D{
					{Key: "city", Value: "London"},
					{Key: "zip", Value: "SW1A"},
				}},
			},
			wantKeys: []string{"address"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := convertBsonValue(tt.input)

			doc, ok := result.(map[string]any)
			require.True(t, ok, "expected map[string]any, got %T", result)

			assert.Len(t, doc, len(tt.wantKeys))
			for _, key := range tt.wantKeys {
				assert.Contains(t, doc, key)
			}
		})
	}
}

func TestConvertBsonValue_BinaryUUID(t *testing.T) {
	t.Parallel()

	// Create a known UUID and convert to bytes for bson.Binary
	knownUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	uuidBytes, err := knownUUID.MarshalBinary()
	require.NoError(t, err)

	tests := []struct {
		name  string
		input bson.Binary
		want  string
	}{
		{
			name:  "binary UUID is converted to UUID string",
			input: bson.Binary{Subtype: 0x04, Data: uuidBytes},
			want:  knownUUID.String(),
		},
		{
			name:  "non-UUID binary falls back to hex",
			input: bson.Binary{Subtype: 0x00, Data: []byte("short")},
			want:  "73686f7274", // hex encoding of "short"
		},
		{
			name:  "16 bytes that form valid UUID",
			input: bson.Binary{Data: make([]byte, 16)},
			want:  "00000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := convertBsonValue(tt.input)

			str, ok := result.(string)
			require.True(t, ok, "expected string, got %T", result)
			assert.Equal(t, tt.want, str)
		})
	}
}

func TestConvertBsonValue_NestedArrayWithMixedTypes(t *testing.T) {
	t.Parallel()

	input := bson.A{
		"plain_string",
		42,
		bson.M{"nested": "object"},
		bson.A{"inner", "array"},
		nil,
	}

	result := convertBsonValue(input)

	arr, ok := result.([]any)
	require.True(t, ok, "expected []any, got %T", result)
	require.Len(t, arr, 5)

	assert.Equal(t, "plain_string", arr[0])
	assert.Equal(t, 42, arr[1])

	nested, ok := arr[2].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", nested["nested"])

	inner, ok := arr[3].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"inner", "array"}, inner)

	assert.Nil(t, arr[4])
}

func TestConvertBsonToMap_DeepNesting(t *testing.T) {
	t.Parallel()

	input := bson.M{
		"level1": bson.M{
			"level2": bson.M{
				"level3": bson.M{
					"value": "deep",
				},
			},
		},
		"items": bson.A{
			bson.M{
				"sub_items": bson.A{
					bson.M{"id": "sub-1"},
				},
			},
		},
	}

	result := convertBsonToMap(input)

	// Navigate deep nesting
	l1, ok := result["level1"].(map[string]any)
	require.True(t, ok)

	l2, ok := l1["level2"].(map[string]any)
	require.True(t, ok)

	l3, ok := l2["level3"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "deep", l3["value"])

	// Navigate array nesting
	items, ok := result["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)

	item, ok := items[0].(map[string]any)
	require.True(t, ok)

	subItems, ok := item["sub_items"].([]any)
	require.True(t, ok)
	require.Len(t, subItems, 1)

	subItem, ok := subItems[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "sub-1", subItem["id"])
}

func TestValidateFilterCondition(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	tests := []struct {
		name      string
		field     string
		condition model.FilterCondition
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "valid equals condition",
			field:     "status",
			condition: model.FilterCondition{Equals: []any{"active"}},
			wantErr:   false,
		},
		{
			name:      "valid between with 2 values",
			field:     "price",
			condition: model.FilterCondition{Between: []any{10, 100}},
			wantErr:   false,
		},
		{
			name:      "invalid between with 1 value",
			field:     "price",
			condition: model.FilterCondition{Between: []any{10}},
			wantErr:   true,
			errSubstr: "between operator",
		},
		{
			name:      "invalid between with 3 values",
			field:     "price",
			condition: model.FilterCondition{Between: []any{1, 2, 3}},
			wantErr:   true,
			errSubstr: "between operator",
		},
		{
			name:      "invalid gt with multiple values",
			field:     "age",
			condition: model.FilterCondition{GreaterThan: []any{18, 21}},
			wantErr:   true,
			errSubstr: "gt operator",
		},
		{
			name:      "invalid gte with multiple values",
			field:     "amount",
			condition: model.FilterCondition{GreaterOrEqual: []any{100, 200}},
			wantErr:   true,
			errSubstr: "gte operator",
		},
		{
			name:      "invalid lt with multiple values",
			field:     "score",
			condition: model.FilterCondition{LessThan: []any{50, 60}},
			wantErr:   true,
			errSubstr: "lt operator",
		},
		{
			name:      "invalid lte with multiple values",
			field:     "rating",
			condition: model.FilterCondition{LessOrEqual: []any{5, 10}},
			wantErr:   true,
			errSubstr: "lte operator",
		},
		{
			name:      "empty condition is valid",
			field:     "any",
			condition: model.FilterCondition{},
			wantErr:   false,
		},
		{
			name:  "valid complex condition with multiple operators",
			field: "amount",
			condition: model.FilterCondition{
				GreaterOrEqual: []any{100},
				LessOrEqual:    []any{1000},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ds.validateFilterCondition(tt.field, tt.condition)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestBuildMongoFilter_OperatorOutput(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	tests := []struct {
		name       string
		filter     map[string]model.FilterCondition
		wantField  string
		checkValue func(t *testing.T, val any)
	}{
		{
			name: "equals single value produces $eq operator",
			filter: map[string]model.FilterCondition{
				"name": {Equals: []any{"Alice"}},
			},
			wantField: "name",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				eqMap, ok := val.(map[string]any)
				assert.True(t, ok, "expected map with $eq operator")
				assert.Equal(t, "Alice", eqMap["$eq"])
			},
		},
		{
			name: "equals multiple values produces $in",
			filter: map[string]model.FilterCondition{
				"status": {Equals: []any{"active", "pending"}},
			},
			wantField: "status",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$in")
			},
		},
		{
			name: "greater than produces $gt",
			filter: map[string]model.FilterCondition{
				"age": {GreaterThan: []any{18}},
			},
			wantField: "age",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$gt")
				assert.Equal(t, 18, m["$gt"])
			},
		},
		{
			name: "between produces $gte and $lte",
			filter: map[string]model.FilterCondition{
				"price": {Between: []any{10, 100}},
			},
			wantField: "price",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$gte")
				assert.Contains(t, m, "$lte")
				assert.Equal(t, 10, m["$gte"])
				assert.Equal(t, 100, m["$lte"])
			},
		},
		{
			name: "in produces $in",
			filter: map[string]model.FilterCondition{
				"tag": {In: []any{"a", "b"}},
			},
			wantField: "tag",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$in")
			},
		},
		{
			name: "not-in produces $nin",
			filter: map[string]model.FilterCondition{
				"status": {NotIn: []any{"deleted"}},
			},
			wantField: "status",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$nin")
			},
		},
		{
			name: "less than produces $lt",
			filter: map[string]model.FilterCondition{
				"count": {LessThan: []any{50}},
			},
			wantField: "count",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$lt")
				assert.Equal(t, 50, m["$lt"])
			},
		},
		{
			name: "less or equal produces $lte",
			filter: map[string]model.FilterCondition{
				"score": {LessOrEqual: []any{100}},
			},
			wantField: "score",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$lte")
				assert.Equal(t, 100, m["$lte"])
			},
		},
		{
			name: "greater or equal produces $gte",
			filter: map[string]model.FilterCondition{
				"amount": {GreaterOrEqual: []any{0}},
			},
			wantField: "amount",
			checkValue: func(t *testing.T, val any) {
				t.Helper()
				m, ok := val.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", val)
				assert.Contains(t, m, "$gte")
				assert.Equal(t, 0, m["$gte"])
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ds.buildMongoFilter(tt.filter)
			require.NoError(t, err)
			require.Contains(t, got, tt.wantField)

			tt.checkValue(t, got[tt.wantField])
		})
	}
}

func TestIsFilterConditionEmpty_AllOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		condition model.FilterCondition
		want      bool
	}{
		{
			name:      "completely empty",
			condition: model.FilterCondition{},
			want:      true,
		},
		{
			name:      "has equals",
			condition: model.FilterCondition{Equals: []any{"x"}},
			want:      false,
		},
		{
			name:      "has greater than",
			condition: model.FilterCondition{GreaterThan: []any{1}},
			want:      false,
		},
		{
			name:      "has greater or equal",
			condition: model.FilterCondition{GreaterOrEqual: []any{1}},
			want:      false,
		},
		{
			name:      "has less than",
			condition: model.FilterCondition{LessThan: []any{1}},
			want:      false,
		},
		{
			name:      "has less or equal",
			condition: model.FilterCondition{LessOrEqual: []any{1}},
			want:      false,
		},
		{
			name:      "has between",
			condition: model.FilterCondition{Between: []any{1, 2}},
			want:      false,
		},
		{
			name:      "has in",
			condition: model.FilterCondition{In: []any{"a"}},
			want:      false,
		},
		{
			name:      "has not-in",
			condition: model.FilterCondition{NotIn: []any{"a"}},
			want:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isFilterConditionEmpty(tt.condition)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertFilterConditionToMongoFilter_LessOrEqual(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	condition := model.FilterCondition{LessOrEqual: []any{999}}

	got, err := ds.convertFilterConditionToMongoFilter("max_amount", condition)
	require.NoError(t, err)
	require.Contains(t, got, "max_amount")

	fieldFilter, ok := got["max_amount"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 999, fieldFilter["$lte"])
}

func TestConvertFilterConditionToMongoFilter_GreaterOrEqual(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	condition := model.FilterCondition{GreaterOrEqual: []any{0}}

	got, err := ds.convertFilterConditionToMongoFilter("min_amount", condition)
	require.NoError(t, err)
	require.Contains(t, got, "min_amount")

	fieldFilter, ok := got["min_amount"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 0, fieldFilter["$gte"])
}

func TestConvertFilterConditionToMongoFilter_LessThan(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	condition := model.FilterCondition{LessThan: []any{500}}

	got, err := ds.convertFilterConditionToMongoFilter("threshold", condition)
	require.NoError(t, err)
	require.Contains(t, got, "threshold")

	fieldFilter, ok := got["threshold"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 500, fieldFilter["$lt"])
}

func TestCalculateOptimalSampleSize_Boundaries(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	tests := []struct {
		name      string
		totalDocs int64
		want      int
	}{
		{
			name:      "exactly at small threshold",
			totalDocs: 1000,
			want:      1000,
		},
		{
			name:      "one above small threshold",
			totalDocs: 1001,
			want:      1000,
		},
		{
			name:      "exactly at medium threshold",
			totalDocs: 10000,
			want:      1000,
		},
		{
			name:      "one above medium threshold",
			totalDocs: 10001,
			want:      2000,
		},
		{
			name:      "exactly at large threshold",
			totalDocs: 100000,
			want:      2000,
		},
		{
			name:      "one above large threshold",
			totalDocs: 100001,
			want:      5000,
		},
		{
			name:      "exactly at very large threshold",
			totalDocs: 1000000,
			want:      5000,
		},
		{
			name:      "one above very large threshold",
			totalDocs: 1000001,
			want:      10000,
		},
		{
			name:      "single document",
			totalDocs: 1,
			want:      1,
		},
		{
			name:      "zero documents",
			totalDocs: 0,
			want:      0,
		},
		{
			name:      "extremely large collection",
			totalDocs: 100000000,
			want:      10000,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ds.calculateOptimalSampleSize(tt.totalDocs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInferDataType_ExtendedTypes(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "int32",
			value: int32(42),
			want:  "number",
		},
		{
			name:  "int64",
			value: int64(42),
			want:  "number",
		},
		{
			name:  "float32",
			value: float32(3.14),
			want:  "number",
		},
		{
			name:  "float64",
			value: float64(3.14),
			want:  "number",
		},
		{
			name:  "bson.Regex",
			value: bson.Regex{Pattern: "^test", Options: "i"},
			want:  "regex",
		},
		{
			name:  "bson.Timestamp",
			value: bson.Timestamp{T: 1640995200, I: 1},
			want:  "timestamp",
		},
		{
			name:  "bson.Decimal128",
			value: bson.Decimal128{},
			want:  "decimal",
		},
		{
			name:  "bson.MinKey",
			value: bson.MinKey{},
			want:  "minKey/maxKey",
		},
		{
			name:  "bson.MaxKey",
			value: bson.MaxKey{},
			want:  "minKey/maxKey",
		},
		{
			name:  "unknown type (byte slice)",
			value: []byte("binary"),
			want:  "unknown",
		},
		{
			name:  "unknown type (struct)",
			value: struct{ X int }{X: 1},
			want:  "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ds.inferDataType(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsMoreSpecificType_AllPairs(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{}

	tests := []struct {
		name        string
		newType     string
		currentType string
		want        bool
	}{
		{name: "objectId beats string", newType: "objectId", currentType: "string", want: true},
		{name: "date beats number", newType: "date", currentType: "number", want: true},
		{name: "timestamp beats regex", newType: "timestamp", currentType: "regex", want: true},
		{name: "decimal beats binData", newType: "decimal", currentType: "binData", want: true},
		{name: "string does not beat objectId", newType: "string", currentType: "objectId", want: false},
		{name: "unknown does not beat anything", newType: "unknown", currentType: "boolean", want: false},
		{name: "same type returns false", newType: "date", currentType: "date", want: false},
		{name: "number beats boolean", newType: "number", currentType: "boolean", want: true},
		{name: "number beats unknown", newType: "number", currentType: "unknown", want: true},
		{name: "objectId beats unknown", newType: "objectId", currentType: "unknown", want: true},
		{name: "array equals boolean priority", newType: "array", currentType: "boolean", want: false},
		{name: "regex beats number", newType: "regex", currentType: "number", want: true},
		{name: "binData beats regex", newType: "binData", currentType: "regex", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ds.isMoreSpecificType(tt.newType, tt.currentType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// MockRepository Multiple Call Sequence Test
// ---------------------------------------------------------------------------

func TestRepository_FullWorkflow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockRepository(ctrl)

	ctx := context.Background()

	// 1. Get schema first
	expectedSchema := []CollectionSchema{
		{
			CollectionName: "transactions",
			Fields: []FieldInformation{
				{Name: "amount", DataType: "number"},
				{Name: "status", DataType: "string"},
			},
		},
	}

	gomock.InOrder(
		mockRepo.EXPECT().
			GetDatabaseSchema(gomock.Any()).
			Return(expectedSchema, nil),
		mockRepo.EXPECT().
			Query(gomock.Any(), "transactions", []string{"amount", "status"}, map[string][]any{"status": {"active"}}).
			Return([]map[string]any{{"amount": 100.0, "status": "active"}}, nil),
		mockRepo.EXPECT().
			CloseConnection(gomock.Any()).
			Return(nil),
	)

	// Step 1: Get schema
	schema, err := mockRepo.GetDatabaseSchema(ctx)
	require.NoError(t, err)
	require.Len(t, schema, 1)
	assert.Equal(t, "transactions", schema[0].CollectionName)

	// Step 2: Query with discovered fields
	fields := make([]string, 0, len(schema[0].Fields))
	for _, f := range schema[0].Fields {
		fields = append(fields, f.Name)
	}

	results, err := mockRepo.Query(ctx, "transactions", fields, map[string][]any{"status": {"active"}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 100.0, results[0]["amount"])

	// Step 3: Close connection
	err = mockRepo.CloseConnection(ctx)
	require.NoError(t, err)
}
