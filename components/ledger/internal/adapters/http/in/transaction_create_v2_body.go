// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// decodeCreateTransactionV2Body runs the singular direct-v2 strict decode and
// tag-validation pipeline without transport or external work. Batch validation
// can apply the same helper to each ordered raw item while retaining the
// singular precedence of malformed JSON, unknown fields, and struct tags.
func decodeCreateTransactionV2Body(rawBody []byte) (CreateTransactionV2Request, error) {
	var input CreateTransactionV2Request
	if _, details, err := pkgHTTP.DecodeAndValidateWithDetails(rawBody, &input); err != nil {
		return CreateTransactionV2Request{}, pkg.WithFieldErrors(err, details)
	}

	return input, nil
}

// normalizedTransactionV2Body is the complete body-only result needed before a
// direct-v2 request can enter a use case. It contains no repository, fee, Tracer,
// Redis, or accounting state and is therefore safe to reuse while validating a
// batch before external work begins.
type normalizedTransactionV2Body struct {
	transaction mtransaction.Transaction
	scope       TransactionV2Scope
}

type normalizedCrossLedgerTransactionV2Body struct {
	transaction  mtransaction.Transaction
	scopes       []TransactionV2Scope
	debitScopes  []TransactionV2Scope
	creditScopes []TransactionV2Scope
}

// normalizeCreateCrossLedgerTransactionV2Body applies the same body-only rules
// as the singular translator but preserves the scope of every leg instead of
// requiring one common ledger.
func normalizeCreateCrossLedgerTransactionV2Body(in CreateTransactionV2Request, pending bool) (normalizedCrossLedgerTransactionV2Body, error) {
	if err := validateTransactionV2SidesPresent(in.Debits, in.Credits); err != nil {
		return normalizedCrossLedgerTransactionV2Body{}, err
	}

	if err := validateTransactionV2AccountBlockExceptionSurface(pending, in.AccountBlockExceptionID); err != nil {
		return normalizedCrossLedgerTransactionV2Body{}, err
	}

	value, err := normalizeTransactionV2Amount(in.Amount)
	if err != nil {
		return normalizedCrossLedgerTransactionV2Body{}, err
	}

	from, err := normalizeTransactionV2Legs(in.Asset, in.OperationRouteID, in.Debits, true, "debits")
	if err != nil {
		return normalizedCrossLedgerTransactionV2Body{}, err
	}

	to, err := normalizeTransactionV2Legs(in.Asset, in.OperationRouteID, in.Credits, false, "credits")
	if err != nil {
		return normalizedCrossLedgerTransactionV2Body{}, err
	}

	debitScopes, creditScopes, scopes, err := resolveTransactionV2LegScopes(in.Debits, in.Credits)
	if err != nil {
		return normalizedCrossLedgerTransactionV2Body{}, err
	}

	return normalizedCrossLedgerTransactionV2Body{
		transaction: mtransaction.Transaction{
			Description: in.Description, Code: in.Code, Pending: pending, Metadata: in.Metadata,
			RouteID: cloneStringPtr(in.RouteID), Skip: cloneTransactionSkip(in.Skip),
			Send: mtransaction.Send{
				Asset: in.Asset, Value: value, Source: mtransaction.Source{From: from}, Distribute: mtransaction.Distribute{To: to},
			},
		},
		scopes: scopes, debitScopes: debitScopes, creditScopes: creditScopes,
	}, nil
}

func resolveTransactionV2LegScopes(
	debits, credits []TransactionV2LegRequest,
) ([]TransactionV2Scope, []TransactionV2Scope, []TransactionV2Scope, error) {
	debitScopes := make([]TransactionV2Scope, len(debits))
	creditScopes := make([]TransactionV2Scope, len(credits))
	unique := make([]TransactionV2Scope, 0, len(debits)+len(credits))

	appendScope := func(scope TransactionV2Scope, ref string) error {
		if err := (v2ScopeRef{scope: scope, ref: ref}).requireComplete(); err != nil {
			return err
		}

		for _, existing := range unique {
			if existing.namesSameAs(scope) {
				return nil
			}
		}

		unique = append(unique, scope)

		return nil
	}

	for index, leg := range debits {
		debitScopes[index] = TransactionV2Scope{OrganizationID: leg.OrganizationID, LedgerID: leg.LedgerID}
		if err := appendScope(debitScopes[index], legReference("debits", index)); err != nil {
			return nil, nil, nil, err
		}
	}

	for index, leg := range credits {
		creditScopes[index] = TransactionV2Scope{OrganizationID: leg.OrganizationID, LedgerID: leg.LedgerID}
		if err := appendScope(creditScopes[index], legReference("credits", index)); err != nil {
			return nil, nil, nil, err
		}
	}

	return debitScopes, creditScopes, unique, nil
}

