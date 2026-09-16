// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type atomicTransactionBatchV2Action string

const (
	atomicTransactionBatchV2ActionDirect atomicTransactionBatchV2Action = "direct"
	atomicTransactionBatchV2ActionHold   atomicTransactionBatchV2Action = "hold"
)

// revisedDecodedAtomicTransactionBatchV2 is deliberately internal while the
// public route still serves the original direct-only contract. Items are in
// execution order, but originalIndex always refers to the received JSON array.
type revisedDecodedAtomicTransactionBatchV2 struct {
	items         []revisedDecodedAtomicTransactionBatchV2Item
	scope         TransactionV2Scope
	inputLegCount int
}

type revisedDecodedAtomicTransactionBatchV2Item struct {
	request                 CreateTransactionV2Request
	action                  atomicTransactionBatchV2Action
	order                   int
	originalIndex           int
	accountBlockExceptionID *uuid.UUID
}

type revisedAtomicTransactionBatchV2ItemState struct {
	item    revisedDecodedAtomicTransactionBatchV2Item
	details []pkg.FieldError
	valid   bool
}

// decodeAndValidateRevisedAtomicTransactionBatchV2 validates the revised
// direct/hold item envelope without changing the route that currently uses the
// direct-only decoder. It fully collects body-only diagnostics before returning
// the batch-specific primary error and never uses a map for execution order.
func decodeAndValidateRevisedAtomicTransactionBatchV2(
	rawBody []byte,
	configuredMaxSize int,
) (revisedDecodedAtomicTransactionBatchV2, error) {
	rawItems, err := decodeAtomicTransactionBatchV2Wrapper(rawBody, configuredMaxSize)
	if err != nil {
		return revisedDecodedAtomicTransactionBatchV2{}, err
	}

	inputLegCount := countAtomicTransactionBatchV2InputLegs(rawItems)
	if inputLegCount > atomicTransactionBatchV2InputLegLimit {
		return revisedDecodedAtomicTransactionBatchV2{}, pkg.ValidateBusinessError(
			constant.ErrTransactionBatchInputLegsLimitExceeded,
			constant.EntityTransaction,
			inputLegCount,
			atomicTransactionBatchV2InputLegLimit,
		)
	}

	states := make([]revisedAtomicTransactionBatchV2ItemState, len(rawItems))
	for index, rawItem := range rawItems {
		states[index] = collectRevisedAtomicTransactionBatchV2Item(rawItem, index, len(rawItems))
	}

	sequenceValid := validateRevisedAtomicTransactionBatchV2Orders(states)
	if sequenceValid {
		sort.SliceStable(states, func(i, j int) bool {
			return states[i].item.order < states[j].item.order
		})
	}

	collectRevisedAtomicTransactionBatchV2ScopeDiagnostics(states)
	collectRevisedAtomicTransactionBatchV2RepeatedExceptionDiagnostics(states)

	details := revisedAtomicTransactionBatchV2Details(states, sequenceValid)
	if len(details) > 0 {
		return revisedDecodedAtomicTransactionBatchV2{}, pkg.WithFieldErrors(
			pkg.ValidateBusinessError(constant.ErrTransactionBatchStructuralValidation, constant.EntityTransaction),
			details,
		)
	}

	result := revisedDecodedAtomicTransactionBatchV2{
		items:         make([]revisedDecodedAtomicTransactionBatchV2Item, len(states)),
		inputLegCount: inputLegCount,
	}
	for index, state := range states {
		result.items[index] = state.item
	}
	if len(result.items) > 0 {
		if scope, scopeErr := resolveTransactionV2Scope(result.items[0].request.Debits, result.items[0].request.Credits); scopeErr == nil {
			result.scope = scope
		}
	}

	return result, nil
}

func collectRevisedAtomicTransactionBatchV2Item(
	rawItem json.RawMessage,
	originalIndex int,
	batchSize int,
) revisedAtomicTransactionBatchV2ItemState {
	state := revisedAtomicTransactionBatchV2ItemState{
		item: revisedDecodedAtomicTransactionBatchV2Item{originalIndex: originalIndex},
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawItem, &fields); err != nil || fields == nil {
		state.details = append(state.details, pkg.FieldError{
			Message: "transaction item must be a JSON object",
		})

		return state
	}

	action, actionDetails := parseRevisedAtomicTransactionBatchV2Action(fields["action"])
	state.item.action = action
	state.details = append(state.details, actionDetails...)

	order, orderDetails, orderValid := parseRevisedAtomicTransactionBatchV2Order(fields["order"], batchSize)
	state.item.order = order
	state.valid = orderValid
	state.details = append(state.details, orderDetails...)

	delete(fields, "action")
	delete(fields, "order")
	requestRaw, err := json.Marshal(fields)
	if err != nil {
		state.details = append(state.details, pkg.FieldError{Message: "transaction item could not be decoded"})

		return state
	}

	item, itemDetails, itemErr := collectAtomicTransactionBatchV2Item(requestRaw)
	state.item.request = item.request
	state.item.accountBlockExceptionID = item.accountBlockExceptionID
	state.details = append(state.details, itemDetails...)
	if itemErr != nil && len(itemDetails) == 0 {
		state.details = append(state.details, pkg.FieldError{
			Message: atomicTransactionBatchV2ErrorMessage(itemErr),
		})
	}

	return state
}

