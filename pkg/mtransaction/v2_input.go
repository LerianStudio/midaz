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
// Each side of the transaction is spelled EITHER as a scalar account (From/To) or as a
// leg array (Sources/Destinations), and each side chooses independently: one payer
// paired with many payees is a valid request. Either spelling names, per leg, the
// account alias and the organization and ledger that account belongs to.
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

	// From is the source account of the scalar form (single debit leg). Mutually
	// exclusive with Sources. It carries no `validate:"required"` because a struct tag
	// cannot express "exactly one of a pair"; the side obligation is a Translate rule.
	//
	// The json tag carries no `omitempty` so an explicit `"from": {}` stays a KNOWN
	// field and is answered with the side error instead of an unknown-field error, and
	// so the re-marshal the decoder diffs against the body always emits the key — which
	// is what makes a submitted `"from": null` an unexpected field.
	// `required:"false"` is what then keeps the published schema from declaring the
	// scalar form mandatory.
	//
	// It is a value and not a pointer for that same diff: a nil pointer would re-marshal
	// as `null` and make a submitted `null` match instead of being rejected.
	From V2AccountInput `json:"from" required:"false"`

	// To is the destination account of the scalar form (single credit leg). Mutually
	// exclusive with Destinations, and typed and tagged for the same reasons as From.
	To V2AccountInput `json:"to" required:"false"`

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

// V2AccountInput names one account of the scalar form together with the scope that account
// belongs to. The scope travels in the body, per side, so a request states the organization
// and ledger of every account it touches instead of inheriting one from the URL.
//
// None of the three fields carries `validate:"required"`, and that is not an oversight: the
// validator walks a value struct whether or not the client spelled the side, so a `required`
// tag here would reject every body that spells its sides with the leg arrays. The obligation
// is a Translate rule for all three. The published schema still lists them as required,
// which is the accurate contract: a body that spells the side must fill them.
type V2AccountInput struct {
	// Alias is the account alias. Same value semantics as v1.
	Alias string `json:"alias"`

	// OrganizationID is the organization the account belongs to. Validated as a UUID at
	// decode (same tag as the route fields) so a malformed value is a clean 400, not a deep
	// funnel error. `omitempty` is what lets an unspelled side through to the side rule.
	OrganizationID string `json:"organizationId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// LedgerID is the ledger the account belongs to, tagged for the same reasons as
	// OrganizationID.
	LedgerID string `json:"ledgerId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
}

