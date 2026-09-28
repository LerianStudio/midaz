// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
)

type validationDetailsRequest struct {
	Name  string                  `json:"name" validate:"required"`
	Items []validationDetailsItem `json:"items" validate:"min=1,dive"`
}

type validationDetailsItem struct {
	Alias string `json:"alias" validate:"required"`
}

func TestDecodeAndValidateWithDetails_CollectsAndSortsKnownAndUnknownFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"items": [
			{"zeta": "unexpected"},
			{"alias": "", "alpha": "unexpected"}
		]
	}`)

	var input validationDetailsRequest
	_, details, err := DecodeAndValidateWithDetails(raw, &input)
	require.Error(t, err)

	var unknownErr pkg.ValidationUnknownFieldsError
	require.True(t, errors.As(err, &unknownErr), "unknown fields keep singular precedence")
	assert.Equal(t, []pkg.FieldError{
		{Location: "items[0].alias", Message: "items[0].alias is a required field"},
		{Location: "items[0].zeta", Message: "unexpected field"},
		{Location: "items[1].alias", Message: "items[1].alias is a required field"},
		{Location: "items[1].alpha", Message: "unexpected field"},
		{Location: "name", Message: "name is a required field"},
	}, details)
}

func TestDecodeAndValidateWithDetails_PreservesDecodeAndValidatePrimary(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"items":[{"alias":123}]}`)

	var legacyInput validationDetailsRequest
	_, legacyErr := DecodeAndValidate(raw, &legacyInput)
	require.Error(t, legacyErr)

	var detailedInput validationDetailsRequest
	_, details, detailedErr := DecodeAndValidateWithDetails(raw, &detailedInput)
	require.Error(t, detailedErr)

	assert.Equal(t, legacyErr, detailedErr)
	assert.Equal(t, []pkg.FieldError{{
		Location: "items[0].alias",
		Message:  "invalid value: expected type 'string', but got 'number'",
	}}, details)
}

func TestDecodeAndValidateWithDetails_RecoversNestedArrayIndexFromRawBody(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"items":[{"alias":"valid"},{"alias":123}]}`)

	var input validationDetailsRequest
	_, details, err := DecodeAndValidateWithDetails(raw, &input)
	require.Error(t, err)
	assert.Equal(t, []pkg.FieldError{{
		Location: "items[1].alias",
		Message:  "invalid value: expected type 'string', but got 'number'",
	}}, details)
}

func TestUnmarshallingFieldDetails_RecoversOmittedSliceIndex(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"items":[{"alias":"valid"},{"alias":123}]}`)
	details := unmarshallingFieldDetails(raw, &json.UnmarshalTypeError{
		Value: "number",
		Type:  reflect.TypeFor[string](),
		Field: "items.alias",
	})

	assert.Equal(t, []pkg.FieldError{{
		Location: "items[1].alias",
		Message:  "invalid value: expected type 'string', but got 'number'",
	}}, details)
}

type nullDetailsContact struct {
	PrimaryEmail *string `json:"primaryEmail,omitempty"`
}

type nullDetailsRequest struct {
	Name    string              `json:"name" validate:"required"`
	Contact *nullDetailsContact `json:"contact,omitempty"`
}

func TestDecodeAndValidateWithDetails_UnknownFieldsWithNullKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		fields   []string
		details  []pkg.FieldError
		excluded []string
	}{
		{
			name:   "several unknown non-null keys are listed together",
			body:   `{"name": "x", "foo": 1, "contact": {"bar": "y"}}`,
			fields: []string{"foo", "contact"},
			details: []pkg.FieldError{
				{Location: "contact.bar", Message: "unexpected field"},
				{Location: "foo", Message: "unexpected field"},
			},
		},
		{
			name:   "unknown non-null key wins over unknown null key",
			body:   `{"name": "x", "foo": 1, "createdAt": null}`,
			fields: []string{"foo"},
			details: []pkg.FieldError{
				{Location: "foo", Message: "unexpected field"},
			},
			excluded: []string{"createdAt"},
		},
		{
			name:   "nested form of a managed field stays rejected",
			body:   `{"name": "x", "search": {"document": null}}`,
			fields: []string{"search"},
			details: []pkg.FieldError{
				{Location: "search", Message: "unexpected field"},
			},
		},
		{
			name:   "unknown null key keeps precedence over missing required field",
			body:   `{"contact": {"bogus": null}}`,
			fields: []string{"contact.bogus"},
			details: []pkg.FieldError{
				{Location: "contact.bogus", Message: "unexpected field"},
				{Location: "name", Message: "name is a required field"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var input nullDetailsRequest
			originalMap, details, err := DecodeAndValidateWithDetails([]byte(tc.body), &input)
			require.Error(t, err)
			assert.Nil(t, originalMap)

			var unknownErr pkg.ValidationUnknownFieldsError
			require.ErrorAs(t, err, &unknownErr)
			assert.Equal(t, "0053", unknownErr.Code)

			for _, field := range tc.fields {
				assert.Contains(t, unknownErr.Fields, field)
			}

			for _, field := range tc.excluded {
				assert.NotContains(t, unknownErr.Fields, field)
			}

			assert.Equal(t, tc.details, details)
		})
	}
}
