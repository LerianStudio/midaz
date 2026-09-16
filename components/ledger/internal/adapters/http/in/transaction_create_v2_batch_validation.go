// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const (
	atomicTransactionBatchV2AbsoluteMaxSize = 50
	atomicTransactionBatchV2InputLegLimit   = 1000
)

// decodedAtomicTransactionBatchV2 is the body-only admission result. Its items
// retain request order and contain no data obtained from repositories, fees,
// Tracer, Redis, or the accounting engine.
type decodedAtomicTransactionBatchV2 struct {
	items         []decodedAtomicTransactionBatchV2Item
	scope         TransactionV2Scope
	inputLegCount int
}

type decodedAtomicTransactionBatchV2Item struct {
	request                 CreateTransactionV2Request
	normalized              normalizedTransactionV2Body
	accountBlockExceptionID *uuid.UUID
}

// decodeAndValidateAtomicTransactionBatchV2 enforces request-wide wrapper and
// resource limits before collecting body-only diagnostics from each item. It is
// intentionally transport-independent so the future Huma handler can apply the
// decoded-body byte limit before calling it and start external work only after
// this function succeeds.
func decodeAndValidateAtomicTransactionBatchV2(
	rawBody []byte,
	configuredMaxSize int,
) (decodedAtomicTransactionBatchV2, error) {
	rawItems, err := decodeAtomicTransactionBatchV2Wrapper(rawBody, configuredMaxSize)
	if err != nil {
		return decodedAtomicTransactionBatchV2{}, err
	}

	inputLegCount := countAtomicTransactionBatchV2InputLegs(rawItems)
	if inputLegCount > atomicTransactionBatchV2InputLegLimit {
		return decodedAtomicTransactionBatchV2{}, pkg.ValidateBusinessError(
			constant.ErrTransactionBatchInputLegsLimitExceeded,
			constant.EntityTransaction,
			inputLegCount,
			atomicTransactionBatchV2InputLegLimit,
		)
	}

	return collectAtomicTransactionBatchV2Items(rawItems, inputLegCount)
}

func decodeAtomicTransactionBatchV2Wrapper(rawBody []byte, configuredMaxSize int) ([]json.RawMessage, error) {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &wrapper); err != nil {
		return nil, pkg.ValidateUnmarshallingError(err)
	}

	if wrapper == nil {
		return nil, pkg.ValidateUnmarshallingError(errors.New("request body must be a JSON object"))
	}

	unknownFields := make(map[string]any)
	for field, value := range wrapper {
		if field != "transactions" {
			unknownFields[field] = value
		}
	}
	if len(unknownFields) > 0 {
		return nil, pkg.ValidateBadRequestFieldsError(
			pkg.FieldValidations{},
			pkg.FieldValidations{},
			constant.EntityTransaction,
			unknownFields,
		)
	}

	var rawItems []json.RawMessage
	if rawTransactions, found := wrapper["transactions"]; found {
		if err := json.Unmarshal(rawTransactions, &rawItems); err != nil {
			return nil, pkg.ValidateUnmarshallingError(fmt.Errorf("transactions: %w", err))
		}
	}

	effectiveMaxSize := configuredMaxSize
	if effectiveMaxSize < 1 {
		effectiveMaxSize = 1
	}
	if effectiveMaxSize > atomicTransactionBatchV2AbsoluteMaxSize {
		effectiveMaxSize = atomicTransactionBatchV2AbsoluteMaxSize
	}

	if len(rawItems) == 0 || len(rawItems) > effectiveMaxSize {
		return nil, pkg.ValidateBusinessError(
			constant.ErrTransactionBatchCardinality,
			constant.EntityTransaction,
			len(rawItems),
			effectiveMaxSize,
		)
	}

	return rawItems, nil
}

func countAtomicTransactionBatchV2InputLegs(rawItems []json.RawMessage) int {
	total := 0
	for _, rawItem := range rawItems {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(rawItem, &item); err != nil {
			continue
		}

		total += countRawTransactionV2Legs(item["debits"])
		total += countRawTransactionV2Legs(item["credits"])
	}

	return total
}

func countRawTransactionV2Legs(raw json.RawMessage) int {
	var legs []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &legs) != nil {
		return 0
	}

	return len(legs)
}

