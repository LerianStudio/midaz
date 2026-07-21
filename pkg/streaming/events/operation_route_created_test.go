// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	operationRouteID  = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	operationRouteOrg = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")
	operationRouteLed = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0c")
)

func minimalOperationRoute() *mmodel.OperationRoute {
	return &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: operationRouteOrg,
		LedgerID:       operationRouteLed,
		Title:          "Cashin from service charge",
		OperationType:  "source",
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

func operationRouteWithAccountRule() *mmodel.OperationRoute {
	o := minimalOperationRoute()
	o.Account = &mmodel.AccountRule{
		RuleType: "alias",
		ValidIf:  "@cash_account",
	}

	return o
}

func operationRouteWithAccountingEntries() *mmodel.OperationRoute {
	o := minimalOperationRoute()
	o.AccountingEntries = &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit: &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
		},
	}

	return o
}

func TestOperationRouteCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "operation-route.created", events.OperationRouteCreatedDefinition.Key())
	assert.Equal(t, "operation-route", events.OperationRouteCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.OperationRouteCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.OperationRouteCreatedDefinition.SchemaVersion)
}

func TestNewOperationRouteCreated_MapsMinimalOperationRoute(t *testing.T) {
	o := minimalOperationRoute()

	payload := events.NewOperationRouteCreated(o)

	assert.Equal(t, operationRouteID.String(), payload.ID)
	assert.Equal(t, operationRouteOrg.String(), payload.OrganizationID)
	assert.Equal(t, operationRouteLed.String(), payload.LedgerID)
	assert.Equal(t, "Cashin from service charge", payload.Title)
	assert.Empty(t, payload.Description)
	assert.Empty(t, payload.Code)
	assert.Equal(t, "source", payload.OperationType)
	assert.Nil(t, payload.Account)
	assert.Nil(t, payload.AccountingEntries)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewOperationRouteCreated_MapsOptionalScalars(t *testing.T) {
	o := minimalOperationRoute()
	o.Description = "Long-form description"
	o.Code = "EXT-001"

	payload := events.NewOperationRouteCreated(o)

	assert.Equal(t, "Long-form description", payload.Description)
	assert.Equal(t, "EXT-001", payload.Code)
}

func TestNewOperationRouteCreated_MapsAccountRule(t *testing.T) {
	o := operationRouteWithAccountRule()

	payload := events.NewOperationRouteCreated(o)

	require.NotNil(t, payload.Account)
	assert.Equal(t, "alias", payload.Account.RuleType)
	assert.Equal(t, "@cash_account", payload.Account.ValidIf)
}

func TestNewOperationRouteCreated_MapsAccountingEntries(t *testing.T) {
	o := operationRouteWithAccountingEntries()

	payload := events.NewOperationRouteCreated(o)

	require.NotNil(t, payload.AccountingEntries)
	require.NotNil(t, payload.AccountingEntries.Direct)
	require.NotNil(t, payload.AccountingEntries.Direct.Debit)
	assert.Equal(t, "1001", payload.AccountingEntries.Direct.Debit.Code)
}

func TestOperationRouteCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewOperationRouteCreated(minimalOperationRoute())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.OperationRouteCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.OperationRouteCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestOperationRouteCreatedPayload_JSONShape_MinimalOmitsOptionals(t *testing.T) {
	payload := events.NewOperationRouteCreated(minimalOperationRoute())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "title", "operationType", "createdAt", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, key := range []string{"description", "code", "account", "accountingEntries"} {
		_, has := generic[key]
		assert.Falsef(t, has, "%q must omitempty when zero/nil", key)
	}

	assert.Lenf(t, generic, 7, "expected 7 top-level fields when optionals omitted, got %d (drift?)", len(generic))
}

func TestOperationRouteCreatedPayload_JSONShape_WithAllFields(t *testing.T) {
	o := minimalOperationRoute()
	o.Description = "desc"
	o.Code = "EXT-001"
	o.Account = &mmodel.AccountRule{RuleType: "alias", ValidIf: "@cash"}
	o.AccountingEntries = &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit: &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
		},
	}

	payload := events.NewOperationRouteCreated(o)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	assert.Lenf(t, generic, 11, "expected 11 top-level fields with all optionals present, got %d (drift?)", len(generic))
}
