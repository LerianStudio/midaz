// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

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

	// RouteID is the optional TRANSACTION route UUID. Validated as a UUID at
	// decode (same tag as the v1 input) so a malformed value is a clean 400, not
	// a deep funnel error.
	RouteID *string `json:"routeId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// OperationRouteID is the optional per-leg OPERATION route UUID. Validated as
	// a UUID at decode for the same reason as RouteID.
	OperationRouteID *string `json:"operationRouteId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Metadata holds flat custom key-value attributes. Values must be flat
	// (string, number, boolean) — no nested objects.
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
}

// Translate converts the flat v2 input into the canonical single-leg
// Transaction. The pending flag encodes the action intent (direct=false,
// hold=true) and is set by the endpoint, not the request body.
//
// Route identifiers map at two independent levels: RouteID is the TRANSACTION
// route (Transaction.RouteID); OperationRouteID is the per-leg OPERATION route
// (FromTo.RouteID) and is copied onto both the source and distribution legs.
// Nil route pointers stay nil so downstream ledger settings resolve defaults.
//
// A malformed or non-positive amount and a source equal to the destination are
// rejected as business errors (422), mirroring the v1 transaction-value and
// ambiguity validations, so the HTTP layer maps them to 4xx.
func (in CreateTransactionV2Input) Translate(pending bool) (Transaction, error) {
	value, err := decimal.NewFromString(in.Amount)
	if err != nil || value.LessThanOrEqual(decimal.Zero) {
		return Transaction{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
	}

	if in.From == in.To {
		return Transaction{}, pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, constant.EntityTransaction)
	}

	send := Send{
		Asset: in.Asset,
		Value: value,
		Source: Source{
			From: []FromTo{{
				AccountAlias: in.From,
				Amount:       &Amount{Asset: in.Asset, Value: value},
				RouteID:      cloneStringPtr(in.OperationRouteID),
				IsFrom:       true,
			}},
		},
		Distribute: Distribute{
			To: []FromTo{{
				AccountAlias: in.To,
				Amount:       &Amount{Asset: in.Asset, Value: value},
				RouteID:      cloneStringPtr(in.OperationRouteID),
			}},
		},
	}

	return Transaction{
		Description: in.Description,
		Code:        in.Code,
		Pending:     pending,
		Metadata:    in.Metadata,
		RouteID:     cloneStringPtr(in.RouteID),
		Send:        send,
	}, nil
}

// cloneStringPtr returns an independent copy of p, or nil when p is nil, so
// callers never alias the input's route pointers onto the produced legs.
func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}

	v := *p

	return &v
}
