// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeValidateBody_CreatePackageTransactionRoute pins what a client may
// scope a fee package to.
//
// The field holds the transaction route a package applies on, and the fee
// selector compares it to the route identifier the payment carries, which is a
// route UUID. A free-form name therefore matches no payment that can ever be
// posted: the package is stored, listed, looks configured, and silently charges
// nobody. Refusing it at create is the only point where a client still has the
// context to fix it.
//
// Carrying no route stays valid in both spellings this endpoint accepts, absent
// and empty, because that is how a client says the package applies on every
// route. An explicit null is refused, and always was: the decoder re-marshals
// the body and flags every key that disappears, which is every omitempty pointer
// a client sent as null. That is the endpoint answering for description and
// waivedAccounts too, not a rule about routes, and it is pinned here so the next
// reader does not take it for one.
func TestDecodeValidateBody_CreatePackageTransactionRoute(t *testing.T) {
	t.Parallel()

	routeID := uuid.NewString()

	body := func(routeField string) string {
		return `{
			"feeGroupLabel": "Standard",
			"minimumAmount": "100.00",
			"maximumAmount": "1000.00",
			"enable": true,` + routeField + `
			"fees": {
				"test": {
					"feeLabel": "TestFee",
					"referenceAmount": "originalAmount",
					"priority": 1,
					"isDeductibleFrom": false,
					"creditAccount": "@fee_account",
					"calculationModel": {
						"applicationRule": "flatFee",
						"calculations": [{"type": "flat", "value": "10"}]
					}
				}
			}
		}`
	}

	tests := []struct {
		name       string
		routeField string
		wantErr    bool
		// wantUnknownField marks the refusal that comes from the decoder
		// re-marshalling the body rather than from the route rule, so it is not
		// asserted to name the field the way a validation failure does.
		wantUnknownField bool
	}{
		{
			name:       "a free-form route name is refused",
			routeField: `"transactionRoute": "debitoted",`,
			wantErr:    true,
		},
		{
			name:       "a route uuid is accepted",
			routeField: `"transactionRoute": "` + routeID + `",`,
		},
		{
			name:       "an omitted route is accepted",
			routeField: ``,
		},
		{
			// Pre-existing and endpoint-wide: see the header. A client saying
			// no route sends no key, or an empty string.
			name:             "a null route is refused as an unexpected field",
			routeField:       `"transactionRoute": null,`,
			wantErr:          true,
			wantUnknownField: true,
		},
		{
			name:       "an empty route is accepted",
			routeField: `"transactionRoute": "",`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := new(model.CreatePackageInput)

			_, err := DecodeValidateBody([]byte(body(tc.routeField)), payload)

			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)

			if tc.wantUnknownField {
				return
			}

			// The fee body validator returns the envelope by pointer.
			var fieldsErr *pkg.ValidationKnownFieldsError

			require.True(t, errors.As(err, &fieldsErr),
				"the refusal must be the known-fields envelope, so a client is told which field to fix")
			assert.Contains(t, fieldsErr.Fields, "transactionRoute",
				"the refusal must name transactionRoute rather than leaving the client to guess")
		})
	}
}