func collectAtomicTransactionBatchV2Items(
	rawItems []json.RawMessage,
	inputLegCount int,
) (decodedAtomicTransactionBatchV2, error) {
	result := decodedAtomicTransactionBatchV2{
		items:         make([]decodedAtomicTransactionBatchV2Item, 0, len(rawItems)),
		inputLegCount: inputLegCount,
	}
	details := make([]pkg.FieldError, 0)

	var primary error
	var commonScope *TransactionV2Scope
	scopeMismatchReported := false
	seenExceptions := make(map[uuid.UUID]struct{})
	repeatedExceptionReported := false

	for index, rawItem := range rawItems {
		item, itemDetails, itemPrimary := collectAtomicTransactionBatchV2Item(rawItem)

		if itemScope, scopeErr := resolveTransactionV2Scope(item.request.Debits, item.request.Credits); scopeErr == nil {
			if commonScope == nil {
				commonScope = &itemScope
				result.scope = itemScope
			} else if !scopeMismatchReported && !commonScope.namesSameAs(itemScope) {
				scopeMismatchReported = true
				scopeErr = pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)
				itemDetails = append(itemDetails, pkg.FieldError{
					Location: firstTransactionV2ScopeDifference(item.request, *commonScope),
					Message:  atomicTransactionBatchV2ErrorMessage(scopeErr),
				})
				if itemPrimary == nil {
					itemPrimary = scopeErr
				}
			}
		}

		if item.accountBlockExceptionID != nil {
			if _, found := seenExceptions[*item.accountBlockExceptionID]; found {
				if !repeatedExceptionReported {
					repeatedExceptionReported = true
					repeatedErr := pkg.ValidateBusinessError(
						constant.ErrAccountBlockExceptionInvalid,
						constant.EntityTransaction,
					)
					itemDetails = append(itemDetails, pkg.FieldError{
						Location: "accountBlockExceptionId",
						Message:  "accountBlockExceptionId must not be repeated within one transaction batch",
					})
					if itemPrimary == nil {
						itemPrimary = repeatedErr
					}
				}
			} else {
				seenExceptions[*item.accountBlockExceptionID] = struct{}{}
			}
		}

		sortTransactionV2FieldErrors(itemDetails)
		for _, detail := range itemDetails {
			details = append(details, pkg.FieldError{
				Location: prefixAtomicTransactionBatchV2Location(index, detail.Location),
				Message:  detail.Message,
			})
		}

		if primary == nil && itemPrimary != nil {
			primary = itemPrimary
		}

		result.items = append(result.items, item)
		if len(details) > pkg.MaxFieldErrors {
			break
		}
	}

	if primary != nil {
		return decodedAtomicTransactionBatchV2{}, pkg.WithFieldErrors(primary, details)
	}

	return result, nil
}

func collectAtomicTransactionBatchV2Item(
	rawItem json.RawMessage,
) (decodedAtomicTransactionBatchV2Item, []pkg.FieldError, error) {
	var request CreateTransactionV2Request
	_, details, decodeErr := pkgHTTP.DecodeAndValidateWithDetails(rawItem, &request)
	item := decodedAtomicTransactionBatchV2Item{request: request}

	var responseErr pkg.ResponseError
	if decodeErr == nil || !errors.As(decodeErr, &responseErr) {
		details = append(details, collectTransactionV2PureFieldErrors(request)...)
	}

	if exceptionID, err := ParseAccountBlockExceptionID(request.AccountBlockExceptionID); err == nil {
		item.accountBlockExceptionID = exceptionID
	}

	if decodeErr != nil {
		if len(details) == 0 {
			details = append(details, pkg.FieldError{
				Message: atomicTransactionBatchV2ErrorMessage(decodeErr),
			})
		}

		return item, details, decodeErr
	}

	normalized, normalizeErr := normalizeCreateTransactionV2Body(request, false)
	if normalizeErr != nil {
		return item, details, normalizeErr
	}

	item.normalized = normalized

	return item, details, nil
}

