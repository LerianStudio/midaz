package mongo

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/tests/utils/stubs"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

func TestBuildDocumentToPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		updateDocument bson.M
		fieldsToRemove []string
		wantSet        bson.M
		wantUnset      bson.M
	}{
		{
			name:           "empty document and no fields to remove",
			updateDocument: bson.M{},
			fieldsToRemove: nil,
			wantSet:        nil,
			wantUnset:      nil,
		},
		{
			name:           "flat document with no removals",
			updateDocument: bson.M{"name": "John", "age": 30},
			fieldsToRemove: nil,
			wantSet:        bson.M{"name": "John", "age": 30},
			wantUnset:      nil,
		},
		{
			name: "nested document flattens to dot notation",
			updateDocument: bson.M{
				"address": bson.M{
					"city":  "NYC",
					"state": "NY",
				},
			},
			fieldsToRemove: nil,
			wantSet:        bson.M{"address.city": "NYC", "address.state": "NY"},
			wantUnset:      nil,
		},
		{
			name: "deeply nested document",
			updateDocument: bson.M{
				"level1": bson.M{
					"level2": bson.M{
						"level3": "deep",
					},
				},
			},
			fieldsToRemove: nil,
			wantSet:        bson.M{"level1.level2.level3": "deep"},
			wantUnset:      nil,
		},
		{
			name:           "metadata prefix preserved in unset",
			updateDocument: bson.M{},
			fieldsToRemove: []string{"metadata.customKey"},
			wantSet:        nil,
			wantUnset:      bson.M{"metadata.customKey": ""},
		},
		{
			name:           "non-metadata field converted to snake_case in unset",
			updateDocument: bson.M{},
			fieldsToRemove: []string{"bankingDetails"},
			wantSet:        nil,
			wantUnset:      bson.M{"banking_details": "bankingDetails"},
		},
		{
			name:           "nested non-metadata field preserves dots in snake_case",
			updateDocument: bson.M{},
			fieldsToRemove: []string{"bankingDetails.routingNumber"},
			wantSet:        nil,
			wantUnset:      bson.M{"banking_details.routing_number": "bankingDetails.routingNumber"},
		},
		{
			name: "combined set and unset operations",
			updateDocument: bson.M{
				"name": "Updated",
				"nested": bson.M{
					"keep": "this",
				},
			},
			fieldsToRemove: []string{"metadata.toRemove", "oldField"},
			wantSet:        bson.M{"name": "Updated", "nested.keep": "this"},
			wantUnset:      bson.M{"metadata.toRemove": "", "old_field": "oldField"},
		},
		{
			name: "fields to remove excludes matching keys from set",
			updateDocument: bson.M{
				"keep":   "value",
				"remove": "this",
			},
			fieldsToRemove: []string{"remove"},
			wantSet:        bson.M{"keep": "value"},
			wantUnset:      bson.M{"remove": "remove"},
		},
		{
			name: "nested field removal excludes parent and children from set",
			updateDocument: bson.M{
				"addresses": bson.M{
					"primary": bson.M{
						"city":  "NYC",
						"state": "NY",
					},
					"secondary": bson.M{
						"city": "LA",
					},
				},
			},
			fieldsToRemove: []string{"addresses.primary"},
			wantSet:        bson.M{"addresses.secondary.city": "LA"},
			wantUnset:      bson.M{"addresses.primary": "addresses.primary"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := BuildDocumentToPatch(tt.updateDocument, tt.fieldsToRemove)

			if tt.wantSet == nil {
				assert.NotContains(t, result, "$set", "should not contain $set")
			} else {
				assert.Equal(t, tt.wantSet, result["$set"], "$set mismatch")
			}

			if tt.wantUnset == nil {
				assert.NotContains(t, result, "$unset", "should not contain $unset")
			} else {
				assert.Equal(t, tt.wantUnset, result["$unset"], "$unset mismatch")
			}
		})
	}
}

func TestFlattenBSONM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  bson.M
		prefix string
		want   bson.M
	}{
		{
			name:   "empty map",
			input:  bson.M{},
			prefix: "",
			want:   bson.M{},
		},
		{
			name:   "flat map no prefix",
			input:  bson.M{"a": 1, "b": "two"},
			prefix: "",
			want:   bson.M{"a": 1, "b": "two"},
		},
		{
			name:   "flat map with prefix",
			input:  bson.M{"a": 1},
			prefix: "parent",
			want:   bson.M{"parent.a": 1},
		},
		{
			name: "nested map",
			input: bson.M{
				"outer": bson.M{
					"inner": "value",
				},
			},
			prefix: "",
			want:   bson.M{"outer.inner": "value"},
		},
		{
			name: "deeply nested map",
			input: bson.M{
				"a": bson.M{
					"b": bson.M{
						"c": bson.M{
							"d": "deep",
						},
					},
				},
			},
			prefix: "",
			want:   bson.M{"a.b.c.d": "deep"},
		},
		{
			name: "mixed nested and flat",
			input: bson.M{
				"flat": "value",
				"nested": bson.M{
					"child": "nested_value",
				},
			},
			prefix: "",
			want:   bson.M{"flat": "value", "nested.child": "nested_value"},
		},
		{
			name: "nested with prefix",
			input: bson.M{
				"child": bson.M{
					"grandchild": "value",
				},
			},
			prefix: "root",
			want:   bson.M{"root.child.grandchild": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := bson.M{}
			flattenBSONM(tt.input, tt.prefix, result)

			assert.Equal(t, tt.want, result)
		})
	}
}

