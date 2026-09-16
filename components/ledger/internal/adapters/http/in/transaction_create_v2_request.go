// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateTransactionV2Request is the request payload for the Transaction API v2. It
// mirrors the canonical Transaction shape explicitly rather than embedding
// mmodel/canonical types, so domain evolution never leaks onto the wire contract.
// It is published as CreateTransactionV2Input to preserve the existing OpenAPI contract.
//
// Each side of the transaction is a leg array (Debits/Credits): one debit paired with
// many credits, or the reverse, is a valid request. Each leg names the account alias
// and the organization and ledger that account belongs to. Description, Code, Asset,
// Amount, RouteID, OperationRouteID and Metadata sit alongside the two leg arrays.
// Amount is the transaction total that the legs' share expressions divide.
type CreateTransactionV2Request struct {
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
	Debits []TransactionV2LegRequest `json:"debits" validate:"min=1,max=500,dive" minItems:"1" maxItems:"500" nullable:"false"`

	// Credits are the credit legs of the transaction, tagged for the same reasons as
	// Debits.
	Credits []TransactionV2LegRequest `json:"credits" validate:"min=1,max=500,dive" minItems:"1" maxItems:"500" nullable:"false"`

	// RouteID is the optional TRANSACTION route UUID. Validated as a UUID at
	// decode (same tag as the v1 input) so a malformed value is a clean 400, not
	// a deep funnel error.
	RouteID *string `json:"routeId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// OperationRouteID is the optional per-leg OPERATION route UUID. Validated as
	// a UUID at decode for the same reason as RouteID.
	OperationRouteID *string `json:"operationRouteId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Metadata holds flat custom key-value attributes. Values must be flat
	// (string, number, boolean) — no nested objects.
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,noreservedkey,endkeys,omitempty,nonested,valuemax=2000"`

	// Skip carries the per-call control opt-outs. Each flag is honored only when the
	// matching per-ledger override is enabled; otherwise the request is rejected with 422.
	// The controls it opts out of — the fee engine and the tracer reservation — exist only
	// on this contract, so the field does too.
	Skip *mtransaction.TransactionSkip `json:"skip,omitempty"`

	// AccountBlockExceptionID presents a single-use account-block exception,
	// minted by the block-exception create route. It authorizes ONE debit of an
	// exact amount out of a blocked source account, and dies on this use.
	//
	// Additive and optional: an absent field leaves every barrier exactly as it
	// was before the field existed. Present, it is validated and CONSUMED even by
	// a transaction that needed no bypass — an identifier must not outlive a
	// request that presented it — and a presented identifier that does not
	// authorize this transaction's source account and debited amount rejects it.
	//
	// The DIRECT action accepts it; the HOLD rejects it (Translate, 0509), because
	// a two-phase transaction would need two grants: one for the hold and one for
	// the commit. A pending is released by presenting a grant on the commit.
	//
	// The field lives on this contract only, the same way Skip does, because the
	// route that mints an exception is /v2-only: a /v1 body naming it is a 400
	// unknown field.
	AccountBlockExceptionID *string `json:"accountBlockExceptionId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid" doc:"Single-use account-block exception identifier. Authorizes one debit of an exact amount out of a blocked source account, and is consumed on use. Rejected on the hold action."`
}

