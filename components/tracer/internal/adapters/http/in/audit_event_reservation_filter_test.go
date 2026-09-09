// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestListAuditEvents_AcceptsEveryValueTheWriterRecords pins that an audit query
// can filter on every value the audit trail actually contains. The reservation
// lifecycle and API-key actors were recorded but not queryable: narrowing a
// query to them came back as a rejected request instead of the records.
func TestListAuditEvents_AcceptsEveryValueTheWriterRecords(t *testing.T) {
	eventTypes := []model.AuditEventType{
		model.AuditEventTransactionValidated,
		model.AuditEventRuleCreated, model.AuditEventRuleUpdated,
		model.AuditEventRuleActivated, model.AuditEventRuleDeactivated,
		model.AuditEventRuleDrafted, model.AuditEventRuleDeleted,
		model.AuditEventLimitCreated, model.AuditEventLimitUpdated,
		model.AuditEventLimitActivated, model.AuditEventLimitDeactivated,
		model.AuditEventLimitDrafted, model.AuditEventLimitDeleted,
		model.AuditEventReservationReserved, model.AuditEventReservationConfirmed,
		model.AuditEventReservationReleased, model.AuditEventReservationExpired,
		model.AuditEventReservationSkipped,
	}

	for _, eventType := range eventTypes {
		t.Run("event_type="+string(eventType), func(t *testing.T) {
			require.True(t, eventType.IsValid(), "the writer records this event type")

			input := &ListAuditEventsInput{EventType: &eventType}
			assert.NoError(t, input.Validate(),
				"an audit query cannot be narrowed to an event type the trail contains")
		})
	}

	actions := []model.AuditAction{
		model.AuditActionValidate, model.AuditActionCreate, model.AuditActionUpdate,
		model.AuditActionDelete, model.AuditActionActivate, model.AuditActionDeactivate,
		model.AuditActionDraft,
		model.AuditActionReserve, model.AuditActionConfirm, model.AuditActionRelease,
		model.AuditActionExpire, model.AuditActionSkip,
	}

	for _, action := range actions {
		t.Run("action="+string(action), func(t *testing.T) {
			require.True(t, action.IsValid(), "the writer records this action")

			input := &ListAuditEventsInput{Action: &action}
			assert.NoError(t, input.Validate(),
				"an audit query cannot be narrowed to an action the trail contains")
		})
	}

	resourceTypes := []model.ResourceType{
		model.ResourceTypeTransaction, model.ResourceTypeRule,
		model.ResourceTypeLimit, model.ResourceTypeReservation,
	}

	for _, resourceType := range resourceTypes {
		t.Run("resource_type="+string(resourceType), func(t *testing.T) {
			require.True(t, resourceType.IsValid(), "the writer records this resource type")

			input := &ListAuditEventsInput{ResourceType: &resourceType}
			assert.NoError(t, input.Validate(),
				"an audit query cannot be narrowed to a resource type the trail contains")
		})
	}

	actorTypes := []model.ActorType{
		model.ActorTypeUser, model.ActorTypeSystem, model.ActorTypeAPIKey,
	}

	for _, actorType := range actorTypes {
		t.Run("actor_type="+string(actorType), func(t *testing.T) {
			require.True(t, actorType.IsValid(), "the writer records this actor type")

			input := &ListAuditEventsInput{ActorType: &actorType}
			assert.NoError(t, input.Validate(),
				"an audit query cannot be narrowed to an actor type the trail contains")
		})
	}
}

// TestListAuditEvents_StillRejectsValuesTheTrailNeverHolds pins that widening
// the filters did not turn them into pass-throughs.
func TestListAuditEvents_StillRejectsValuesTheTrailNeverHolds(t *testing.T) {
	badEventType := model.AuditEventType("NOT_AN_EVENT")
	badAction := model.AuditAction("FROBNICATE")
	badResult := model.AuditResult("MAYBE")
	badResourceType := model.ResourceType("account")
	badActorType := model.ActorType("robot")

	tests := map[string]*ListAuditEventsInput{
		"event_type":    {EventType: &badEventType},
		"action":        {Action: &badAction},
		"result":        {Result: &badResult},
		"resource_type": {ResourceType: &badResourceType},
		"actor_type":    {ActorType: &badActorType},
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, input.Validate(), "an unknown %s filter value was accepted", name)
		})
	}
}