func TestShouldUnset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		key            string
		fieldsToRemove []string
		want           bool
	}{
		{
			name:           "empty fieldsToRemove returns false",
			key:            "anything",
			fieldsToRemove: nil,
			want:           false,
		},
		{
			name:           "empty slice fieldsToRemove returns false",
			key:            "anything",
			fieldsToRemove: []string{},
			want:           false,
		},
		{
			name:           "exact match returns true",
			key:            "address",
			fieldsToRemove: []string{"address"},
			want:           true,
		},
		{
			name:           "prefix match returns true",
			key:            "addresses.primary.city",
			fieldsToRemove: []string{"addresses.primary"},
			want:           true,
		},
		{
			name:           "no match returns false",
			key:            "name",
			fieldsToRemove: []string{"address", "phone"},
			want:           false,
		},
		{
			name:           "partial key name is not a match",
			key:            "addresses",
			fieldsToRemove: []string{"address"},
			want:           false,
		},
		{
			name:           "key is prefix of field does not match",
			key:            "addr",
			fieldsToRemove: []string{"address"},
			want:           false,
		},
		{
			name:           "multiple fields one matches",
			key:            "phone",
			fieldsToRemove: []string{"address", "phone", "email"},
			want:           true,
		},
		{
			name:           "deeply nested prefix match",
			key:            "a.b.c.d.e",
			fieldsToRemove: []string{"a.b"},
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := shouldUnset(tt.key, tt.fieldsToRemove)

			assert.Equal(t, tt.want, result)
		})
	}
}

func TestExtractMongoPortAndParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		port               string
		parameters         string
		expectedPort       string
		expectedParameters string
		expectWarning      bool
		warningSubstring   string
	}{
		{
			name:               "legacy_embedded_parameters",
			port:               "5703/replicaSet=rs0&authSource=admin",
			parameters:         "",
			expectedPort:       "5703",
			expectedParameters: "replicaSet=rs0&authSource=admin",
			expectWarning:      true,
			warningSubstring:   "DEPRECATED",
		},
		{
			name:               "legacy_embedded_with_question_mark",
			port:               "5703?replicaSet=rs0",
			parameters:         "",
			expectedPort:       "5703",
			expectedParameters: "replicaSet=rs0",
			expectWarning:      true,
			warningSubstring:   "DEPRECATED",
		},
		{
			name:               "new_clean_port_with_parameters",
			port:               "5703",
			parameters:         "replicaSet=rs0&authSource=admin",
			expectedPort:       "5703",
			expectedParameters: "replicaSet=rs0&authSource=admin",
			expectWarning:      false,
			warningSubstring:   "",
		},
		{
			name:               "transition_both_set_parameters_wins",
			port:               "5703/embedded=old",
			parameters:         "explicit=new",
			expectedPort:       "5703",
			expectedParameters: "explicit=new",
			expectWarning:      true,
			warningSubstring:   "takes precedence",
		},
		{
			name:               "default_no_parameters",
			port:               "27017",
			parameters:         "",
			expectedPort:       "27017",
			expectedParameters: "",
			expectWarning:      false,
			warningSubstring:   "",
		},
		{
			name:               "empty_port_and_parameters",
			port:               "",
			parameters:         "",
			expectedPort:       "",
			expectedParameters: "",
			expectWarning:      false,
			warningSubstring:   "",
		},
		{
			name:               "port_with_whitespace",
			port:               " 5703 ",
			parameters:         "",
			expectedPort:       " 5703 ",
			expectedParameters: "",
			expectWarning:      false,
			warningSubstring:   "",
		},
		{
			name:               "non_numeric_port",
			port:               "abc",
			parameters:         "",
			expectedPort:       "abc",
			expectedParameters: "",
			expectWarning:      false,
			warningSubstring:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := &stubs.LoggerStub{}

			actualPort, actualParameters := ExtractMongoPortAndParameters(tt.port, tt.parameters, logger)

			assert.Equal(t, tt.expectedPort, actualPort, "port mismatch")
			assert.Equal(t, tt.expectedParameters, actualParameters, "parameters mismatch")

			if tt.expectWarning {
				assert.True(t, logger.WarningCount() > 0, "expected warning to be logged")
				assert.True(t, logger.HasWarning(tt.warningSubstring),
					"expected warning containing %q, got: %v", tt.warningSubstring, logger.Warnings)
			} else {
				assert.Equal(t, 0, logger.WarningCount(), "expected no warnings, got: %v", logger.Warnings)
			}
		})
	}
}

func TestExtractMongoPortAndParameters_NilLogger(t *testing.T) {
	t.Parallel()

	t.Run("nil_logger_with_embedded_parameters_does_not_panic", func(t *testing.T) {
		t.Parallel()

		actualPort, actualParameters := ExtractMongoPortAndParameters("5703/replicaSet=rs0", "", nil)

		assert.Equal(t, "5703", actualPort)
		assert.Equal(t, "replicaSet=rs0", actualParameters)
	})

	t.Run("nil_logger_with_both_embedded_and_explicit_does_not_panic", func(t *testing.T) {
		t.Parallel()

		actualPort, actualParameters := ExtractMongoPortAndParameters("5703/embedded=old", "explicit=new", nil)

		assert.Equal(t, "5703", actualPort)
		assert.Equal(t, "explicit=new", actualParameters)
	})
}

