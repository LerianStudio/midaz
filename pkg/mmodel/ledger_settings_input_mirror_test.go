// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonFieldsByName indexes a struct type's fields by the name in their json tag.
func jsonFieldsByName(t reflect.Type) map[string]reflect.StructField {
	fields := make(map[string]reflect.StructField, t.NumField())

	for i := range t.NumField() {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]

		if name != "" && name != "-" {
			fields[name] = field
		}
	}

	return fields
}

// schemaLeafKind maps a settingsSchema type name to the element kind its mirror pointer
// field must carry.
var schemaLeafKind = map[string]reflect.Kind{
	"bool":   reflect.Bool,
	"string": reflect.String,
	"number": reflect.Int,
}

// nonZeroLeaf returns a pointer to a non-zero value of elem, for use as a sent leaf.
func nonZeroLeaf(t *testing.T, elem reflect.Type) reflect.Value {
	t.Helper()

	ptr := reflect.New(elem)

	switch elem.Kind() {
	case reflect.Bool:
		ptr.Elem().SetBool(true)
	case reflect.String:
		ptr.Elem().SetString("x")
	case reflect.Int:
		ptr.Elem().SetInt(1)
	default:
		t.Fatalf("unsupported leaf kind %s; extend nonZeroLeaf alongside schemaLeafKind", elem.Kind())
	}

	return ptr
}

// TestSettingsSchema_HasMatchingInputField walks settingsSchema — the value space PATCH and
// POST share — and asserts every parent.leaf also reaches the POST request tree: the group
// has a mirror field on LedgerSettingsInput, the leaf has a pointer field of the schema's
// type on that group, and ToSparseMap emits it under the schema's own key spelling.
//
// Without this walk, nothing catches a schema leaf the input tree never grew: that gap stays
// green in CI and surfaces in production as a field PATCH accepts and POST rejects with an
// unknown-field 400. TestLedgerSettingsInput_HasMatchingSchemaLeaf closes the same drift in
// the opposite direction; the ToSparseMap tables in ledger_settings_input_test.go are
// hand-written rows, so they only cover leaves someone remembered to add.
func TestSettingsSchema_HasMatchingInputField(t *testing.T) {
	t.Parallel()

	type leafCase struct {
		parent     string
		leaf       string
		schemaType string
	}

	cases := make([]leafCase, 0, len(settingsSchema))

	for parent, leaves := range settingsSchema {
		for leaf, schemaType := range leaves {
			cases = append(cases, leafCase{parent: parent, leaf: leaf, schemaType: schemaType})
		}
	}

	sort.Slice(cases, func(i, j int) bool {
		if cases[i].parent != cases[j].parent {
			return cases[i].parent < cases[j].parent
		}

		return cases[i].leaf < cases[j].leaf
	})

	require.NotEmpty(t, cases, "settingsSchema must not be empty")

	inputType := reflect.TypeOf(LedgerSettingsInput{})
	groupFields := jsonFieldsByName(inputType)

	require.Len(t, groupFields, len(settingsSchema),
		"LedgerSettingsInput must mirror exactly the settingsSchema groups, no more and no fewer")

	for _, tt := range cases {
		tt := tt

		t.Run(tt.parent+"."+tt.leaf, func(t *testing.T) {
			t.Parallel()

			groupField, ok := groupFields[tt.parent]
			require.True(t, ok, "settingsSchema group %q has no field on LedgerSettingsInput", tt.parent)
			require.Equal(t, reflect.Ptr, groupField.Type.Kind(),
				"LedgerSettingsInput.%s must be a pointer so an absent group stays distinguishable", groupField.Name)

			groupType := groupField.Type.Elem()

			leafField, ok := jsonFieldsByName(groupType)[tt.leaf]
			require.True(t, ok, "settingsSchema leaf %s.%s has no field on %s", tt.parent, tt.leaf, groupType.Name())
			require.Equal(t, reflect.Ptr, leafField.Type.Kind(),
				"%s.%s must be a pointer so an explicit zero stays distinguishable", groupType.Name(), leafField.Name)

			wantKind, ok := schemaLeafKind[tt.schemaType]
			require.True(t, ok, "settingsSchema type %q has no mapping in schemaLeafKind", tt.schemaType)
			assert.Equal(t, wantKind, leafField.Type.Elem().Kind(),
				"%s.%s must carry the schema type %q", groupType.Name(), leafField.Name, tt.schemaType)

			// Declaring the field is not enough: ToSparseMap has to emit it, under the
			// schema's key spelling, or POST still rejects it as an unknown field.
			input := reflect.New(inputType)
			group := reflect.New(groupType)
			leaf := nonZeroLeaf(t, leafField.Type.Elem())

			group.Elem().FieldByName(leafField.Name).Set(leaf)
			input.Elem().FieldByName(groupField.Name).Set(group)

			sparse, ok := input.Interface().(*LedgerSettingsInput)
			require.True(t, ok)

			got := sparse.ToSparseMap()

			emitted, ok := got[tt.parent].(map[string]any)
			require.True(t, ok, "ToSparseMap must emit group %q, got %#v", tt.parent, got[tt.parent])
			assert.Equal(t, map[string]any{tt.leaf: leaf.Elem().Interface()}, emitted,
				"ToSparseMap must emit exactly the sent leaf under its schema key")
		})
	}
}

