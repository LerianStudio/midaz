// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// componentSchema returns the named component schema from the assembled contract,
// or nil if the document declares no such component.
func componentSchema(api huma.API, name string) *huma.Schema {
	return api.OpenAPI().Components.Schemas.Map()[name]
}

// hasProperty reports whether s declares the named JSON property.
func hasProperty(s *huma.Schema, prop string) bool {
	if s == nil {
		return false
	}

	_, ok := s.Properties[prop]

	return ok
}

// TestInternalFieldsHiddenFromContract is the security gate for the typed-request-body
// feature: Huma's schema generator does NOT honor the Swaggo `swaggerignore:"true"`
// tag the domain types carry, so publishing the transaction input graph would leak the
// internal / derived fields on mtransaction.Amount — including the security-control
// opt-outs routeValidationEnabled and overdraftAmount — plus the read-model skip opt-out
// on TransactionInput. The publish-time schema transform must strip exactly those
// swaggerignore-tagged properties from the shared component schemas, while every
// legitimately-published field (including the documented create-input skip) survives.
func TestInternalFieldsHiddenFromContract(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	amount := componentSchema(api, "Amount")
	require.NotNil(t, amount, "the Amount request component must be published by the typed request bodies")

	// The five swaggerignore-tagged internal fields on mtransaction.Amount must NOT
	// appear on the published Amount component.
	for _, internal := range []string{"operation", "transactionType", "direction", "routeValidationEnabled", "overdraftAmount"} {
		assert.Falsef(t, hasProperty(amount, internal),
			"Amount schema must not expose the internal swaggerignore field %q", internal)
	}

	// The two contract fields on Amount must remain.
	for _, legit := range []string{"asset", "value"} {
		assert.Truef(t, hasProperty(amount, legit),
			"Amount schema must retain the published contract field %q", legit)
	}

	// TransactionInput (mtransaction.Transaction, embedded in the fee-estimate request)
	// carries a swaggerignore skip opt-out; it must not leak onto the contract.
	txInput := componentSchema(api, "TransactionInput")
	require.NotNil(t, txInput, "TransactionInput must be published")
	assert.False(t, hasProperty(txInput, "skip"),
		"TransactionInput must not expose the internal swaggerignore skip opt-out")
	// A representative legit field survives on TransactionInput.
	assert.True(t, hasProperty(txInput, "send"),
		"TransactionInput must retain the published send field")

	// Precision guard: the create-input skip is a legitimately-published field (no
	// swaggerignore tag, documented in the historical contract), so the transform must
	// leave it and its send field in place.
	createTx := componentSchema(api, "CreateTransactionInput")
	require.NotNil(t, createTx, "CreateTransactionInput must be published")
	assert.True(t, hasProperty(createTx, "skip"),
		"CreateTransactionInput must retain its documented skip field")
	assert.True(t, hasProperty(createTx, "send"),
		"CreateTransactionInput must retain its send field")
}