func collectTransactionV2PureFieldErrors(request CreateTransactionV2Request) []pkg.FieldError {
	details := make([]pkg.FieldError, 0)

	if request.Amount != "" {
		if _, err := normalizeTransactionV2Amount(request.Amount); err != nil {
			details = append(details, pkg.FieldError{
				Location: "amount",
				Message:  atomicTransactionBatchV2ErrorMessage(err),
			})
		}
	}

	details = append(details, collectTransactionV2LegPureFieldErrors(request.Debits, "debits")...)
	details = append(details, collectTransactionV2LegPureFieldErrors(request.Credits, "credits")...)

	if location, mismatch := firstTransactionV2InternalScopeDifference(request); mismatch {
		err := pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)
		details = append(details, pkg.FieldError{
			Location: location,
			Message:  atomicTransactionBatchV2ErrorMessage(err),
		})
	}

	sortTransactionV2FieldErrors(details)

	return details
}

func collectTransactionV2LegPureFieldErrors(legs []TransactionV2LegRequest, side string) []pkg.FieldError {
	details := make([]pkg.FieldError, 0)

	for index, leg := range legs {
		location := legReference(side, index)

		if leg.Alias != "" {
			if err := validateV2Alias(leg.Alias); err != nil {
				details = append(details, pkg.FieldError{
					Location: location + ".alias",
					Message:  atomicTransactionBatchV2ErrorMessage(err),
				})
			}
		}

		switch {
		case leg.Amount != "" && leg.Share == nil:
			if _, err := normalizeTransactionV2Amount(leg.Amount); err != nil {
				details = append(details, pkg.FieldError{
					Location: location + ".amount",
					Message:  atomicTransactionBatchV2ErrorMessage(err),
				})
			}
		case leg.Share != nil && leg.Amount == "":
			// Share bounds are completely represented by decode tags.
		default:
			err := invalidLegExpression(location)
			details = append(details, pkg.FieldError{
				Location: location,
				Message:  atomicTransactionBatchV2ErrorMessage(err),
			})
		}
	}

	return details
}

func firstTransactionV2InternalScopeDifference(request CreateTransactionV2Request) (string, bool) {
	refs := make([]v2ScopeRef, 0, len(request.Debits)+len(request.Credits))
	refs = appendTransactionV2ScopeRefs(refs, request.Debits, "debits")
	refs = appendTransactionV2ScopeRefs(refs, request.Credits, "credits")
	if len(refs) < 2 || refs[0].requireComplete() != nil {
		return "", false
	}

	base := refs[0].scope
	for _, ref := range refs[1:] {
		if ref.requireComplete() != nil || base.namesSameAs(ref.scope) {
			continue
		}

		return transactionV2ScopeDifferenceLocation(ref.ref, base, ref.scope), true
	}

	return "", false
}

func firstTransactionV2ScopeDifference(request CreateTransactionV2Request, expected TransactionV2Scope) string {
	refs := make([]v2ScopeRef, 0, len(request.Debits)+len(request.Credits))
	refs = appendTransactionV2ScopeRefs(refs, request.Debits, "debits")
	refs = appendTransactionV2ScopeRefs(refs, request.Credits, "credits")

	for _, ref := range refs {
		if ref.requireComplete() == nil && !expected.namesSameAs(ref.scope) {
			return transactionV2ScopeDifferenceLocation(ref.ref, expected, ref.scope)
		}
	}

	return ""
}

func transactionV2ScopeDifferenceLocation(ref string, expected, actual TransactionV2Scope) string {
	if !strings.EqualFold(expected.OrganizationID, actual.OrganizationID) {
		return ref + ".organizationId"
	}

	return ref + ".ledgerId"
}

func sortTransactionV2FieldErrors(details []pkg.FieldError) {
	sort.SliceStable(details, func(i, j int) bool {
		if details[i].Location == details[j].Location {
			return details[i].Message < details[j].Message
		}

		return details[i].Location < details[j].Location
	})
}

func prefixAtomicTransactionBatchV2Location(index int, location string) string {
	prefix := "body.transactions[" + strconv.Itoa(index) + "]"
	if location == "" {
		return prefix
	}

	return prefix + "." + location
}

func atomicTransactionBatchV2ErrorMessage(err error) string {
	var responseErr pkg.ResponseError
	if errors.As(err, &responseErr) {
		return responseErr.Message
	}

	if detail, ok := pkgHTTP.ProblemDetail(err); ok {
		return detail.ErrorModel.Detail
	}

	return err.Error()
}