// normalizeCreateTransactionV2Body applies the pure direct-v2 translation rules
// in their released singular precedence:
//
//  1. required debit and credit sides;
//  2. action-specific account-block-exception surface;
//  3. positive transaction amount;
//  4. debit legs in array order;
//  5. credit legs in array order;
//  6. complete and common body scope in debit-then-credit order.
//
// Keeping the orchestration here gives singular creation and the batch structural
// collector one reusable source for body-only normalization and validation.
func normalizeCreateTransactionV2Body(in CreateTransactionV2Request, pending bool) (normalizedTransactionV2Body, error) {
	if err := validateTransactionV2SidesPresent(in.Debits, in.Credits); err != nil {
		return normalizedTransactionV2Body{}, err
	}

	if err := validateTransactionV2AccountBlockExceptionSurface(pending, in.AccountBlockExceptionID); err != nil {
		return normalizedTransactionV2Body{}, err
	}

	value, err := normalizeTransactionV2Amount(in.Amount)
	if err != nil {
		return normalizedTransactionV2Body{}, err
	}

	from, err := normalizeTransactionV2Legs(in.Asset, in.OperationRouteID, in.Debits, true, "debits")
	if err != nil {
		return normalizedTransactionV2Body{}, err
	}

	to, err := normalizeTransactionV2Legs(in.Asset, in.OperationRouteID, in.Credits, false, "credits")
	if err != nil {
		return normalizedTransactionV2Body{}, err
	}

	scope, err := resolveTransactionV2Scope(in.Debits, in.Credits)
	if err != nil {
		return normalizedTransactionV2Body{}, err
	}

	return normalizedTransactionV2Body{
		transaction: mtransaction.Transaction{
			Description: in.Description,
			Code:        in.Code,
			Pending:     pending,
			Metadata:    in.Metadata,
			RouteID:     cloneStringPtr(in.RouteID),
			Send: mtransaction.Send{
				Asset:      in.Asset,
				Value:      value,
				Source:     mtransaction.Source{From: from},
				Distribute: mtransaction.Distribute{To: to},
			},
			Skip: cloneTransactionSkip(in.Skip),
		},
		scope: scope,
	}, nil
}

// validateTransactionV2SidesPresent rejects a request whose debit or credit
// side is empty, naming the first field the caller has to fill. The imperative
// rule also covers callers that assemble the request in Go and skip struct tags.
func validateTransactionV2SidesPresent(debits, credits []TransactionV2LegRequest) error {
	if len(debits) == 0 {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, "debits")
	}

	if len(credits) == 0 {
		return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, "credits")
	}

	return nil
}

// validateTransactionV2AccountBlockExceptionSurface rejects an account-block
// exception presented on HOLD. DIRECT accepts an optional identifier; its UUID
// spelling is validated by the decode tags and ParseAccountBlockExceptionID.
func validateTransactionV2AccountBlockExceptionSurface(pending bool, exceptionID *string) error {
	if pending && exceptionID != nil {
		return pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionNotSupported, constant.EntityTransaction, "hold")
	}

	return nil
}

// normalizeTransactionV2Amount parses a required monetary string without using
// float64 and rejects malformed, zero, and negative values with the released
// singular business error.
func normalizeTransactionV2Amount(raw string) (decimal.Decimal, error) {
	value, err := decimal.NewFromString(raw)
	if err != nil || value.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
	}

	return value, nil
}

// normalizeTransactionV2Legs expands one side into canonical legs in array order.
func normalizeTransactionV2Legs(asset string, defaultOperationRouteID *string, legs []TransactionV2LegRequest, isFrom bool, fieldName string) ([]mtransaction.FromTo, error) {
	out := make([]mtransaction.FromTo, 0, len(legs))

	for i, leg := range legs {
		built, err := normalizeTransactionV2Leg(asset, defaultOperationRouteID, leg, isFrom, legReference(fieldName, i))
		if err != nil {
			return nil, err
		}

		out = append(out, built)
	}

	return out, nil
}

