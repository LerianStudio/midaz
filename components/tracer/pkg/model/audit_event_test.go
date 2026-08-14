// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuditEvent(t *testing.T) {
	t.Run("Success - creates audit event with all required fields", func(t *testing.T) {
		actor := Actor{
			ActorType: ActorTypeSystem,
			ID:        "svc_tracer",
			Name:      "Tracer Service",
			Role:      "system",
			IPAddress: "10.0.1.5",
		}

		resourceID := testutil.MustDeterministicUUID(50).String()

		event, err := NewAuditEvent(
			AuditEventRuleCreated,
			AuditActionCreate,
			AuditResultSuccess,
			resourceID,
			ResourceTypeRule,
			actor,
		)

		require.NoError(t, err)
		require.NotNil(t, event)
		assert.NotEqual(t, uuid.Nil, event.EventID)
		assert.Equal(t, AuditEventRuleCreated, event.EventType)
		assert.Equal(t, AuditActionCreate, event.Action)
		assert.Equal(t, AuditResultSuccess, event.Result)
		assert.Equal(t, resourceID, event.ResourceID)
		assert.Equal(t, ResourceTypeRule, event.ResourceType)
		assert.Equal(t, actor, event.Actor)
		assert.False(t, event.CreatedAt.IsZero())
		assert.NotNil(t, event.Context)
		assert.NotNil(t, event.Metadata)
		assert.Empty(t, event.Context)
		assert.Empty(t, event.Metadata)
	})
}

func TestAuditEvent_WithContext(t *testing.T) {
	t.Run("Success - sets context data", func(t *testing.T) {
		event := createTestAuditEvent(t)
		contextData := map[string]any{
			"before": map[string]any{"status": "INACTIVE"},
			"after":  map[string]any{"status": "ACTIVE"},
			"reason": "Activated for production",
		}

		result := event.WithContext(contextData)

		assert.Equal(t, event, result)
		assert.Equal(t, contextData, event.Context)
	})
}

func TestAuditEvent_WithValidationContext(t *testing.T) {
	t.Run("Success - sets validation context with request and response", func(t *testing.T) {
		event := createTestAuditEvent(t)

		request := map[string]any{
			"transactionType": "PIX",
			"amount":          150000,
			"currency":        "BRL",
		}

		matchedRuleID := testutil.MustDeterministicUUID(1)
		evaluatedRuleID := testutil.MustDeterministicUUID(2)

		evalResult := EvaluationResult{
			Decision:         DecisionAllow,
			Reason:           "All rules passed",
			MatchedRuleIDs:   []uuid.UUID{matchedRuleID},
			EvaluatedRuleIDs: []uuid.UUID{evaluatedRuleID},
			TotalRulesLoaded: 10,
			Truncated:        false,
		}

		responseContext := ValidationResponseContext{
			ProcessingTimeMs:  45,
			LimitUsageDetails: []LimitUsageDetail{},
		}

		result := event.WithValidationContext(request, evalResult, responseContext)

		assert.Equal(t, event, result)
		assert.NotNil(t, event.Context["request"])
		assert.NotNil(t, event.Context["response"])

		responseData, ok := event.Context["response"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, string(DecisionAllow), responseData["decision"])
		assert.Equal(t, "All rules passed", responseData["reason"])
		assert.Equal(t, float64(45), responseData["processingTimeMs"])
	})
}

func TestAuditEvent_WithCRUDContext(t *testing.T) {
	t.Run("Success - sets CRUD context with before/after", func(t *testing.T) {
		event := createTestAuditEvent(t)

		before := map[string]any{"status": "INACTIVE", "threshold": 0.7}
		after := map[string]any{"status": "ACTIVE", "threshold": 0.7}
		reason := "Activated for production use"

		result := event.WithCRUDContext(before, after, reason)

		assert.Equal(t, event, result)
		assert.Equal(t, before, event.Context["before"])
		assert.Equal(t, after, event.Context["after"])
		assert.Equal(t, reason, event.Context["reason"])
	})

	t.Run("Success - handles nil before", func(t *testing.T) {
		event := createTestAuditEvent(t)

		after := map[string]any{"status": "ACTIVE"}
		result := event.WithCRUDContext(nil, after, "Created")

		assert.Equal(t, event, result)
		assert.Nil(t, event.Context["before"])
		assert.Equal(t, after, event.Context["after"])
	})

	t.Run("Success - empty reason not added to context", func(t *testing.T) {
		event := createTestAuditEvent(t)

		before := map[string]any{"status": "INACTIVE"}
		after := map[string]any{"status": "ACTIVE"}

		result := event.WithCRUDContext(before, after, "")

		assert.Equal(t, event, result)
		assert.Equal(t, before, event.Context["before"])
		assert.Equal(t, after, event.Context["after"])
		assert.Nil(t, event.Context["reason"])
	})
}