// TransactionV2LegRequest is one leg of a transaction side. Exactly ONE value expression per leg:
// an explicit Amount or a Share of the transaction total. The leg exposes no
// balance key, chart of accounts or metadata.
// It is published as V2LegInput to preserve the existing OpenAPI contract.
type TransactionV2LegRequest struct {
	// Alias is the leg's account alias. The obligation is enforced BOTH by this tag and by
	// an imperative check in Translate; see normalizeTransactionV2Leg for why the two are complementary.
	//
	// The accepted SPELLINGS are enforced by validateV2Alias rather than by a tag, because the
	// fee routes decode through a second validator instance that panics on a tag it does not
	// know. The doc tag publishes the rule so a client reads it instead of discovering it by
	// rejection.
	Alias string `json:"alias" validate:"required" example:"@person1" doc:"The leg's account alias. Accepts letters, digits and the characters @ : _ and -, or an external account alias spelled @external/ followed by the uppercase asset code. Any other spelling is refused with 400 before the transaction is calculated."`

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
	Share *TransactionV2ShareRequest `json:"share,omitempty"`

	// OperationRouteID is the leg's OPERATION route UUID, overriding the
	// request-level OperationRouteID for this leg. Validated as a UUID at decode
	// (same tag as the request-level field) so a malformed value is a clean 400,
	// not a deep funnel error.
	OperationRouteID *string `json:"operationRouteId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
}

// TransactionV2ShareRequest expresses a leg's value as a percentage of the transaction total.
// It is published as V2ShareInput to preserve the existing OpenAPI contract. The resolver
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
type TransactionV2ShareRequest struct {
	// Percentage is the leg's share of the transaction total, in percent, bounded to 1..100. It
	// must be positive: a zero share moves nothing while the transaction still commits, and a
	// negative one inverts the leg's accounting direction. normalizeTransactionV2Leg enforces the lower bound
	// imperatively as well, covering adapter tests that build the request in Go and skip the decoder.
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

// TransactionV2Scope is the organization and ledger a v2 request is scoped by: the pair every leg of the
// request named. Translate resolves it from the body and hands it back, so a caller scopes the
// transaction by what the request says rather than by where it was posted.
//
// The identifiers are carried as the client spelled them. Their UUID shape is a contract
// obligation the decode boundary answers, so this type asserts no format of its own.
type TransactionV2Scope struct {
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
func (s TransactionV2Scope) namesSameAs(other TransactionV2Scope) bool {
	return strings.EqualFold(s.OrganizationID, other.OrganizationID) &&
		strings.EqualFold(s.LedgerID, other.LedgerID)
}

// accountAliasCharset is the registered account alias charset, compiled once at package level
// because validateV2Alias runs per leg and a 500-leg body would otherwise recompile it 500 times.
var accountAliasCharset = regexp.MustCompile(constant.AccountAliasAcceptedChars)

// validateV2Alias refuses a v2 account alias that no account can carry. Every alias the v2 surface
// accepts routes through here, both leg arrays and the fee estimate, so no leg reaches the funnel
// or the fee engine spelled as something the ledger can never resolve.
//
// An alias an account CAN carry is one of exactly two shapes. The first is the registered charset,
// AccountAliasAcceptedChars, which is what the account create route enforces on every alias it
// stores. The second is the external virtual account, DefaultExternalAccountAliasPrefix followed
// by an asset code, which sits outside that charset because of its slash and is the only way to
// spell funding or withdrawal on a surface that publishes no inflow or outflow action.
//
// The asset half reuses utils.ValidateCode, the ledger's own asset-code rule, rather than
// restating it: an asset is created only after that rule passes, so reusing it is what keeps this
// guard from refusing an external alias the ledger would happily resolve, or admitting one it
// never could. ValidateCode returns nil on an empty string, which the asset create route answers
// with its own required tag, so the non-empty obligation is spelled here.
//
// Two classes of damage sit behind the refusal. The fee engine builds its internal leg keys with
// an arrow and cuts a leg alias at the first one, so a caller leg aliased dst->ops posts to the
// account dst while the body reads as naming something else. Separately, an alias is rewritten
// into a composite separator-joined form before downstream code keys its per-entry maps on it,
// and an alias already spelled in that shape reaches those maps unmutated, where it collides with
// another entry's key or matches none of them; either way an entry is lost, and a transaction
// that loses one side's entry moves value in one direction only. The separator is outside the
// charset, so that older rule is now a consequence of this one rather than a second code path.
func validateV2Alias(alias string) error {
	if accountAliasCharset.MatchString(alias) {
		return nil
	}

	if code, isExternal := strings.CutPrefix(alias, constant.DefaultExternalAccountAliasPrefix); isExternal &&
		code != "" && utils.ValidateCode(code) == nil {
		return nil
	}

	return pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, constant.EntityTransaction)
}

// Translate converts the flat v2 request into the canonical Transaction and TransactionV2Scope the
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
func (in CreateTransactionV2Request) Translate(pending bool) (mtransaction.Transaction, TransactionV2Scope, error) {
	normalized, err := normalizeCreateTransactionV2Body(in, pending)
	if err != nil {
		return mtransaction.Transaction{}, TransactionV2Scope{}, err
	}

	return normalized.transaction, normalized.scope, nil
}

// LifecycleV2Request is the OPTIONAL request body of the /v2 lifecycle actions that accept
// a single-use account-block exception: commit and revert.
//
// Those two actions address an existing transaction and take every other input from the
// URL, so they shipped bodiless. The grant cannot come from the URL — it is a secret an
// operator mints out of band and hands to one request — so the actions grow a body whose
// only field is that grant, and the body stays OPTIONAL: a request that sends none is the
// exact request they accepted before, byte for byte.
//
// Cancel is deliberately absent. A cancel is never barred by an account block (it returns
// on-hold funds or aborts a future credit, so blocking it would deadlock an innocent
// counterparty), which leaves it nothing a grant could unlock.
type LifecycleV2Request struct {
	// AccountBlockExceptionID presents a single-use account-block exception. Optional;
	// see CreateTransactionV2Request.AccountBlockExceptionID for what a grant authorizes
	// and when it is consumed.
	AccountBlockExceptionID *string `json:"accountBlockExceptionId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid" doc:"Single-use account-block exception identifier. Authorizes one debit of an exact amount out of a blocked source account, and is consumed on use. Rejected on the hold action."`
}

// AccountBlockException returns the presented exception identifier parsed into a UUID, or
// nil when the body presented none.
func (in LifecycleV2Request) AccountBlockException() (*uuid.UUID, error) {
	return ParseAccountBlockExceptionID(in.AccountBlockExceptionID)
}

// AccountBlockException returns the presented exception identifier parsed into a UUID,
// or nil when the body presented none.
func (in CreateTransactionV2Request) AccountBlockException() (*uuid.UUID, error) {
	return ParseAccountBlockExceptionID(in.AccountBlockExceptionID)
}

// ParseAccountBlockExceptionID parses an optional account-block exception identifier.
// nil in, nil out.
//
// An HTTP caller never reaches the failure branch: the field's `uuid` validate tag
// refuses a malformed value at decode with a 400 naming the field. What does reach it
// is an input assembled in Go that skipped the decoder, and a value that cannot be a
// UUID cannot name a minted exception either — so it is refused as an invalid
// exception (0508) rather than dropped, which would silently downgrade the request to
// one presenting no grant at all.
func ParseAccountBlockExceptionID(raw *string) (*uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}

	parsed, err := uuid.Parse(*raw)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionInvalid, constant.EntityTransaction)
	}

	return &parsed, nil
}
