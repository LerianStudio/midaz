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
// Each side of the transaction is a leg array (Debits/Credits): one debit paired with
// many credits, or the reverse, is a valid request. Each leg names the account alias
// and the organization and ledger that account belongs to. Description, Code, Asset,
// Amount, RouteID, OperationRouteID and Metadata sit alongside the two leg arrays.
// Amount is the transaction total that the legs' share expressions divide.
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

	// Debits are the debit legs of the transaction. Required and non-empty: `min=1`
	// rejects both an omitted key (which decodes to a nil slice) and an explicit
	// `"debits": []`, naming the field either way.
	//
	// The json tag carries no `omitempty`: an explicit `"debits": []` must stay a KNOWN
	// field, answered by the `min=1` rejection, rather than vanish from the re-marshal
	// the decoder diffs against the submitted body and come back as an unknown field.
	// Carrying no `omitempty` also leaves the field without a Huma `required` override,
	// so the published schema marks it mandatory by Huma's own default for a field with
	// no `omitempty`.
	//
	// `max=500` bounds the per-side leg count, which nothing else does: the request-body
	// byte ceiling alone admits tens of thousands of legs, and each one carries its own
	// downstream cost. `dive` is what makes the per-leg tags apply to each element.
	//
	// `minItems`/`maxItems` publish the bounds the `validate` tags enforce, so a client reads
	// them instead of discovering them by rejection. `nullable:"false"` keeps `null` out of
	// the published type: a nil slice is refused, and a schema admitting null would promise a
	// generated client that the server accepts a body it answers 400 to.
	Debits []V2LegInput `json:"debits" validate:"min=1,max=500,dive" minItems:"1" maxItems:"500" nullable:"false"`

	// Credits are the credit legs of the transaction, tagged for the same reasons as
	// Debits.
	Credits []V2LegInput `json:"credits" validate:"min=1,max=500,dive" minItems:"1" maxItems:"500" nullable:"false"`

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

	// OperationalTypeCode is the account-exception operational type code applied to this
	// transaction. Integrator-defined identifier (code, not prose): no whitespace, up to
	// 100 characters, mirroring the account-exception key bound.
	OperationalTypeCode string `json:"operationalTypeCode,omitempty" validate:"omitempty,nowhitespaces,max=100" example:"PIX_IN" maxLength:"100"`
}

// V2LegInput is one leg of a transaction side. Exactly ONE value expression per leg:
// an explicit Amount or a Share of the transaction total. The leg exposes no
// balance key, chart of accounts or metadata.
type V2LegInput struct {
	// Alias is the leg's account alias. The obligation is enforced BOTH by this tag and by
	// an imperative check in Translate; see buildLeg for why the two are complementary.
	Alias string `json:"alias" validate:"required"`

	// Description is the leg's own operation description, persisted on the operation this leg
	// produces. A leg that omits it produces an operation carrying the TRANSACTION-level
	// description instead — the same inheritance rule the v1 surface has always applied, decided
	// downstream by the operation builders rather than here.
	//
	// `max=256` bounds the value at the decode boundary so an oversized string is a clean 400
	// naming the field rather than a persistence-layer failure; `maxLength` publishes that bound
	// in the contract so a client reads it instead of discovering it by rejection.
	Description string `json:"description,omitempty" validate:"max=256" maxLength:"256"`

	// OrganizationID is the organization the leg's account belongs to. The `required` tag
	// makes an absent value a clean 400 at decode; the `uuid` tag does the same for a
	// malformed one. Translate enforces the presence obligation as well, for the same
	// reason it does for Alias.
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
// scope rules can report the offending entry.
type v2ScopeRef struct {
	scope V2Scope
	ref   string
}

// scope reads the scope off a leg.
func (l V2LegInput) scope() V2Scope {
	return V2Scope{OrganizationID: l.OrganizationID, LedgerID: l.LedgerID}
}

// validateV2Alias rejects a v2 account alias carrying AliasSeparator. Every alias the v2 surface
// accepts routes through here — both leg arrays — so no leg can reach the funnel with an alias
// that can be forged onto another entry's map key.
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
// Both sides are required, non-empty leg arrays; each entry produces one canonical leg with
// exactly one value expression — an explicit amount parsed into a decimal, or a share of the
// total.
//
// Route identifiers map at two independent levels: RouteID is the TRANSACTION
// route (Transaction.RouteID); OperationRouteID is the per-leg OPERATION route
// (FromTo.RouteID). A leg's own route wins; without one the leg inherits the
// request-level route. Nil route pointers stay nil so downstream ledger settings
// resolve defaults.
//
// Whether the legs sum to the transaction total, and whether the same account was named on both
// sides, are NOT checked here: the total check needs the resolved per-leg values, which Translate
// does not compute, and the same-account check needs the resolved balance-key entries the funnel
// builds. Both stay ValidateSendSourceAndDistribute's job.
func (in CreateTransactionV2Input) Translate(pending bool) (Transaction, V2Scope, error) {
	if err := in.validateSidesPresent(); err != nil {
		return Transaction{}, V2Scope{}, err
	}

	value, err := decimal.NewFromString(in.Amount)
	if err != nil || value.LessThanOrEqual(decimal.Zero) {
		return Transaction{}, V2Scope{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
	}

	from, err := in.buildLegs(in.Debits, true, "debits")
	if err != nil {
		return Transaction{}, V2Scope{}, err
	}

	to, err := in.buildLegs(in.Credits, false, "credits")
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
		Description:         in.Description,
		Code:                in.Code,
		Pending:             pending,
		Metadata:            in.Metadata,
		RouteID:             cloneStringPtr(in.RouteID),
		Send:                send,
		OperationalTypeCode: in.OperationalTypeCode,
	}, scope, nil
}

// validateSidesPresent rejects a request whose debit or credit side is empty, naming the field
// the caller has to fill.
//
// The obligation is enforced here as well as by the two arrays' `min=1` tags, for the same
// reason the leg alias obligation is: Translate is exported from a shared package, so a caller
// that assembles the input in Go and skips the decoder meets no tag at all.
func (in CreateTransactionV2Input) validateSidesPresent() error {
	if len(in.Debits) == 0 {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, "debits")
	}

	if len(in.Credits) == 0 {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, "credits")
	}

	return nil
}

// resolveScope folds every leg's scope into the single pair the request is scoped by. Every leg
// must name a complete scope, and all of them must name the SAME one.
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

// scopeRefs lists one entry per leg of the request, debit side first, each carrying the field
// reference its rejections are named by.
func (in CreateTransactionV2Input) scopeRefs() []v2ScopeRef {
	refs := make([]v2ScopeRef, 0, len(in.Debits)+len(in.Credits))
	refs = appendSideScopeRefs(refs, in.Debits, "debits")

	return appendSideScopeRefs(refs, in.Credits, "credits")
}

// appendSideScopeRefs appends one entry per leg of a single side, referenced by index.
// legsField names the side.
func appendSideScopeRefs(refs []v2ScopeRef, legs []V2LegInput, legsField string) []v2ScopeRef {
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
// assembles the input in Go meets no tag at all.
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

// buildLegs expands one side's leg array into canonical legs, one per entry. fieldName names the
// side in the per-leg error messages.
func (in CreateTransactionV2Input) buildLegs(legs []V2LegInput, isFrom bool, fieldName string) ([]FromTo, error) {
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

// legReference spells the indexed reference to one entry of a side — `debits[0]` — so a caller
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
		Description:  leg.Description,
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