// normalizeTransactionV2Leg maps one array entry onto a canonical leg. It
// validates the alias and requires exactly one positive amount or share value.
func normalizeTransactionV2Leg(asset string, defaultOperationRouteID *string, leg TransactionV2LegRequest, isFrom bool, legRef string) (mtransaction.FromTo, error) {
	if leg.Alias == "" {
		return mtransaction.FromTo{}, pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, constant.EntityTransaction, legRef+".alias")
	}

	if err := validateV2Alias(leg.Alias); err != nil {
		return mtransaction.FromTo{}, err
	}

	route := leg.OperationRouteID
	if route == nil {
		route = defaultOperationRouteID
	}

	built := mtransaction.FromTo{
		AccountAlias: leg.Alias,
		BalanceKey:   leg.BalanceKey,
		Description:  leg.Description,
		RouteID:      cloneStringPtr(route),
		IsFrom:       isFrom,
	}

	switch {
	case leg.Amount != "" && leg.Share == nil:
		value, err := normalizeTransactionV2Amount(leg.Amount)
		if err != nil {
			return mtransaction.FromTo{}, err
		}

		built.Amount = &mtransaction.Amount{Asset: asset, Value: value}
	case leg.Share != nil && leg.Amount == "":
		if leg.Share.Percentage <= 0 {
			return mtransaction.FromTo{}, pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
		}

		built.Share = &mtransaction.Share{
			Percentage:             leg.Share.Percentage,
			PercentageOfPercentage: leg.Share.PercentageOfPercentage,
		}
	default:
		return mtransaction.FromTo{}, invalidLegExpression(legRef)
	}

	return built, nil
}

// v2ScopeRef pairs one leg's scope with the indexed field reference named by a
// missing-scope error.
type v2ScopeRef struct {
	scope TransactionV2Scope
	ref   string
}

// resolveTransactionV2Scope folds every leg into one complete body scope. The
// first debit leg's spelling wins and all later legs must name the same pair.
func resolveTransactionV2Scope(debits, credits []TransactionV2LegRequest) (TransactionV2Scope, error) {
	refs := make([]v2ScopeRef, 0, len(debits)+len(credits))
	refs = appendTransactionV2ScopeRefs(refs, debits, "debits")
	refs = appendTransactionV2ScopeRefs(refs, credits, "credits")

	var resolved TransactionV2Scope

	for i, ref := range refs {
		if err := ref.requireComplete(); err != nil {
			return TransactionV2Scope{}, err
		}

		if i == 0 {
			resolved = ref.scope

			continue
		}

		if !resolved.namesSameAs(ref.scope) {
			return TransactionV2Scope{}, pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)
		}
	}

	return resolved, nil
}

// appendTransactionV2ScopeRefs appends one indexed reference per leg.
func appendTransactionV2ScopeRefs(refs []v2ScopeRef, legs []TransactionV2LegRequest, fieldName string) []v2ScopeRef {
	for i, leg := range legs {
		refs = append(refs, v2ScopeRef{
			scope: TransactionV2Scope{OrganizationID: leg.OrganizationID, LedgerID: leg.LedgerID},
			ref:   legReference(fieldName, i),
		})
	}

	return refs
}

// requireComplete rejects the first missing half of one leg's scope.
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

// legReference spells one zero-based side entry such as debits[0].
func legReference(fieldName string, index int) string {
	return fieldName + "[" + strconv.Itoa(index) + "]"
}

// invalidLegExpression rejects an entry that does not fill exactly one value
// expression, naming only the two expressions published by direct-v2.
func invalidLegExpression(legRef string) error {
	return pkg.ValidateTransactionTypeError(constant.EntityTransaction,
		constant.TransactionTypeOptionsLeg, legRef)
}

// cloneStringPtr returns an independent copy of p, or nil when p is nil.
func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}

	v := *p

	return &v
}

// cloneTransactionSkip returns an independent copy of s, or nil when s is nil.
func cloneTransactionSkip(s *mtransaction.TransactionSkip) *mtransaction.TransactionSkip {
	if s == nil {
		return nil
	}

	clone := *s

	return &clone
}
