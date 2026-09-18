// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithFieldErrors_PreservesPrimaryError(t *testing.T) {
	t.Parallel()

	cause := errors.New("primary cause")
	primary := ValidationError{
		Code:    "0513",
		Title:   "Invalid Transaction Batch Cardinality",
		Message: "invalid batch",
		Err:     cause,
	}

	wrapped := WithFieldErrors(primary, []FieldError{{
		Location: "body.transactions[0].type",
		Message:  "type is required",
	}})

	require.Error(t, wrapped)
	assert.Equal(t, primary.Error(), wrapped.Error())
	assert.ErrorIs(t, wrapped, cause)
	assert.True(t, IsBusinessError(wrapped))

	var classified ValidationError
	require.ErrorAs(t, wrapped, &classified)
	assert.Equal(t, primary, classified)
}

func TestWithFieldErrors_PreservesOrderAndCopiesDetails(t *testing.T) {
	t.Parallel()

	input := []FieldError{
		{Location: "body.transactions[1].description", Message: "description is required"},
		{Location: "body.transactions[0].credits[2].alias", Message: "alias is required"},
		{Location: "body.transactions[0].debits[0].amount", Message: "amount must be positive"},
	}

	wrapped := WithFieldErrors(errors.New("invalid request"), input)

	var carrier *FieldErrorCarrier
	require.ErrorAs(t, wrapped, &carrier)

	input[0].Location = "mutated input"
	firstRead := carrier.FieldErrors()
	assert.Equal(t, []FieldError{
		{Location: "body.transactions[1].description", Message: "description is required"},
		{Location: "body.transactions[0].credits[2].alias", Message: "alias is required"},
		{Location: "body.transactions[0].debits[0].amount", Message: "amount must be positive"},
	}, firstRead)

	firstRead[0].Location = "mutated output"
	assert.Equal(t, "body.transactions[1].description", carrier.FieldErrors()[0].Location)
}

func TestWithFieldErrors_EnforcesDetailCeiling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		count           int
		wantLast        FieldError
		wantRealEntries int
	}{
		{
			name:            "exact ceiling keeps every real diagnostic",
			count:           MaxFieldErrors,
			wantLast:        numberedFieldError(MaxFieldErrors - 1),
			wantRealEntries: MaxFieldErrors,
		},
		{
			name:  "crossing ceiling appends explicit truncation marker",
			count: MaxFieldErrors + 1,
			wantLast: FieldError{
				Location: FieldErrorTruncationLocation,
				Message:  FieldErrorTruncationMessage,
			},
			wantRealEntries: MaxFieldErrors - 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			input := make([]FieldError, testCase.count)
			for index := range input {
				input[index] = numberedFieldError(index)
			}

			wrapped := WithFieldErrors(errors.New("invalid request"), input)

			var carrier *FieldErrorCarrier
			require.ErrorAs(t, wrapped, &carrier)
			got := carrier.FieldErrors()

			require.Len(t, got, MaxFieldErrors)
			assert.Equal(t, input[:testCase.wantRealEntries], got[:testCase.wantRealEntries])
			assert.Equal(t, testCase.wantLast, got[MaxFieldErrors-1])
		})
	}
}

func TestWithFieldErrors_NoDetailsOrPrimary(t *testing.T) {
	t.Parallel()

	primary := errors.New("invalid request")
	assert.Same(t, primary, WithFieldErrors(primary, nil))
	assert.NoError(t, WithFieldErrors(nil, []FieldError{{Location: "body.name", Message: "required"}}))
}

func numberedFieldError(index int) FieldError {
	return FieldError{
		Location: fmt.Sprintf("body.transactions[%d].description", index),
		Message:  fmt.Sprintf("diagnostic %d", index),
	}
}
