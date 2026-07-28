// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import "fmt"

// CreateTransactionV2Input is the flat, single-leg request payload for the
// Transaction API v2. It mirrors the canonical Transaction shape explicitly
// rather than embedding mmodel/canonical types, so domain evolution never leaks
// onto the wire contract. The richer discriminated (flat-OR-arrays) format is
// reserved for a later phase; v2.1.1 keeps every leg field a simple scalar.
type CreateTransactionV2Input struct {
	// Human-readable description of the transaction.
	Description string `json:"description,omitempty"`

	// Transaction code for reference.
	Code string `json:"code,omitempty"`

	// Asset code shared by both legs. Same value semantics as v1.
	Asset string `json:"asset" validate:"required"`

	// Amount carried as a string to preserve JSON precision. Same value
	// semantics as v1; Translate parses it into a decimal.
	Amount string `json:"amount" validate:"required"`

	// From is the source account alias (single debit leg).
	From string `json:"from" validate:"required"`

	// To is the destination account alias (single credit leg).
	To string `json:"to" validate:"required"`

	// RouteID is the optional TRANSACTION route UUID.
	RouteID *string `json:"routeId,omitempty"`

	// OperationRouteID is the optional per-leg OPERATION route UUID.
	OperationRouteID *string `json:"operationRouteId,omitempty"`

	// Metadata holds flat custom key-value attributes. Values must be flat
	// (string, number, boolean) — no nested objects.
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
}

// Translate converts the flat v2 input into the canonical Transaction. The
// pending flag encodes the action intent (direct=false, hold=true) and is set
// by the endpoint, not the request body.
//
// The mapping is implemented in Task 1.2.2. Until then Translate returns a
// not-implemented error so no caller can mistake a zero-value Transaction for a
// successfully translated one.
func (in CreateTransactionV2Input) Translate(pending bool) (Transaction, error) {
	return Transaction{}, fmt.Errorf(
		"mtransaction: CreateTransactionV2Input.Translate is not implemented (pending=%t): mapping lands in Task 1.2.2",
		pending,
	)
}
