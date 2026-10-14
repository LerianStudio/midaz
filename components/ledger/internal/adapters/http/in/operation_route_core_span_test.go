// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const createOperationRouteSpanName = "handler.create_operation_route"

// removedOperationRouteValidationSpans are the validation steps that no longer open a span.
var removedOperationRouteValidationSpans = []string{
	"handler.validate_account_rule",
	"handler.validate_accounting_entries",
	"handler.validate_accounting_rules_matrix",
	"handler.validate_direction_scenario_matrix",
	"handler.validate_reserve_group_atomicity",
	"handler.validate_direct_mandatory",
	"handler.validate_cross_ledger_entry",
	"handler.validate_entry_field_requirements",
}

// Every validation step passes under the handler span without opening a span of its own.
// The request is then refused by the unknown-key probe, before any service call.
func TestOperationRouteSpans_PassingValidationsOpenNoSpan(t *testing.T) {
	t.Parallel()

	payload := &mmodel.CreateOperationRouteInput{
		OperationType: constant.OperationRouteTypeSource,
		AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
				Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
			},
		},
	}
	rawBody := []byte(`{"accountingEntries":{"direct":{"debit":{"code":"1001","description":"Debit"},"credit":{"code":"2001","description":"Credit"}},"foobar":{}}}`)

	ctx, recorder := spantest.RecordingContext()

	_, err := (&OperationRouteHandler{}).createOperationRoute(ctx, uuid.New(), nil, payload, rawBody)
	require.Error(t, err, "the unknown accountingEntries key must still be refused")

	spantest.AssertNoSpan(t, recorder, removedOperationRouteValidationSpans...)
	assert.Equal(t, []string{createOperationRouteSpanName}, spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, createOperationRouteSpanName)
	assert.Empty(t, span.Events(), "passing validations record no event")
}

// A failing validation step records its event on the handler span, keeps the status unset
// (business error) and leaves the handler's payload attributes untouched.
func TestOperationRouteSpans_FailingValidationIsAnEventOnTheHandlerSpan(t *testing.T) {
	t.Parallel()

	payload := &mmodel.CreateOperationRouteInput{
		OperationType: constant.OperationRouteTypeSource,
		Account:       &mmodel.AccountRule{RuleType: constant.AccountRuleTypeAlias},
	}

	ctx, recorder := spantest.RecordingContext()

	_, err := (&OperationRouteHandler{}).createOperationRoute(ctx, uuid.New(), nil, payload, nil)
	require.Error(t, err)

	spantest.AssertNoSpan(t, recorder, removedOperationRouteValidationSpans...)

	span := spantest.FindSpan(t, recorder, createOperationRouteSpanName)
	event, ok := spantest.FindEvent(span, "handler.validate_account_rule")
	require.True(t, ok, "the failing step must record an event named after the step on the handler span")
	assert.Contains(t, spantest.EventText(event), constant.ErrMissingFieldsInRequest.Error(),
		"the failure detail rides the event's error attribute")
	assert.Equal(t, codes.Unset, span.Status().Code, "a business rejection keeps the span status unset")
	assert.Contains(t, span.Attributes(), attribute.String("app.request.payload.type", "CreateOperationRouteInput"),
		"the step must not overwrite the handler's payload attributes with its sub-struct")
}

// On the PATCH path the handler records the payload attributes only after its validators
// run, so a failing validator leaves the step-named event on the handler span and no
// payload attribute at all: the step must not write its sub-struct attributes there.
func TestOperationRouteSpans_UpdateFailingValidationIsAnEventOnTheHandlerSpan(t *testing.T) {
	t.Parallel()

	payload := &mmodel.UpdateOperationRouteInput{
		Account: &mmodel.AccountRule{RuleType: constant.AccountRuleTypeAlias},
	}

	ctx, recorder := spantest.RecordingContext()

	_, err := (&OperationRouteHandler{}).updateOperationRoute(ctx, uuid.New(), uuid.New(), payload, nil)
	require.Error(t, err)

	spantest.AssertNoSpan(t, recorder, removedOperationRouteValidationSpans...)
	assert.Equal(t, []string{"handler.update_operation_route"}, spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, "handler.update_operation_route")
	event, ok := spantest.FindEvent(span, "handler.validate_account_rule")
	require.True(t, ok, "the failing step must record an event named after the step on the handler span")
	assert.Contains(t, spantest.EventText(event), constant.ErrMissingFieldsInRequest.Error())
	assert.Equal(t, codes.Unset, span.Status().Code, "a business rejection keeps the span status unset")

	for _, kv := range span.Attributes() {
		assert.NotContains(t, string(kv.Key), "app.request.payload.",
			"no payload attribute is recorded before the validators pass, and the step must not record its sub-struct")
	}
}
