// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
	"tracer/pkg/constant"
)

func TestNewAuditEvent_Validation(t *testing.T) {
	t.Parallel()

	validActor := Actor{
		ActorType: ActorTypeSystem,
		ID:        "test_actor",
		Name:      "Test Actor",
	}

	t.Run("validation errors", func(t *testing.T) {
		testCases := []struct {
			name         string
			eventType    AuditEventType
			action       AuditAction
			result       AuditResult
			resourceID   string
			resourceType ResourceType
			actor        Actor
			expectedErr  error
		}{
			{
				name:         "invalid event type",
				eventType:    AuditEventType("INVALID"),
				action:       AuditActionCreate,
				result:       AuditResultSuccess,
				resourceID:   testutil.MustDeterministicUUID(1).String(),
				resourceType: ResourceTypeRule,
				actor:        validActor,
				expectedErr:  constant.ErrAuditEventInvalidType,
			},
			{
				name:         "invalid action",
				eventType:    AuditEventRuleCreated,
				action:       AuditAction("INVALID"),
				result:       AuditResultSuccess,
				resourceID:   testutil.MustDeterministicUUID(2).String(),
				resourceType: ResourceTypeRule,
				actor:        validActor,
				expectedErr:  constant.ErrAuditEventInvalidAction,
			},
			{
				name:         "invalid result",
				eventType:    AuditEventRuleCreated,
				action:       AuditActionCreate,
				result:       AuditResult("INVALID"),
				resourceID:   testutil.MustDeterministicUUID(3).String(),
				resourceType: ResourceTypeRule,
				actor:        validActor,
				expectedErr:  constant.ErrAuditEventInvalidResult,
			},
			{
				name:         "empty resource ID",
				eventType:    AuditEventRuleCreated,
				action:       AuditActionCreate,
				result:       AuditResultSuccess,
				resourceID:   "",
				resourceType: ResourceTypeRule,
				actor:        validActor,
				expectedErr:  constant.ErrAuditEventResourceIDRequired,
			},
			{
				name:         "invalid resource type",
				eventType:    AuditEventRuleCreated,
				action:       AuditActionCreate,
				result:       AuditResultSuccess,
				resourceID:   testutil.MustDeterministicUUID(4).String(),
				resourceType: ResourceType("INVALID"),
				actor:        validActor,
				expectedErr:  constant.ErrAuditEventInvalidResourceType,
			},
			{
				name:         "empty actor ID",
				eventType:    AuditEventRuleCreated,
				action:       AuditActionCreate,
				result:       AuditResultSuccess,
				resourceID:   testutil.MustDeterministicUUID(5).String(),
				resourceType: ResourceTypeRule,
				actor: Actor{
					ActorType: ActorTypeUser,
					ID:        "",
					Name:      "Test Actor",
				},
				expectedErr: constant.ErrAuditEventActorIDRequired,
			},
			{
				name:         "invalid actor type",
				eventType:    AuditEventRuleCreated,
				action:       AuditActionCreate,
				result:       AuditResultSuccess,
				resourceID:   testutil.MustDeterministicUUID(6).String(),
				resourceType: ResourceTypeRule,
				actor: Actor{
					ActorType: ActorType("INVALID"),
					ID:        "test_actor",
					Name:      "Test Actor",
				},
				expectedErr: constant.ErrAuditEventActorTypeInvalid,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				event, err := NewAuditEvent(
					tc.eventType,
					tc.action,
					tc.result,
					tc.resourceID,
					tc.resourceType,
					tc.actor,
				)

				require.Error(t, err)
				assert.Nil(t, event)
				assert.ErrorIs(t, err, tc.expectedErr)
			})
		}
	})

	t.Run("Success - all valid enums", func(t *testing.T) {
		testCases := []struct {
			name         string
			eventType    AuditEventType
			action       AuditAction
			result       AuditResult
			resourceType ResourceType
			resourceID   string
		}{
			{
				name:         "Rule created",
				eventType:    AuditEventRuleCreated,
				action:       AuditActionCreate,
				result:       AuditResultSuccess,
				resourceType: ResourceTypeRule,
				resourceID:   testutil.MustDeterministicUUID(10).String(),
			},
			{
				name:         "Limit activated",
				eventType:    AuditEventLimitActivated,
				action:       AuditActionActivate,
				result:       AuditResultSuccess,
				resourceType: ResourceTypeLimit,
				resourceID:   testutil.MustDeterministicUUID(11).String(),
			},
			{
				name:         "Transaction validated",
				eventType:    AuditEventTransactionValidated,
				action:       AuditActionValidate,
				result:       AuditResultAllow,
				resourceType: ResourceTypeTransaction,
				resourceID:   testutil.MustDeterministicUUID(12).String(),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				event, err := NewAuditEvent(
					tc.eventType,
					tc.action,
					tc.result,
					tc.resourceID,
					tc.resourceType,
					validActor,
				)

				require.NoError(t, err)
				require.NotNil(t, event)

				// Assert generated fields are populated
				assert.NotEmpty(t, event.EventID, "EventID should be generated")
				assert.False(t, event.CreatedAt.IsZero(), "CreatedAt should be set")

				// Assert input fields are preserved
				assert.Equal(t, tc.eventType, event.EventType)
				assert.Equal(t, tc.action, event.Action)
				assert.Equal(t, tc.result, event.Result)
				assert.Equal(t, tc.resourceType, event.ResourceType)
				assert.Equal(t, tc.resourceID, event.ResourceID, "ResourceID should be preserved")

				// Assert actor is preserved
				assert.Equal(t, validActor.ActorType, event.Actor.ActorType, "Actor.ActorType should be preserved")
				assert.Equal(t, validActor.ID, event.Actor.ID, "Actor.ID should be preserved")
				assert.Equal(t, validActor.Name, event.Actor.Name, "Actor.Name should be preserved")
			})
		}
	})

	t.Run("Success - context and metadata initialized as empty maps", func(t *testing.T) {
		resourceID20 := testutil.MustDeterministicUUID(20).String()

		event, err := NewAuditEvent(
			AuditEventRuleCreated,
			AuditActionCreate,
			AuditResultSuccess,
			resourceID20,
			ResourceTypeRule,
			validActor,
		)

		require.NoError(t, err)
		require.NotNil(t, event)

		// Assert generated fields are populated
		assert.NotEmpty(t, event.EventID, "EventID should be generated")
		assert.False(t, event.CreatedAt.IsZero(), "CreatedAt should be set")
		assert.NotNil(t, event.Context, "Context should be initialized")
		assert.NotNil(t, event.Metadata, "Metadata should be initialized")
		assert.Empty(t, event.Context, "Context should be empty map")
		assert.Empty(t, event.Metadata, "Metadata should be empty map")
	})

	t.Run("Success - normalizes resourceID with whitespace", func(t *testing.T) {
		testCases := []struct {
			name       string
			resourceID string
			expected   string
		}{
			{
				name:       "leading spaces",
				resourceID: "  resource-123",
				expected:   "resource-123",
			},
			{
				name:       "trailing spaces",
				resourceID: "resource-123  ",
				expected:   "resource-123",
			},
			{
				name:       "leading and trailing spaces",
				resourceID: "  resource-123  ",
				expected:   "resource-123",
			},
			{
				name:       "tabs and newlines",
				resourceID: "\t resource-123 \n",
				expected:   "resource-123",
			},
			{
				name:       "no whitespace",
				resourceID: "resource-123",
				expected:   "resource-123",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				event, err := NewAuditEvent(
					AuditEventRuleCreated,
					AuditActionCreate,
					AuditResultSuccess,
					tc.resourceID,
					ResourceTypeRule,
					validActor,
				)

				require.NoError(t, err)
				require.NotNil(t, event)

				// Assert generated fields are populated
				assert.NotEmpty(t, event.EventID, "EventID should be generated")
				assert.False(t, event.CreatedAt.IsZero(), "CreatedAt should be set")

				assert.Equal(t, tc.expected, event.ResourceID, "ResourceID should be trimmed")
			})
		}
	})

	t.Run("Success - normalizes actor.ID with whitespace", func(t *testing.T) {
		testCases := []struct {
			name     string
			actorID  string
			expected string
		}{
			{
				name:     "leading spaces",
				actorID:  "  actor-123",
				expected: "actor-123",
			},
			{
				name:     "trailing spaces",
				actorID:  "actor-123  ",
				expected: "actor-123",
			},
			{
				name:     "leading and trailing spaces",
				actorID:  "  actor-123  ",
				expected: "actor-123",
			},
			{
				name:     "tabs and newlines",
				actorID:  "\t actor-123 \n",
				expected: "actor-123",
			},
			{
				name:     "no whitespace",
				actorID:  "actor-123",
				expected: "actor-123",
			},
		}

		for i, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resourceID := testutil.MustDeterministicUUID(int64(30 + i)).String()

				actorWithWhitespace := Actor{
					ActorType: ActorTypeSystem,
					ID:        tc.actorID,
					Name:      "Test Actor",
				}

				event, err := NewAuditEvent(
					AuditEventRuleCreated,
					AuditActionCreate,
					AuditResultSuccess,
					resourceID,
					ResourceTypeRule,
					actorWithWhitespace,
				)

				require.NoError(t, err)
				require.NotNil(t, event)

				// Assert generated fields are populated
				assert.NotEmpty(t, event.EventID, "EventID should be generated")
				assert.False(t, event.CreatedAt.IsZero(), "CreatedAt should be set")

				assert.Equal(t, tc.expected, event.Actor.ID, "Actor.ID should be trimmed")
			})
		}
	})

	t.Run("Error - whitespace-only resourceID", func(t *testing.T) {
		event, err := NewAuditEvent(
			AuditEventRuleCreated,
			AuditActionCreate,
			AuditResultSuccess,
			"   ",
			ResourceTypeRule,
			validActor,
		)

		require.Error(t, err)
		assert.Nil(t, event)
		assert.ErrorIs(t, err, constant.ErrAuditEventResourceIDRequired)
	})

	t.Run("Error - whitespace-only actor.ID", func(t *testing.T) {
		resourceID40 := testutil.MustDeterministicUUID(40).String()

		invalidActor := Actor{
			ActorType: ActorTypeUser,
			ID:        "   ",
			Name:      "Test Actor",
		}

		event, err := NewAuditEvent(
			AuditEventRuleCreated,
			AuditActionCreate,
			AuditResultSuccess,
			resourceID40,
			ResourceTypeRule,
			invalidActor,
		)

		require.Error(t, err)
		assert.Nil(t, event)
		assert.ErrorIs(t, err, constant.ErrAuditEventActorIDRequired)
	})
}