func TestAuditEvent_WithMetadata(t *testing.T) {
	t.Run("Success - sets metadata", func(t *testing.T) {
		event := createTestAuditEvent(t)

		metadata := map[string]any{
			"ticketId":      "JIRA-123",
			"correlationId": testutil.MustDeterministicUUID(51).String(),
		}

		result := event.WithMetadata(metadata)

		assert.Equal(t, event, result)
		assert.Equal(t, metadata, event.Metadata)
	})
}

func TestDecisionToAuditResult(t *testing.T) {
	tests := []struct {
		name     string
		decision Decision
		expected AuditResult
	}{
		{
			name:     "ALLOW maps to AuditResultAllow",
			decision: DecisionAllow,
			expected: AuditResultAllow,
		},
		{
			name:     "DENY maps to AuditResultDeny",
			decision: DecisionDeny,
			expected: AuditResultDeny,
		},
		{
			name:     "REVIEW maps to AuditResultReview",
			decision: DecisionReview,
			expected: AuditResultReview,
		},
		{
			name:     "Invalid decision maps to FAILED with warning",
			decision: Decision("INVALID"),
			expected: AuditResultFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecisionToAuditResult(tt.decision)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAuditEvent_GetValidationRequest(t *testing.T) {
	t.Run("Success - extracts request from context", func(t *testing.T) {
		event := createTestAuditEvent(t)
		request := map[string]any{
			"transactionType": "PIX",
			"amount":          100,
		}

		event.Context = map[string]any{"request": request}

		result := event.GetValidationRequest()

		assert.Equal(t, request, result)
	})

	t.Run("Returns nil when request not present", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{"other": "data"}

		result := event.GetValidationRequest()

		assert.Nil(t, result)
	})

	t.Run("Returns nil when request is not a map", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{"request": "not a map"}

		result := event.GetValidationRequest()

		assert.Nil(t, result)
	})
}

func TestAuditEvent_GetValidationResponse(t *testing.T) {
	t.Run("Success - extracts response from context", func(t *testing.T) {
		event := createTestAuditEvent(t)
		response := map[string]any{
			"decision": "ALLOW",
			"reason":   "All rules passed",
		}

		event.Context = map[string]any{"response": response}

		result := event.GetValidationResponse()

		assert.Equal(t, response, result)
	})

	t.Run("Returns nil when response not present", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{"other": "data"}

		result := event.GetValidationResponse()

		assert.Nil(t, result)
	})
}

func TestAuditEvent_GetReason(t *testing.T) {
	t.Run("Success - extracts reason from validation response", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{
			"response": map[string]any{
				"reason": "All rules passed",
			},
		}

		reason := event.GetReason()

		assert.Equal(t, "All rules passed", reason)
	})

	t.Run("Success - extracts reason from CRUD context", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{
			"reason": "Activated for production",
		}

		reason := event.GetReason()

		assert.Equal(t, "Activated for production", reason)
	})

	t.Run("Returns empty string when reason not found", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{"other": "data"}

		reason := event.GetReason()

		assert.Equal(t, "", reason)
	})
}

func TestAuditEvent_GetProcessingTimeMs(t *testing.T) {
	t.Run("Success - extracts processing time from response (int64)", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{
			"response": map[string]any{
				"processingTimeMs": int64(45),
			},
		}

		result := event.GetProcessingTimeMs()

		assert.Equal(t, float64(45), result)
	})

	t.Run("Success - extracts processing time from response (float64)", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{
			"response": map[string]any{
				"processingTimeMs": float64(45),
			},
		}

		result := event.GetProcessingTimeMs()

		assert.Equal(t, float64(45), result)
	})

	t.Run("Returns 0 when processing time not found", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{"other": "data"}

		result := event.GetProcessingTimeMs()

		assert.Equal(t, float64(0), result)
	})
}

func TestAuditEvent_GetMatchedRuleIDs(t *testing.T) {
	t.Run("Success - extracts matched rule IDs from response", func(t *testing.T) {
		event := createTestAuditEvent(t)
		ruleID1 := testutil.MustDeterministicUUID(10)
		ruleID2 := testutil.MustDeterministicUUID(11)

		event.Context = map[string]any{
			"response": map[string]any{
				"matchedRuleIds": []any{ruleID1.String(), ruleID2.String()},
			},
		}

		result := event.GetMatchedRuleIDs()

		require.Len(t, result, 2)
		assert.Equal(t, ruleID1, result[0])
		assert.Equal(t, ruleID2, result[1])
	})

	t.Run("Returns nil when matched rule IDs not found", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{"other": "data"}

		result := event.GetMatchedRuleIDs()

		assert.Nil(t, result)
	})

	t.Run("Skips invalid UUID strings", func(t *testing.T) {
		event := createTestAuditEvent(t)
		validID := testutil.MustDeterministicUUID(12)

		event.Context = map[string]any{
			"response": map[string]any{
				"matchedRuleIds": []any{validID.String(), "invalid-uuid", "not-a-uuid"},
			},
		}

		result := event.GetMatchedRuleIDs()

		require.Len(t, result, 1)
		assert.Equal(t, validID, result[0])
	})
}

