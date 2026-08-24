// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// holderCreateBody builds the smallest well-formed POST /v1/holders body carrying
// the given document, so each case differs ONLY in the field under test.
func holderCreateBody(t *testing.T, doc string) []byte {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"type":     "NATURAL_PERSON",
		"name":     "Test Holder",
		"document": doc,
	})
	require.NoError(t, err)

	return body
}

// TestDecodeAndValidate_HolderDocument_AcceptsValidAndUnrecognisedShapes is the
// negative-control half: a valid CPF, a valid CNPJ and documents that are not
// shaped like either must all still pass the door. A rule that refuses these has
// broken legitimate holders, which is worse than the gap it closes.
func TestDecodeAndValidate_HolderDocument_AcceptsValidAndUnrecognisedShapes(t *testing.T) {
	t.Parallel()

	accepted := []struct {
		name     string
		document string
	}{
		{"valid CPF", "12345678909"},
		{"valid CPF, punctuated", "123.456.789-09"},
		{"valid CNPJ", "11222333000181"},
		{"valid CNPJ, punctuated", "11.222.333/0001-81"},
		{"passport number is not a CPF or CNPJ", "AB1234567"},
		{"nine-digit national ID is not a CPF or CNPJ", "123456789"},
		{"twelve digits is not a CPF or CNPJ", "123456789012"},
		{"alphanumeric registry ID is not a CPF or CNPJ", "12ABC34567DE89"},
	}

	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := new(mmodel.CreateHolderInput)
			_, err := DecodeAndValidate(holderCreateBody(t, tt.document), payload)
			require.NoErrorf(t, err, "document %q must be accepted", tt.document)
		})
	}
}

// TestDecodeAndValidate_HolderDocument_RefusesFailedCheckDigits is the enforcing
// half. Every assertion here is load-bearing:
//
//   - the call must fail at all (without the rule the malformed document is stored);
//   - it must fail as a *known-fields* validation error naming "document", not a
//     generic error and not a 5xx;
//   - the code must be ErrBadRequest, NOT ErrMissingFieldsInRequest — fieldsRequired
//     substring-matches the word "required" into the translated message and would
//     misreport a bad VALUE as a MISSING field;
//   - the message must name both accepted shapes, so the caller learns what is
//     wrong rather than only that something is.
func TestDecodeAndValidate_HolderDocument_RefusesFailedCheckDigits(t *testing.T) {
	t.Parallel()

	refused := []struct {
		name     string
		document string
	}{
		{"CPF with a wrong first check digit", "12345678919"},
		{"CPF with a wrong second check digit", "12345678900"},
		{"CPF that is all zeros", "00000000000"},
		{"CPF that is all ones", "11111111111"},
		{"CNPJ with wrong check digits", "11222333000180"},
		{"CNPJ that is all zeros", "00000000000000"},
		{"punctuated CPF with wrong check digits", "123.456.789-00"},
	}

	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload := new(mmodel.CreateHolderInput)

			_, err := DecodeAndValidate(holderCreateBody(t, tt.document), payload)
			require.Errorf(t, err, "document %q must be refused", tt.document)

			var known *pkg.ValidationKnownFieldsError
			require.ErrorAs(t, err, &known,
				"refusal must be a known-fields validation error (400), not a generic or server error")

			require.Contains(t, known.Fields, "document",
				"the refusal must name the offending field")
			require.Equal(t, cn.ErrBadRequest.Error(), known.Code,
				"a bad document VALUE must not be reported as a MISSING field")
			require.Contains(t, known.Fields["document"], "CPF")
			require.Contains(t, known.Fields["document"], "CNPJ")
		})
	}
}

// TestValidateStruct_HolderDocument_RuleDoesNotRunOnTheReadModel locks where the
// rule deliberately does NOT run. mmodel.Holder is what a stored holder is read
// back as; holders carrying a document that fails the new rule already exist in
// real environments, and a rule that also ran here would make those records
// unreadable — turning a data-quality problem into an outage.
func TestValidateStruct_HolderDocument_RuleDoesNotRunOnTheReadModel(t *testing.T) {
	t.Parallel()

	invalid := "11111111111"
	holderType := "NATURAL_PERSON"
	name := "Legacy Holder"

	stored := &mmodel.Holder{
		Type:     &holderType,
		Name:     &name,
		Document: &invalid,
	}

	require.NoError(t, ValidateStruct(stored),
		"a stored holder whose document predates the rule must still validate on read")
}

// TestDecodeAndValidate_HolderUpdate_DocumentIsNotAnUpdatableField pins the other
// half of "where it does not run": UpdateHolderInput carries no document field at
// all, so PATCH refuses it as unexpected rather than re-validating it. That is
// what makes creation the only door the rule has to hold — and it is also why an
// invalid document, once stored, cannot be corrected through the API.
func TestDecodeAndValidate_HolderUpdate_DocumentIsNotAnUpdatableField(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(map[string]any{"document": "12345678909"})
	require.NoError(t, err)

	payload := new(mmodel.UpdateHolderInput)

	_, err = DecodeAndValidate(body, payload)
	require.Error(t, err, "PATCH must not silently accept a document field")

	var unknown pkg.ValidationUnknownFieldsError
	require.True(t, errors.As(err, &unknown),
		"a document sent to PATCH must be refused as an unexpected field")
	require.Contains(t, unknown.Fields, "document")
	require.Equal(t, cn.ErrUnexpectedFieldsInTheRequest.Error(), unknown.Code)
}