// V2LegInput is one leg of the array form. Exactly ONE value expression per leg:
// an explicit Amount or a Share of the transaction total. The leg exposes no
// balance key, chart of accounts, description or metadata, keeping the array form
// symmetric with the scalar one.
type V2LegInput struct {
	// Alias is the leg's account alias. The obligation is enforced BOTH by this tag and by
	// an imperative check in Translate; see buildLeg for why the two are complementary.
	Alias string `json:"alias" validate:"required"`

	// OrganizationID is the organization the leg's account belongs to. An array entry is
	// always spelled, so unlike the scalar form's fields this one can be tagged `required`
	// outright; the `uuid` tag makes a malformed value a clean 400 rather than a deep funnel
	// error. Translate enforces the presence obligation as well, for the same reason it does
	// for Alias.
	OrganizationID string `json:"organizationId" validate:"required,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// LedgerID is the ledger the leg's account belongs to, tagged for the same reasons as
	// OrganizationID.
	LedgerID string `json:"ledgerId" validate:"required,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

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

// V2ShareInput expresses a leg's value as a percentage of the transaction total. The resolver
// computes total x (percentage/100) x (percentageOfPercentage/100).
//
// Both factors carry the same upper bound of 100, so whether a body is accepted does not depend on
// which of the two the client puts the larger number in. That symmetry is chosen with its cost
// named: a body whose two factors multiply back into range — 150 narrowed to 50, resolving to 75%
// of the total — is refused even though it would balance, and not every such body can be respelled
// with both factors in range. The refusal is a 400 naming the field.
//
// Whether a side's legs sum to the transaction total stays a whole-body property that no per-field
// bound can decide; the funnel's total check owns that rule.
type V2ShareInput struct {
	// Percentage is the leg's share of the transaction total, in percent, bounded to 1..100. It
	// must be positive: a zero share moves nothing while the transaction still commits, and a
	// negative one inverts the leg's accounting direction. buildLeg enforces the lower bound
	// imperatively as well, covering a caller that builds the input in Go and skips the decoder.
	//
	// The `minimum` and `maximum` tags publish the bounds in the contract, so a client reads them
	// instead of discovering them by rejection. They do not enforce anything: the create ops
	// decode the body imperatively, so the `validate` tags are the only evaluated ones.
	Percentage int64 `json:"percentage" validate:"required,gt=0,lte=100" example:"60" minimum:"1" maximum:"100"`

	// PercentageOfPercentage narrows Percentage, in percent: 25 against a Percentage of 60
	// yields 15% of the transaction total.
	//
	// ZERO MEANS NO NARROWING, not a zero share. On an int64 it is indistinguishable from
	// the field being omitted, and omitted has to mean "take the whole Percentage", so a leg
	// that spells 0 gets the full Percentage. Rejecting 0 instead would reject every body
	// that leaves the field out — which is why its lower bound is 0 where Percentage's is 1.
	//
	// Bounds published in the contract for the same reason as Percentage's.
	PercentageOfPercentage int64 `json:"percentageOfPercentage,omitempty" validate:"omitempty,gte=0,lte=100" example:"50" minimum:"0" maximum:"100"`
}

// V2Scope is the organization and ledger a v2 request is scoped by: the pair every leg of the
// request named. Translate resolves it from the body and hands it back, so a caller scopes the
// transaction by what the request says rather than by where it was posted.
//
// The identifiers are carried as the client spelled them. Their UUID shape is a contract
// obligation the decode boundary answers, so this type asserts no format of its own.
type V2Scope struct {
	// OrganizationID is the organization every leg named.
	OrganizationID string

	// LedgerID is the ledger every leg named.
	LedgerID string
}

// namesSameAs reports whether other names the same organization and ledger.
//
// The comparison ignores letter case because a UUID's text spelling does: two legs that spell one
// ledger in different cases name one ledger, and refusing that body would reject a request that
// never left a single ledger.
func (s V2Scope) namesSameAs(other V2Scope) bool {
	return strings.EqualFold(s.OrganizationID, other.OrganizationID) &&
		strings.EqualFold(s.LedgerID, other.LedgerID)
}

// v2ScopeRef is one leg's scope paired with the field reference a rejection names it by, so the
// scope rules can report the offending entry without knowing which spelling produced it.
type v2ScopeRef struct {
	scope V2Scope
	ref   string
}

// scope reads the scope off a scalar side.
func (a V2AccountInput) scope() V2Scope {
	return V2Scope{OrganizationID: a.OrganizationID, LedgerID: a.LedgerID}
}

// scope reads the scope off an array entry.
func (l V2LegInput) scope() V2Scope {
	return V2Scope{OrganizationID: l.OrganizationID, LedgerID: l.LedgerID}
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

// Translate converts the flat v2 input into the canonical Transaction and the V2Scope the
// request is scoped by. The pending flag encodes the action intent (direct=false, hold=true)
// and is set by the endpoint, not the request body.
//
// The scope is returned alongside the transaction because it is a property of the BODY: every
// leg names the organization and ledger its account belongs to, and all of them must name the
// same pair. A request whose legs disagree is refused, so the caller receives one scope or an
// error and never has to pick between two.
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
func (in CreateTransactionV2Input) Translate(pending bool) (Transaction, V2Scope, error) {
	if err := in.validateSideSpelling(); err != nil {
		return Transaction{}, V2Scope{}, err
	}

	value, err := decimal.NewFromString(in.Amount)
	if err != nil || value.LessThanOrEqual(decimal.Zero) {
		return Transaction{}, V2Scope{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
	}

	if in.From.Alias != "" && in.From.Alias == in.To.Alias {
		return Transaction{}, V2Scope{}, pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, constant.EntityTransaction)
	}

	from, err := in.buildLegs(in.Sources, in.From.Alias, value, true, "sources")
	if err != nil {
		return Transaction{}, V2Scope{}, err
	}

	to, err := in.buildLegs(in.Destinations, in.To.Alias, value, false, "destinations")
	if err != nil {
		return Transaction{}, V2Scope{}, err
	}

	scope, err := in.resolveScope()
	if err != nil {
		return Transaction{}, V2Scope{}, err
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
	}, scope, nil
}

// resolveScope folds every leg's scope into the single pair the request is scoped by, whichever
// spelling each side chose. Every leg must name a complete scope, and all of them must name the
// SAME one.
//
// The agreement rule is what keeps a request inside one ledger: a body naming two would have no
// single ledger to post against, and honouring each half against its own ledger leaves value
// moving in one direction only on each of them. This is also the single place that decides the
// rule, so widening it later is a change here and nowhere else.
//
// The pair returned is the FIRST leg's spelling, and the legs are walked source side first.
func (in CreateTransactionV2Input) resolveScope() (V2Scope, error) {
	var resolved V2Scope

	for i, ref := range in.scopeRefs() {
		if err := ref.requireComplete(); err != nil {
			return V2Scope{}, err
		}

		if i == 0 {
			resolved = ref.scope

			continue
		}

		if !resolved.namesSameAs(ref.scope) {
			return V2Scope{}, pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)
		}
	}

	return resolved, nil
}

// scopeRefs lists one entry per leg of the request, source side first, each carrying the field
// reference its rejections are named by.
func (in CreateTransactionV2Input) scopeRefs() []v2ScopeRef {
	refs := make([]v2ScopeRef, 0, len(in.Sources)+len(in.Destinations)+2)
	refs = appendSideScopeRefs(refs, in.From, in.Sources, "from", "sources")

	return appendSideScopeRefs(refs, in.To, in.Destinations, "to", "destinations")
}

// appendSideScopeRefs appends one entry per leg of a single side. With no legs the side is in the
// scalar spelling and contributes its one account; otherwise each array entry contributes its own,
// referenced by index. scalarRef and legsField name the side in the two spellings.
func appendSideScopeRefs(refs []v2ScopeRef, account V2AccountInput, legs []V2LegInput, scalarRef, legsField string) []v2ScopeRef {
	if len(legs) == 0 {
		return append(refs, v2ScopeRef{scope: account.scope(), ref: scalarRef})
	}

	for i, leg := range legs {
		refs = append(refs, v2ScopeRef{scope: leg.scope(), ref: legReference(legsField, i)})
	}

	return refs
}

// requireComplete rejects a leg that leaves either half of its scope empty, naming the field the
// caller has to fill.
//
// The obligation is enforced here as well as by the leg arrays' `required` tags, for the same
// reason the alias obligation is: Translate is exported from a shared package, so a caller that
// assembles the input in Go meets no tag at all, and the scalar spelling cannot carry a `required`
// tag because the validator walks it whether or not the client spelled that side.
func (r v2ScopeRef) requireComplete() error {
	switch {
	case r.scope.OrganizationID == "":
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, r.ref+".organizationId")
	case r.scope.LedgerID == "":
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, r.ref+".ledgerId")
	default:
		return nil
	}
}

