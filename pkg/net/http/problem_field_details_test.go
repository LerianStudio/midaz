// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestProblemDetail_ProjectsOrderedFieldErrorsAndPrimary(t *testing.T) {
	t.Parallel()

	primary := pkg.ValidationKnownFieldsError{
		EntityType: "Transaction",
		Code:       constant.ErrMissingFieldsInRequest.Error(),
		Title:      "Invalid Transaction Batch",
		Message:    "the first item is invalid",
		Fields: pkg.FieldValidations{
			"legacy-map-entry": "the ordered carrier must take precedence",
		},
	}
	wrapped := pkg.WithFieldErrors(primary, []pkg.FieldError{
		{Location: "body.transactions[0].type", Message: "type is required"},
		{Location: "body.transactions[0].debits[0].alias", Message: "alias is required"},
		{Location: "body.transactions[2].description", Message: "description is required"},
	})

	body, ok := ProblemDetail(wrapped)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, body.Status)
	assert.Equal(t, primary.Code, body.Code)
	assert.Equal(t, primary.Title, body.Title)
	assert.Equal(t, primary.Message, body.ErrorModel.Detail)
	assert.Equal(t, primary.EntityType, body.EntityType)
	require.Len(t, body.Errors, 3)

	assert.Equal(t, "body.transactions[0].type", body.Errors[0].Location)
	assert.Equal(t, "type is required", body.Errors[0].Message)
	assert.Nil(t, body.Errors[0].Value)
	assert.Equal(t, "body.transactions[0].debits[0].alias", body.Errors[1].Location)
	assert.Equal(t, "alias is required", body.Errors[1].Message)
	assert.Nil(t, body.Errors[1].Value)
	assert.Equal(t, "body.transactions[2].description", body.Errors[2].Location)
	assert.Equal(t, "description is required", body.Errors[2].Message)
	assert.Nil(t, body.Errors[2].Value)
}

func TestProblemDetail_ProjectsFieldErrorTruncationMarker(t *testing.T) {
	t.Parallel()

	fields := make([]pkg.FieldError, pkg.MaxFieldErrors+1)
	for index := range fields {
		fields[index] = pkg.FieldError{
			Location: fmt.Sprintf("body.transactions[%d].description", index),
			Message:  fmt.Sprintf("diagnostic %d", index),
		}
	}

	body, ok := ProblemDetail(pkg.WithFieldErrors(pkg.ValidationError{
		Code:    "0513",
		Title:   "Invalid Transaction Batch",
		Message: "the first item is invalid",
	}, fields))
	require.True(t, ok)
	require.Len(t, body.Errors, pkg.MaxFieldErrors)

	last := body.Errors[pkg.MaxFieldErrors-1]
	assert.Equal(t, pkg.FieldErrorTruncationLocation, last.Location)
	assert.Equal(t, pkg.FieldErrorTruncationMessage, last.Message)
	assert.Nil(t, last.Value)
}

func TestHumaProblem_PreservesOrderedFieldsAroundPointerValidationError(t *testing.T) {
	t.Parallel()

	primary := &pkg.ValidationKnownFieldsError{
		EntityType: "Transaction",
		Code:       constant.ErrMissingFieldsInRequest.Error(),
		Title:      "Missing Fields in Request",
		Message:    "the first item is missing required fields",
		Fields:     pkg.FieldValidations{"asset": "asset is required"},
	}
	wrapper := pkg.WithFieldErrors(primary, []pkg.FieldError{
		{Location: "body.transactions[0].asset", Message: "asset is required"},
		{Location: "body.transactions[1].amount", Message: "amount is required"},
	})

	rendered := HumaProblem(wrapper)
	detail, ok := rendered.(*Detail)
	require.True(t, ok)
	assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), detail.Code)
	require.Len(t, detail.Errors, 2)
	assert.Equal(t, "body.transactions[0].asset", detail.Errors[0].Location)
	assert.Equal(t, "body.transactions[1].amount", detail.Errors[1].Location)
}
