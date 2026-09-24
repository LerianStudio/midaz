// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// installContextReservationSchemas describes the strict decoder without coupling
// the shared facts package to Huma. Pointers retain presence in Go, but null is
// never a valid explicit boolean or list in the wire contract.
func installContextReservationSchemas(registry huma.Registry) {
	request := registry.Schema(reflect.TypeFor[tracercontract.ReserveRequest](), false, "")
	request.Properties["longLived"].Nullable = false
	request.Properties["validationMode"].Enum = []any{string(tracercontract.ValidationLimits), string(tracercontract.ValidationRulesAndLimits)}
	account := registry.Schema(reflect.TypeFor[tracercontract.Account](), false, "")
	account.Properties["blocked"].Nullable = false
	contextSchema := registry.Schema(reflect.TypeFor[tracercontract.Context](), false, "")
	contextSchema.Properties["accounts"].Nullable = false
	contextSchema.Properties["entries"].Nullable = false
	entry := registry.Schema(reflect.TypeFor[tracercontract.Entry](), false, "")
	entry.Properties["direction"].Enum = []any{string(tracercontract.Debit), string(tracercontract.Credit)}
	result := registry.Schema(reflect.TypeFor[tracercontract.ReserveResult](), false, "")
	result.Properties["reservationIds"].Nullable = false
	result.Properties["reasons"].Nullable = false
	result.Properties["decision"].Enum = []any{string(tracercontract.DecisionAllow), string(tracercontract.DecisionDeny), string(tracercontract.DecisionReview)}
	controls := registry.Schema(reflect.TypeFor[tracercontract.ReserveControls](), false, "")
	controls.Properties["rules"].Enum = []any{string(tracercontract.RulesEvaluated), string(tracercontract.RulesNotRequested)}
	controls.Properties["limits"].Enum = []any{string(tracercontract.LimitsEvaluated), string(tracercontract.LimitsSkippedRuleDeny)}
	completion := registry.Schema(reflect.TypeFor[tracercontract.CompletionRequest](), false, "")
	transaction := registry.Schema(reflect.TypeFor[tracercontract.TransactionCompletionResult](), false, "")
	reservation := registry.Schema(reflect.TypeFor[tracercontract.ReservationCompletionResult](), false, "")

	for _, schema := range []*huma.Schema{request, result, completion, transaction, reservation} {
		schema.Properties["contractRevision"].Enum = []any{tracercontract.ReserveContractRevision}
	}

	for _, schema := range []*huma.Schema{transaction, reservation} {
		schema.Properties["status"].Enum = []any{"CONFIRMED", "RELEASED"}
		schema.Properties["evaluationId"].Nullable = false
	}
	// A named reservation always has an immutable owning decision.
	reservation.Required = append(reservation.Required, "evaluationId")
	minimum := float64(0)
	transaction.Properties["flipped"].Minimum = &minimum
}