func parseRevisedAtomicTransactionBatchV2Action(raw json.RawMessage) (atomicTransactionBatchV2Action, []pkg.FieldError) {
	if len(raw) == 0 {
		return "", []pkg.FieldError{{
			Location: "action",
			Message:  "action is required and must be either direct or hold",
		}}
	}

	var action string
	if err := json.Unmarshal(raw, &action); err != nil {
		return "", []pkg.FieldError{{
			Location: "action",
			Message:  "action must be either direct or hold",
		}}
	}

	switch atomicTransactionBatchV2Action(action) {
	case atomicTransactionBatchV2ActionDirect, atomicTransactionBatchV2ActionHold:
		return atomicTransactionBatchV2Action(action), nil
	default:
		return "", []pkg.FieldError{{
			Location: "action",
			Message:  "action must be either direct or hold",
		}}
	}
}

func parseRevisedAtomicTransactionBatchV2Order(
	raw json.RawMessage,
	batchSize int,
) (int, []pkg.FieldError, bool) {
	if len(raw) == 0 {
		return 0, []pkg.FieldError{{
			Location: "order",
			Message:  "order is required",
		}}, false
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, []pkg.FieldError{{
			Location: "order",
			Message:  "order must be an integer",
		}}, false
	}

	number, ok := value.(json.Number)
	if !ok {
		return 0, []pkg.FieldError{{
			Location: "order",
			Message:  "order must be an integer",
		}}, false
	}

	order, err := strconv.Atoi(number.String())
	if err != nil || order < 1 || order > batchSize {
		return 0, []pkg.FieldError{{
			Location: "order",
			Message:  fmt.Sprintf("order must be an integer between 1 and %d", batchSize),
		}}, false
	}

	return order, nil, true
}

func validateRevisedAtomicTransactionBatchV2Orders(states []revisedAtomicTransactionBatchV2ItemState) bool {
	byOrder := make(map[int][]int, len(states))
	valid := true
	for index := range states {
		if !states[index].valid {
			valid = false
			continue
		}

		byOrder[states[index].item.order] = append(byOrder[states[index].item.order], index)
	}

	for _, indexes := range byOrder {
		if len(indexes) < 2 {
			continue
		}

		valid = false
		for _, index := range indexes {
			states[index].details = append(states[index].details, pkg.FieldError{
				Location: "order",
				Message:  "order must be unique within the batch",
			})
		}
	}

	return valid
}

func collectRevisedAtomicTransactionBatchV2ScopeDiagnostics(states []revisedAtomicTransactionBatchV2ItemState) {
	var commonScope *TransactionV2Scope
	for index := range states {
		itemScope, err := resolveTransactionV2Scope(states[index].item.request.Debits, states[index].item.request.Credits)
		if err != nil {
			continue
		}

		if commonScope == nil {
			commonScope = &itemScope
			continue
		}

		if commonScope.namesSameAs(itemScope) {
			continue
		}

		states[index].details = append(states[index].details, pkg.FieldError{
			Location: firstTransactionV2ScopeDifference(states[index].item.request, *commonScope),
			Message:  "transaction scope must match every item in the batch",
		})
	}
}

func collectRevisedAtomicTransactionBatchV2RepeatedExceptionDiagnostics(states []revisedAtomicTransactionBatchV2ItemState) {
	byException := make(map[uuid.UUID][]int)
	for index := range states {
		if states[index].item.accountBlockExceptionID != nil {
			byException[*states[index].item.accountBlockExceptionID] = append(byException[*states[index].item.accountBlockExceptionID], index)
		}
	}

	for _, indexes := range byException {
		if len(indexes) < 2 {
			continue
		}

		for _, index := range indexes {
			states[index].details = append(states[index].details, pkg.FieldError{
				Location: "accountBlockExceptionId",
				Message:  "accountBlockExceptionId must not be repeated within one transaction batch",
			})
		}
	}
}

func revisedAtomicTransactionBatchV2Details(
	states []revisedAtomicTransactionBatchV2ItemState,
	sequenceValid bool,
) []pkg.FieldError {
	if !sequenceValid {
		sort.SliceStable(states, func(i, j int) bool {
			return states[i].item.originalIndex < states[j].item.originalIndex
		})
	}

	details := make([]pkg.FieldError, 0)
	for _, state := range states {
		sortTransactionV2FieldErrors(state.details)
		for _, detail := range state.details {
			details = append(details, pkg.FieldError{
				Location: prefixAtomicTransactionBatchV2Location(state.item.originalIndex, detail.Location),
				Message:  revisedAtomicTransactionBatchV2DetailMessage(state.item, detail.Message),
			})
		}
	}

	return details
}

func revisedAtomicTransactionBatchV2DetailMessage(item revisedDecodedAtomicTransactionBatchV2Item, message string) string {
	if item.order > 0 {
		return fmt.Sprintf("Transaction order %d: %s", item.order, message)
	}

	return fmt.Sprintf("Transaction at index %d: %s", item.originalIndex, message)
}