func TestAuditEventType_IsValid(t *testing.T) {
	t.Parallel()

	t.Run("Valid event types", func(t *testing.T) {
		t.Parallel()

		validTypes := []AuditEventType{
			AuditEventRuleCreated,
			AuditEventRuleUpdated,
			AuditEventRuleActivated,
			AuditEventRuleDeactivated,
			AuditEventRuleDrafted,
			AuditEventRuleDeleted,
			AuditEventLimitCreated,
			AuditEventLimitUpdated,
			AuditEventLimitActivated,
			AuditEventLimitDeactivated,
			AuditEventLimitDrafted,
			AuditEventLimitDeleted,
			AuditEventTransactionValidated,
		}

		for _, eventType := range validTypes {
			assert.True(t, eventType.IsValid(), "Event type %s should be valid", eventType)
		}
	})

	t.Run("Invalid event types", func(t *testing.T) {
		t.Parallel()

		invalidTypes := []AuditEventType{
			AuditEventType(""),
			AuditEventType("INVALID"),
			AuditEventType("rule_created"),
			AuditEventType("RuleDeleted"),
		}

		for _, eventType := range invalidTypes {
			assert.False(t, eventType.IsValid(), "Event type %s should be invalid", eventType)
		}
	})
}

func TestAuditAction_IsValid(t *testing.T) {
	t.Parallel()

	t.Run("Valid actions", func(t *testing.T) {
		t.Parallel()

		validActions := []AuditAction{
			AuditActionCreate,
			AuditActionUpdate,
			AuditActionDelete,
			AuditActionActivate,
			AuditActionDeactivate,
			AuditActionDraft,
			AuditActionValidate,
		}

		for _, action := range validActions {
			assert.True(t, action.IsValid(), "Action %s should be valid", action)
		}
	})

	t.Run("Invalid actions", func(t *testing.T) {
		t.Parallel()

		invalidActions := []AuditAction{
			AuditAction(""),
			AuditAction("INVALID"),
			AuditAction("create"),
			AuditAction("READ"),
		}

		for _, action := range invalidActions {
			assert.False(t, action.IsValid(), "Action %s should be invalid", action)
		}
	})
}