// validateSideSpelling checks that each side spells itself exactly one way. The
// two sides are independent: a scalar payer paired with an array of payees is
// valid, which is the shape a one-to-many payout takes.
//
// An explicit empty leg array counts as no legs, so it reads as an unspelled side
// rather than as a choice of the array form.
func (in CreateTransactionV2Input) validateSideSpelling() error {
	if err := validateSideSpelledOnce(in.From.Alias, len(in.Sources) > 0, "from or sources"); err != nil {
		return err
	}

	return validateSideSpelledOnce(in.To.Alias, len(in.Destinations) > 0, "to or destinations")
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
// The alias obligation and the share's positive-percentage obligation are each enforced here AND
// as a struct tag. They are complementary, not redundant: the tag is the guard every HTTP caller
// meets, because it fires at the decode boundary before Translate runs, while these checks are the
// only ones covering a caller that builds the input in Go and skips the decoder — Translate is
// exported from a shared package. An empty alias reaching the funnel names no account at all, and
// a non-positive percentage resolves to no operation row (zero) or an inverted movement
// (negative) while the transaction still commits.
func (in CreateTransactionV2Input) buildLeg(leg V2LegInput, isFrom bool, legRef string) (FromTo, error) {
	if leg.Alias == "" {
		return FromTo{}, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, legRef+".alias")
	}

	if err := validateV2Alias(leg.Alias); err != nil {
		return FromTo{}, err
	}

	route := leg.OperationRouteID
	if route == nil {
		route = in.OperationRouteID
	}

	built := FromTo{
		AccountAlias: leg.Alias,
		RouteID:      cloneStringPtr(route),
		IsFrom:       isFrom,
	}

	// Each arm demands its own expression AND the absence of the other, so "exactly one" is
	// decided in one place. That leaves the default arm reachable: it answers both-filled and
	// neither-filled alike, and a THIRD expression filled on its own lands there too instead of
	// producing a leg with no value at all, which reads as a valid entry and moves nothing.
	switch {
	case leg.Amount != "" && leg.Share == nil:
		value, err := decimal.NewFromString(leg.Amount)
		if err != nil || value.LessThanOrEqual(decimal.Zero) {
			return FromTo{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		}

		built.Amount = &Amount{Asset: in.Asset, Value: value}
	case leg.Share != nil && leg.Amount == "":
		if leg.Share.Percentage <= 0 {
			return FromTo{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		}

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
	return pkg.ValidateTransactionTypeError(constant.EntityTransaction,
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
