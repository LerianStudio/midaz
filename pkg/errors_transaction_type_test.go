// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// v1DetailedTransactionTypeMessage is the rendered invalid-transaction-type message the
// detailed transaction body has shipped, captured verbatim. The option set is a parameter so
// each surface can name the expressions IT accepts, and the surfaces do not accept the same
// set — this locks the released rendering byte-for-byte so parameterising it cannot move a
// single character of the message a v1 client already reads.
const v1DetailedTransactionTypeMessage = "Only one transaction type ('amount', 'share', or 'remaining') " +
	"must be specified in the 'send.source.from' field for each entry. Please review your input and try again."

// v2LegTransactionTypeMessage is the same message rendered for a v2 leg. It must NOT name
// `remaining`: the v2 surface publishes no such expression, so a caller told to send one is
// answered with a different 400 and has no way out of the loop.
const v2LegTransactionTypeMessage = "Only one transaction type ('amount' or 'share') " +
	"must be specified in the 'sources[1]' field for each entry. Please review your input and try again."

// renderedTransactionTypeMessage drives the sentinel through the real registry and hands back
// the rendered ValidationError message, which is the only part a client reads.
func renderedTransactionTypeMessage(t *testing.T, options, fieldRef string) string {
	t.Helper()

	err := ValidateBusinessError(constant.ErrInvalidTransactionType, constant.EntityTransaction, options, fieldRef)

	var vErr ValidationError
	require.ErrorAs(t, err, &vErr, "the invalid-transaction-type sentinel must render as a ValidationError (400)")
	assert.Equal(t, constant.ErrInvalidTransactionType.Error(), vErr.Code)
	assert.Equal(t, "Invalid Transaction Type", vErr.Title)

	return vErr.Message
}

// TestValidateBusinessError_InvalidTransactionTypePerSurface proves the option set is what
// varies between surfaces and nothing else: the detailed body renders byte-for-byte what it
// shipped, and the v2 leg renders its own two expressions.
func TestValidateBusinessError_InvalidTransactionTypePerSurface(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		options  string
		fieldRef string
		want     string
	}{
		{
			name:     "detailed body keeps its released rendering",
			options:  constant.TransactionTypeOptionsDetailed,
			fieldRef: "send.source.from",
			want:     v1DetailedTransactionTypeMessage,
		},
		{
			name:     "v2 leg names only the expressions it publishes",
			options:  constant.TransactionTypeOptionsLeg,
			fieldRef: "sources[1]",
			want:     v2LegTransactionTypeMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, renderedTransactionTypeMessage(t, tt.options, tt.fieldRef))
		})
	}
}

// TestTransactionTypeOptions_LegSetExcludesRemaining pins the difference that motivates the
// parameter. `remaining` resolves during validation but contributes no operation row, so the
// v2 surface does not publish it; offering it in a rejection message would steer the caller
// toward an expression that is answered with an unexpected-field 400.
func TestTransactionTypeOptions_LegSetExcludesRemaining(t *testing.T) {
	t.Parallel()

	assert.Contains(t, constant.TransactionTypeOptionsDetailed, "remaining",
		"the detailed body accepts the remaining expression and must keep naming it")
	assert.NotContains(t, constant.TransactionTypeOptionsLeg, "remaining",
		"a v2 leg has no remaining expression, so its option set must not name one")

	for _, expression := range []string{"amount", "share"} {
		assert.Contains(t, constant.TransactionTypeOptionsLeg, expression,
			"the leg option set must name the %s expression", expression)
	}
}

// TestValidateBusinessError_InvalidTransactionTypeRendersEveryArgument guards the failure mode
// a two-placeholder message introduces: a caller that passes only one argument renders a
// message carrying fmt's MISSING marker instead of prose. Both placeholders must be filled by
// every call site.
func TestValidateBusinessError_InvalidTransactionTypeRendersEveryArgument(t *testing.T) {
	t.Parallel()

	for _, options := range []string{constant.TransactionTypeOptionsDetailed, constant.TransactionTypeOptionsLeg} {
		message := renderedTransactionTypeMessage(t, options, "sources[0]")

		assert.NotContains(t, message, "%!", "the rendered message must not carry a formatting marker")
		assert.NotContains(t, message, "MISSING", "the rendered message must not carry a formatting marker")
	}
}

// TestValidateBusinessError_InvalidTransactionTypeIsNotWrapped keeps the sentinel resolvable
// from the rendered error, which is what the handlers' errors.As cascade keys on.
func TestValidateBusinessError_InvalidTransactionTypeIsNotWrapped(t *testing.T) {
	t.Parallel()

	err := ValidateBusinessError(constant.ErrInvalidTransactionType, constant.EntityTransaction,
		constant.TransactionTypeOptionsLeg, "sources[0]")

	var vErr ValidationError
	require.True(t, errors.As(err, &vErr))
	assert.Equal(t, constant.EntityTransaction, vErr.EntityType)
}
