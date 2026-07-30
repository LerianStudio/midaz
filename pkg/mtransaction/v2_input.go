// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// CreateTransactionV2Input is the request payload for the Transaction API v2. It
// mirrors the canonical Transaction shape explicitly rather than embedding
// mmodel/canonical types, so domain evolution never leaks onto the wire contract.
//
// Each side of the transaction is spelled EITHER as a scalar account alias
// (From/To) or as a leg array (Sources/Destinations) — never both, and the choice
// is per request, not per side. Description, Code, Asset, Amount, RouteID,
// OperationRouteID and Metadata are common to both spellings. Amount stays
// mandatory in the array form too: it is the transaction total that the legs'
// share and remaining expressions divide.
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

	// From is the source account alias of the scalar form (single debit leg).
	// Mutually exclusive with Sources. It carries no `required` tag because a
	// struct tag cannot express "exactly one of a pair"; the side obligation is a
	// Translate rule.
	From string `json:"from,omitempty"`

	// To is the destination account alias of the scalar form (single credit leg).
	// Mutually exclusive with Destinations, and untagged for the same reason as
	// From.
	To string `json:"to,omitempty"`

	// Sources are the debit legs of the array form. Mutually exclusive with From.
	//
	// The json tag deliberately omits `omitempty`: an explicit `"sources": []`
	// would otherwise disappear from the re-marshal the request decoder diffs
	// against the submitted body and be reported as an unknown field, masking the
	// side error. `required:"false"` restores on the published schema what
	// `omitempty` normally buys — without it the contract advertises the array form
	// as mandatory. `dive` is what makes the per-leg tags apply to each element.
	Sources []V2LegInput `json:"sources" validate:"dive" required:"false"`

	// Destinations are the credit legs of the array form. Mutually exclusive with
	// To. Same tag reasoning as Sources.
	Destinations []V2LegInput `json:"destinations" validate:"dive" required:"false"`

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