func TestAuditEvent_GetEvaluatedRuleIDs(t *testing.T) {
	t.Run("Success - extracts evaluated rule IDs from response", func(t *testing.T) {
		event := createTestAuditEvent(t)
		ruleID1 := testutil.MustDeterministicUUID(20)
		ruleID2 := testutil.MustDeterministicUUID(21)
		ruleID3 := testutil.MustDeterministicUUID(22)

		event.Context = map[string]any{
			"response": map[string]any{
				"evaluatedRuleIds": []any{ruleID1.String(), ruleID2.String(), ruleID3.String()},
			},
		}

		result := event.GetEvaluatedRuleIDs()

		require.Len(t, result, 3)
		assert.Equal(t, ruleID1, result[0])
		assert.Equal(t, ruleID2, result[1])
		assert.Equal(t, ruleID3, result[2])
	})

	t.Run("Returns nil when evaluated rule IDs not found", func(t *testing.T) {
		event := createTestAuditEvent(t)
		event.Context = map[string]any{"other": "data"}

		result := event.GetEvaluatedRuleIDs()

		assert.Nil(t, result)
	})
}

func TestActorType_Values(t *testing.T) {
	t.Run("ActorType constants are correctly defined", func(t *testing.T) {
		assert.Equal(t, ActorType("user"), ActorTypeUser)
		assert.Equal(t, ActorType("system"), ActorTypeSystem)
	})
}

func TestAuditEventType_Values(t *testing.T) {
	t.Run("AuditEventType constants are correctly defined", func(t *testing.T) {
		assert.Equal(t, AuditEventType("TRANSACTION_VALIDATED"), AuditEventTransactionValidated)
		assert.Equal(t, AuditEventType("RULE_CREATED"), AuditEventRuleCreated)
		assert.Equal(t, AuditEventType("RULE_UPDATED"), AuditEventRuleUpdated)
		assert.Equal(t, AuditEventType("RULE_ACTIVATED"), AuditEventRuleActivated)
		assert.Equal(t, AuditEventType("RULE_DEACTIVATED"), AuditEventRuleDeactivated)
		assert.Equal(t, AuditEventType("RULE_DELETED"), AuditEventRuleDeleted)
		assert.Equal(t, AuditEventType("LIMIT_CREATED"), AuditEventLimitCreated)
		assert.Equal(t, AuditEventType("LIMIT_UPDATED"), AuditEventLimitUpdated)
		assert.Equal(t, AuditEventType("LIMIT_DELETED"), AuditEventLimitDeleted)
		assert.Equal(t, AuditEventType("LIMIT_ACTIVATED"), AuditEventLimitActivated)
		assert.Equal(t, AuditEventType("LIMIT_DEACTIVATED"), AuditEventLimitDeactivated)
	})
}

func TestAuditAction_Values(t *testing.T) {
	t.Run("AuditAction constants are correctly defined", func(t *testing.T) {
		assert.Equal(t, AuditAction("VALIDATE"), AuditActionValidate)
		assert.Equal(t, AuditAction("CREATE"), AuditActionCreate)
		assert.Equal(t, AuditAction("UPDATE"), AuditActionUpdate)
		assert.Equal(t, AuditAction("DELETE"), AuditActionDelete)
		assert.Equal(t, AuditAction("ACTIVATE"), AuditActionActivate)
		assert.Equal(t, AuditAction("DEACTIVATE"), AuditActionDeactivate)
	})
}

func TestAuditResult_Values(t *testing.T) {
	t.Run("AuditResult constants are correctly defined", func(t *testing.T) {
		assert.Equal(t, AuditResult("SUCCESS"), AuditResultSuccess)
		assert.Equal(t, AuditResult("FAILED"), AuditResultFailed)
		assert.Equal(t, AuditResult("ALLOW"), AuditResultAllow)
		assert.Equal(t, AuditResult("DENY"), AuditResultDeny)
		assert.Equal(t, AuditResult("REVIEW"), AuditResultReview)
	})
}

func TestResourceType_Values(t *testing.T) {
	t.Run("ResourceType constants are correctly defined", func(t *testing.T) {
		assert.Equal(t, ResourceType("transaction"), ResourceTypeTransaction)
		assert.Equal(t, ResourceType("rule"), ResourceTypeRule)
		assert.Equal(t, ResourceType("limit"), ResourceTypeLimit)
	})
}

// createTestAuditEvent is a test helper that creates a minimal valid AuditEvent.
func createTestAuditEvent(t *testing.T) *AuditEvent {
	t.Helper()

	event, err := NewAuditEvent(
		AuditEventRuleCreated,
		AuditActionCreate,
		AuditResultSuccess,
		testutil.MustDeterministicUUID(100).String(),
		ResourceTypeRule,
		Actor{
			ActorType: ActorTypeSystem,
			ID:        "test",
			Name:      "Test Actor",
			IPAddress: "127.0.0.1",
		},
	)
	require.NoError(t, err)

	return event
}
