// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
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
