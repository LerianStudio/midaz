// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// CreateTransactionV2Input is the request payload for the Transaction API v2. It
// mirrors the canonical Transaction shape explicitly rather than embedding
// mmodel/canonical types, so domain evolution never leaks onto the wire contract.
//
// Each side of the transaction is spelled EITHER as a scalar account alias
// (From/To) or as a leg array (Sources/Destinations), and each side chooses
// independently: one payer paired with many payees is a valid request.
// Description, Code, Asset, Amount, RouteID, OperationRouteID and Metadata are
// common to both spellings. Amount stays mandatory in the array form too: it is
// the transaction total that the legs' share expressions divide.
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
	// Mutually exclusive with Sources. It carries no `validate:"required"` because a
	// struct tag cannot express "exactly one of a pair"; the side obligation is a
	// Translate rule.
	//
	// The json tag carries no `omitempty` so an explicit `"from": ""` stays a KNOWN
	// field and is answered with the side error instead of an unknown-field error.
	// `required:"false"` is what then keeps the published schema from declaring the
	// scalar form mandatory.
	From string `json:"from" required:"false"`

	// To is the destination account alias of the scalar form (single credit leg).
	// Mutually exclusive with Destinations, and tagged for the same reasons as From.
	To string `json:"to" required:"false"`

	// Sources are the debit legs of the array form. Mutually exclusive with From.
	//
	// The json tag carries no `omitempty` for the same reason as From: an explicit
	// `"sources": []` stays a KNOWN field and is answered with the side error.
	// `required:"false"` keeps the published schema from declaring the array form
	// mandatory. `max=500` bounds the per-side leg count, which nothing else does: the
	// request-body byte ceiling alone admits tens of thousands of legs, and each one
	// carries its own downstream cost. `dive` is what makes the per-leg tags apply to
	// each element.
	Sources []V2LegInput `json:"sources" validate:"max=500,dive" required:"false"`

	// Destinations are the credit legs of the array form. Mutually exclusive with
	// To. Same tag reasoning as Sources.
	Destinations []V2LegInput `json:"destinations" validate:"max=500,dive" required:"false"`

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
// an explicit Amount or a Share of the transaction total. The leg exposes no
// balance key, chart of accounts, description or metadata, keeping the array form
// symmetric with the scalar one.
type V2LegInput struct {
	// Account is the leg's account alias. The obligation is enforced BOTH by this tag and by
	// an imperative check in Translate; see buildLeg for why the two are complementary.
	Account string `json:"account" validate:"required"`

	// Amount is the leg's explicit value, carried as a string to preserve JSON
	// precision. Same value semantics as the request-level amount.
	Amount string `json:"amount,omitempty"`

	// Share expresses the leg's value as a percentage of the transaction total
	// instead of an absolute amount.
	Share *V2ShareInput `json:"share,omitempty"`

	// OperationRouteID is the leg's OPERATION route UUID, overriding the
	// request-level OperationRouteID for this leg. Validated as a UUID at decode
	// (same tag as the request-level field) so a malformed value is a clean 400,
	// not a deep funnel error.
	OperationRouteID *string `json:"operationRouteId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
}

// V2ShareInput expresses a leg's value as a percentage of the transaction total.
//
// PercentageOfPercentage carries an upper bound; Percentage does not. Whether a side's legs sum
// to the transaction total is a property of the whole body, which no single field's bound can
// decide, so the funnel's total check owns that rule and answers a side that cannot balance as a
// business rejection.
type V2ShareInput struct {
	// Percentage is the leg's share of the transaction total, in percent. It must be
	// positive: a zero share moves nothing while the transaction still commits, and a
	// negative one inverts the leg's accounting direction.
	//
	// It carries NO upper bound. PercentageOfPercentage scales it back down — the resolver
	// computes total x (percentage/100) x (percentageOfPercentage/100) — so 150 narrowed to 50
	// resolves to 75% of the total and balances against a sibling resolving to the remaining
	// 25%. A cap here would answer that balanced body 400 at decode.
	//
	// The `minimum` tag publishes the lower bound in the contract, so a client reads it instead
	// of discovering it by rejection. It does not enforce anything: the create ops decode the
	// body imperatively, so the `validate` tags are the only evaluated ones.
	Percentage int64 `json:"percentage" validate:"required,gt=0" example:"60" minimum:"1"`

	// PercentageOfPercentage narrows Percentage, in percent: 25 against a Percentage of 60
	// yields 15% of the transaction total.
	//
	// ZERO MEANS NO NARROWING, not a zero share. On an int64 it is indistinguishable from
	// the field being omitted, and omitted has to mean "take the whole Percentage", so a leg
	// that spells 0 gets the full Percentage. Rejecting 0 instead would reject every body
	// that leaves the field out.
	//
	// The upper bound is 100 because a factor above 100 widens rather than narrows: a leg
	// reading as "half, of which 200%" would move more than the Percentage it is attached to.
	// That is a violation of what this field means, independent of what the side sums to, which
	// is why it is a per-field bound. Published in the contract for the same reason as
	// Percentage's lower bound.
	PercentageOfPercentage int64 `json:"percentageOfPercentage,omitempty" validate:"omitempty,gte=0,lte=100" example:"50" minimum:"0" maximum:"100"`
}

// validateV2Alias rejects a v2 account alias carrying AliasSeparator. Every alias the v2 surface
// accepts routes through here — the two scalar fields as much as the two leg arrays — so no
// spelling of either side can reach the funnel with an alias that can be forged onto another
// entry's map key.
//
// An alias is rewritten into a composite separator-joined form before downstream code keys its
// per-entry maps on it, and isConcatedAlias leaves an alias that already looks composite spelled
// exactly as the client sent it. A client-supplied composite alias therefore reaches those maps
// unmutated, where it can collide with another entry's key or match none of them — either way an
// entry is lost, and a transaction that loses one side's entry moves value in one direction only.
//
// The rejected character is AliasSeparator because that is what the composite form is built and
// parsed with; naming the constant is what keeps this guard and that format from drifting apart.
//
// The narrow guard is deliberate: the registered alias charset would close this too, but it also
// excludes `/` and would therefore reject `@external/<ASSET>`, the alias every ledger's external
// account carries and the only way to spell funding or withdrawal on a surface with no
// inflow/outflow action.
func validateV2Alias(alias string) error {
	if strings.ContainsRune(alias, AliasSeparator) {
		return pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, constant.EntityTransaction)
	}

	return nil
}

// Translate converts the flat v2 input into the canonical Transaction. The
// pending flag encodes the action intent (direct=false, hold=true) and is set by
// the endpoint, not the request body.
//
// Each side is spelled EITHER scalar (From/To) or as a leg array
// (Sources/Destinations), and each side chooses independently. The scalar spelling
// produces one leg carrying the whole transaction total; the array spelling
// produces one leg per entry, each with exactly one value expression — an explicit
// amount parsed into a decimal, or a share of the total.
//
// Route identifiers map at two independent levels: RouteID is the TRANSACTION
// route (Transaction.RouteID); OperationRouteID is the per-leg OPERATION route
// (FromTo.RouteID). A leg's own route wins; without one the leg inherits the
// request-level route. Nil route pointers stay nil so downstream ledger settings
// resolve defaults.
//
// Whether the legs sum to the transaction total is NOT checked here: that comparison needs the
// resolved per-leg values, which Translate does not compute — it only carries each leg's value
// expression forward. The scalar From == To check below stays because comparing a single pair
// costs nothing.
func (in CreateTransactionV2Input) Translate(pending bool) (Transaction, error) {
	if err := in.validateSideSpelling(); err != nil {
		return Transaction{}, err
	}

	value, err := decimal.NewFromString(in.Amount)
	if err != nil || value.LessThanOrEqual(decimal.Zero) {
		return Transaction{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
	}

	if in.From != "" && in.From == in.To {
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

// validateSideSpelling checks that each side spells itself exactly one way. The
// two sides are independent: a scalar payer paired with an array of payees is
// valid, which is the shape a one-to-many payout takes.
//
// An explicit empty leg array counts as no legs, so it reads as an unspelled side
// rather than as a choice of the array form.
func (in CreateTransactionV2Input) validateSideSpelling() error {
	if err := validateSideSpelledOnce(in.From, len(in.Sources) > 0, "from or sources"); err != nil {
		return err
	}

	return validateSideSpelledOnce(in.To, len(in.Destinations) > 0, "to or destinations")
}

// validateSideSpelledOnce enforces "exactly one of (scalar alias, leg array)" for a
// single side: neither spelling is a missing field, both spellings is a
// mutual-exclusivity violation. fieldNames names the pair in the missing-field
// message.
func validateSideSpelledOnce(alias string, hasLegs bool, fieldNames string) error {
	switch {
	case alias == "" && !hasLegs:
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, fieldNames)
	case alias != "" && hasLegs:
		return pkg.ValidateBusinessError(constant.ErrMutuallyExclusiveTransactionFields, constant.EntityTransaction)
	default:
		return nil
	}
}

// buildLegs expands one side into canonical legs. With no legs the side is in the
// scalar spelling and yields a single leg carrying the whole transaction total;
// otherwise each entry yields its own leg. fieldName names the side's leg array in
// the per-leg error messages.
//
// The alias guard runs on BOTH spellings. The scalar alias arrives straight off the request
// with no per-leg tag behind it, so this is the only place it is checked at all.
func (in CreateTransactionV2Input) buildLegs(legs []V2LegInput, alias string, total decimal.Decimal, isFrom bool, fieldName string) ([]FromTo, error) {
	if len(legs) == 0 {
		if err := validateV2Alias(alias); err != nil {
			return nil, err
		}

		return []FromTo{{
			AccountAlias: alias,
			Amount:       &Amount{Asset: in.Asset, Value: total},
			RouteID:      cloneStringPtr(in.OperationRouteID),
			IsFrom:       isFrom,
		}}, nil
	}

	out := make([]FromTo, 0, len(legs))

	for i, leg := range legs {
		built, err := in.buildLeg(leg, isFrom, legReference(fieldName, i))
		if err != nil {
			return nil, err
		}

		out = append(out, built)
	}

	return out, nil
}

// legReference spells the indexed reference to one entry of a side — `sources[0]` — so a caller
// at the 500-leg cap can locate the entry a rejection is about. The shape matches the validator's
// own field namespace, so both classes of rejection read alike.
//
// Not every decoder rejection carries the index: only the tags whose registered translation reads
// the full field namespace (`required` among them) name it. The rest render the bare leaf field
// name, with no side and no index.
func legReference(fieldName string, i int) string {
	return fieldName + "[" + strconv.Itoa(i) + "]"
}

// buildLeg maps one array entry onto a canonical leg. The entry must name an alias, that alias
// must be free of AliasSeparator, and exactly one of the two value expressions must be filled.
// legRef is the indexed reference to the entry, which the rejections carry so a caller can locate
// it.
//
// The account obligation is enforced here AND as a `required` struct tag. They are complementary,
// not redundant: the tag is the guard every HTTP caller meets, because it fires at the decode
// boundary before Translate runs, while this check is the only one covering a caller that builds
// the input in Go and skips the decoder — Translate is exported from a shared package, and an
// empty alias reaching the funnel names no account at all. Both name the entry by index.
func (in CreateTransactionV2Input) buildLeg(leg V2LegInput, isFrom bool, legRef string) (FromTo, error) {
	if leg.Account == "" {
		return FromTo{}, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, legRef+".account")
	}

	if err := validateV2Alias(leg.Account); err != nil {
		return FromTo{}, err
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

	// Each arm demands its own expression AND the absence of the other, so "exactly one" is
	// decided in one place. That leaves the default arm reachable: it answers both-filled and
	// neither-filled alike, and a THIRD expression added to the leg lands there too instead of
	// producing a leg with no value at all, which reads as a valid entry and moves nothing.
	switch {
	case leg.Amount != "" && leg.Share == nil:
		value, err := decimal.NewFromString(leg.Amount)
		if err != nil || value.LessThanOrEqual(decimal.Zero) {
			return FromTo{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		}

		built.Amount = &Amount{Asset: in.Asset, Value: value}
	case leg.Share != nil && leg.Amount == "":
		built.Share = &Share{
			Percentage:             leg.Share.Percentage,
			PercentageOfPercentage: leg.Share.PercentageOfPercentage,
		}
	default:
		return FromTo{}, invalidLegExpression(legRef)
	}

	return built, nil
}

// invalidLegExpression rejects an entry that does not fill exactly one value expression. The
// message names the two expressions a v2 leg accepts; the sentinel is shared with the detailed
// transaction body, which accepts a third, so the option set has to be passed rather than
// assumed.
func invalidLegExpression(legRef string) error {
	return pkg.InvalidTransactionTypeError(constant.EntityTransaction,
		constant.TransactionTypeOptionsLeg, legRef)
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