// TestLedgerSettingsInput_HasMatchingSchemaLeaf is the inverse walk: every json-tagged leaf on
// every LedgerSettingsInput group must have a settingsSchema entry. The group half of this
// direction is already closed by the require.Len in TestSettingsSchema_HasMatchingInputField;
// the leaf half is not covered anywhere
// else, because the ToSparseMap tables enumerate remembered leaves rather than reflecting.
//
// The drift it kills: a leaf added to an *Input group and emitted from ToSparseMap but missing
// from settingsSchema is advertised by the generated OpenAPI, accepted by DecodeAndValidate as
// a known struct field, emitted into the sparse map — and then rejected by ValidateSettings as
// an unknown path. A documented field that always 4xx's.
func TestLedgerSettingsInput_HasMatchingSchemaLeaf(t *testing.T) {
	t.Parallel()

	groupFields := jsonFieldsByName(reflect.TypeOf(LedgerSettingsInput{}))
	require.NotEmpty(t, groupFields, "LedgerSettingsInput must declare at least one group")

	groupNames := make([]string, 0, len(groupFields))
	for name := range groupFields {
		groupNames = append(groupNames, name)
	}

	sort.Strings(groupNames)

	for _, parent := range groupNames {
		parent := parent

		t.Run(parent, func(t *testing.T) {
			t.Parallel()

			groupField := groupFields[parent]
			require.Equal(t, reflect.Ptr, groupField.Type.Kind(),
				"LedgerSettingsInput.%s must be a pointer so an absent group stays distinguishable", groupField.Name)

			groupType := groupField.Type.Elem()

			leaves, ok := settingsSchema[parent]
			require.True(t, ok, "LedgerSettingsInput group %q has no settingsSchema entry", parent)

			leafFields := jsonFieldsByName(groupType)
			require.NotEmpty(t, leafFields, "%s must declare at least one leaf", groupType.Name())

			leafNames := make([]string, 0, len(leafFields))
			for name := range leafFields {
				leafNames = append(leafNames, name)
			}

			sort.Strings(leafNames)

			for _, leaf := range leafNames {
				_, known := leaves[leaf]
				assert.True(t, known,
					"%s.%s emits settings key %q, which settingsSchema does not allow; "+
						"ValidateSettings would reject it as an unknown path", groupType.Name(),
					leafFields[leaf].Name, parent+"."+leaf)
			}

			assert.Len(t, leafFields, len(leaves),
				"%s must mirror exactly the settingsSchema leaves of %q, no more and no fewer",
				groupType.Name(), parent)
		})
	}
}