// V2LegInput is one leg of the array form. Exactly ONE value expression per leg:
// an explicit Amount, a Share of the transaction total, or Remaining. The leg
// exposes no balance key, chart of accounts, description or metadata, keeping the
// array form symmetric with the scalar one.
type V2LegInput struct {
	// Account is the leg's account alias.
	Account string `json:"account"`

	// Amount is the leg's explicit value, carried as a string to preserve JSON
	// precision. Same value semantics as the request-level amount.
	Amount string `json:"amount,omitempty"`

	// Share expresses the leg's value as a percentage of the transaction total
	// instead of an absolute amount.
	Share *V2ShareInput `json:"share,omitempty"`

	// Remaining assigns the leg whatever the other legs on the same side left
	// unallocated of the transaction total.
	Remaining bool `json:"remaining,omitempty"`

	// OperationRouteID is the leg's OPERATION route UUID, overriding the
	// request-level OperationRouteID for this leg. Validated as a UUID at decode
	// (same tag as the request-level field) so a malformed value is a clean 400,
	// not a deep funnel error.
	OperationRouteID *string `json:"operationRouteId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
}

// V2ShareInput expresses a leg's value as a percentage of the transaction total.
type V2ShareInput struct {
	// Percentage is the leg's share of the transaction total, in percent.
	Percentage int64 `json:"percentage"`

	// PercentageOfPercentage narrows Percentage to a fraction of itself, in
	// percent, mirroring the canonical share semantics.
	PercentageOfPercentage int64 `json:"percentageOfPercentage,omitempty"`
}

// legRemainingMarker is the value a canonical leg carries to mean "take whatever
// the other legs on this side left unallocated of the transaction total". The
// amount resolver keys off the marker being non-empty; the value mirrors the DSL
// keyword and the documented v1 JSON example so both spellings look the same
// downstream.
const legRemainingMarker = "remaining"

// Translate converts the flat v2 input into the canonical Transaction. The
// pending flag encodes the action intent (direct=false, hold=true) and is set by
// the endpoint, not the request body.
//
// Each side is spelled EITHER scalar (From/To) or as a leg array
// (Sources/Destinations), and the choice is per request: mixing the two
// spellings across sides is rejected. The scalar spelling produces one leg
// carrying the whole transaction total; the array spelling produces one leg per
// entry, each with exactly one value expression — an explicit amount parsed into
// a decimal, a share of the total, or the remaining marker.
//
// Route identifiers map at two independent levels: RouteID is the TRANSACTION
// route (Transaction.RouteID); OperationRouteID is the per-leg OPERATION route
// (FromTo.RouteID). A leg's own route wins; without one the leg inherits the
// request-level route. Nil route pointers stay nil so downstream ledger settings
// resolve defaults.
//
// Two validations deliberately stay OUT of here and belong to the create funnel,
// which owns the amount resolution both would have to duplicate:
//
//   - Balance: whether the legs sum to the transaction total. Checking it here
//     means reimplementing the resolver's share, percentage-of-percentage and
//     remaining arithmetic and drifting from it.
//   - Cross-side ambiguity in the array spelling, which leans on the funnel's own
//     ambiguity check. The scalar From == To check below stays because comparing a
//     single pair costs nothing. The array spelling therefore carries the weaker of
//     the two guarantees — deliberately, because matching what the v1 detailed body
//     guarantees is the criterion.
func (in CreateTransactionV2Input) Translate(pending bool) (Transaction, error) {
	arrayForm, err := in.resolveSpelling()
	if err != nil {
		return Transaction{}, err
	}

	value, err := decimal.NewFromString(in.Amount)
	if err != nil || value.LessThanOrEqual(decimal.Zero) {
		return Transaction{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
	}

	if !arrayForm && in.From == in.To {
		return Transaction{}, pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, constant.EntityTransaction)
	}

	from, err := in.buildLegs(in.Sources, in.From, value, true, "sources")
	if err != nil {
		return Transaction{}, err
	}

	to, err := in.buildLegs(in.Destinations, in.To, value, false, "destinations")
	if err != nil {
		return Transaction{}, err
	}

	send := Send{
		Asset:      in.Asset,
		Value:      value,
		Source:     Source{From: from},
		Distribute: Distribute{To: to},
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

// resolveSpelling reports whether the request uses the leg-array spelling. It
// rejects a side that spells itself neither way, a side that spells itself both
// ways, and a request that mixes the spellings across its two sides — so on
// success both sides agree and one flag describes the whole request.
//
// An explicit empty leg array counts as no legs, so it reads as an unspelled
// side rather than as a choice of the array form.
func (in CreateTransactionV2Input) resolveSpelling() (bool, error) {
	sourceIsArray := len(in.Sources) > 0
	destinationIsArray := len(in.Destinations) > 0

	if in.From == "" && !sourceIsArray {
		return false, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, "from or sources")
	}

	if in.To == "" && !destinationIsArray {
		return false, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, "to or destinations")
	}

	bothOnOneSide := (in.From != "" && sourceIsArray) || (in.To != "" && destinationIsArray)
	if bothOnOneSide || sourceIsArray != destinationIsArray {
		return false, pkg.ValidateBusinessError(constant.ErrMutuallyExclusiveTransactionFields, constant.EntityTransaction)
	}

	return sourceIsArray, nil
}

// buildLegs expands one side into canonical legs. With no legs the side is in the
// scalar spelling and yields a single leg carrying the whole transaction total;
// otherwise each entry yields its own leg. fieldName names the side's leg array in
// the per-leg error messages.
func (in CreateTransactionV2Input) buildLegs(legs []V2LegInput, alias string, total decimal.Decimal, isFrom bool, fieldName string) ([]FromTo, error) {
	if len(legs) == 0 {
		return []FromTo{{
			AccountAlias: alias,
			Amount:       &Amount{Asset: in.Asset, Value: total},
			RouteID:      cloneStringPtr(in.OperationRouteID),
			IsFrom:       isFrom,
		}}, nil
	}

	out := make([]FromTo, 0, len(legs))

	for _, leg := range legs {
		built, err := in.buildLeg(leg, isFrom, fieldName)
		if err != nil {
			return nil, err
		}

		out = append(out, built)
	}

	return out, nil
}

// buildLeg maps one array entry onto a canonical leg. The entry must name an
// account and fill exactly one of the three value expressions; the shared
// invalid-transaction-type error is the same one the v1 detailed body raises for
// this rule, so both surfaces reject it identically.
func (in CreateTransactionV2Input) buildLeg(leg V2LegInput, isFrom bool, fieldName string) (FromTo, error) {
	if leg.Account == "" {
		return FromTo{}, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, fieldName+".account")
	}

	expressions := 0

	if leg.Amount != "" {
		expressions++
	}

	if leg.Share != nil {
		expressions++
	}

	if leg.Remaining {
		expressions++
	}

	if expressions != 1 {
		return FromTo{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionType, constant.EntityTransaction, fieldName)
	}

	route := leg.OperationRouteID
	if route == nil {
		route = in.OperationRouteID
	}

	built := FromTo{
		AccountAlias: leg.Account,
		RouteID:      cloneStringPtr(route),
		IsFrom:       isFrom,
	}

	switch {
	case leg.Amount != "":
		value, err := decimal.NewFromString(leg.Amount)
		if err != nil || value.LessThanOrEqual(decimal.Zero) {
			return FromTo{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		}

		built.Amount = &Amount{Asset: in.Asset, Value: value}
	case leg.Share != nil:
		built.Share = &Share{
			Percentage:             leg.Share.Percentage,
			PercentageOfPercentage: leg.Share.PercentageOfPercentage,
		}
	default:
		built.Remaining = legRemainingMarker
	}

	return built, nil
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