func TestAuditResult_IsValid(t *testing.T) {
	t.Parallel()

	t.Run("Valid results", func(t *testing.T) {
		t.Parallel()

		validResults := []AuditResult{
			AuditResultSuccess,
			AuditResultFailed,
			AuditResultAllow,
			AuditResultDeny,
			AuditResultReview,
		}

		for _, result := range validResults {
			assert.True(t, result.IsValid(), "Result %s should be valid", result)
		}
	})

	t.Run("Invalid results", func(t *testing.T) {
		t.Parallel()

		invalidResults := []AuditResult{
			AuditResult(""),
			AuditResult("INVALID"),
			AuditResult("success"),
			AuditResult("PENDING"),
		}

		for _, result := range invalidResults {
			assert.False(t, result.IsValid(), "Result %s should be invalid", result)
		}
	})
}

func TestResourceType_IsValid(t *testing.T) {
	t.Parallel()

	t.Run("Valid resource types", func(t *testing.T) {
		t.Parallel()

		validTypes := []ResourceType{
			ResourceTypeRule,
			ResourceTypeLimit,
			ResourceTypeTransaction,
		}

		for _, resourceType := range validTypes {
			assert.True(t, resourceType.IsValid(), "Resource type %s should be valid", resourceType)
		}
	})

	t.Run("Invalid resource types", func(t *testing.T) {
		t.Parallel()

		invalidTypes := []ResourceType{
			ResourceType(""),
			ResourceType("INVALID"),
			ResourceType("RULE"),
			ResourceType("USER"),
		}

		for _, resourceType := range invalidTypes {
			assert.False(t, resourceType.IsValid(), "Resource type %s should be invalid", resourceType)
		}
	})
}
