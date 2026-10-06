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

// TestAuditEventEnumValidators exercises the custom validator.Func enum checks
// registered for audit-event query parameters. The event-type, action,
// resource-type and actor-type validators accept exactly what the model's
// IsValid accepts. The validators are driven through the shared getValidator()
// instance via v.Var so the registered tags themselves are under test.
func TestAuditEventEnumValidators(t *testing.T) {
	v, err := getValidator()
	require.NoError(t, err)
	require.NotNil(t, v)

	tests := []struct {
		name  string
		tag   string
		value string
		valid bool
	}{
		// auditeventtype
		{"eventtype valid TRANSACTION_VALIDATED", "auditeventtype", string(model.AuditEventTransactionValidated), true},
		{"eventtype valid RULE_DEACTIVATED", "auditeventtype", string(model.AuditEventRuleDeactivated), true},
		{"eventtype valid LIMIT_DEACTIVATED", "auditeventtype", string(model.AuditEventLimitDeactivated), true},
		{"eventtype valid RESERVATION_RESERVED", "auditeventtype", string(model.AuditEventReservationReserved), true},
		{"eventtype valid RESERVATION_CONFIRMED", "auditeventtype", string(model.AuditEventReservationConfirmed), true},
		{"eventtype valid RESERVATION_RELEASED", "auditeventtype", string(model.AuditEventReservationReleased), true},
		{"eventtype valid RESERVATION_EXPIRED", "auditeventtype", string(model.AuditEventReservationExpired), true},
		{"eventtype valid RESERVATION_SKIPPED", "auditeventtype", string(model.AuditEventReservationSkipped), true},
		{"eventtype rejects lowercase variant", "auditeventtype", "reservation_reserved", false},
		{"eventtype rejects garbage", "auditeventtype", "NOT_AN_EVENT", false},

		// auditaction
		{"action valid VALIDATE", "auditaction", string(model.AuditActionValidate), true},
		{"action valid DEACTIVATE", "auditaction", string(model.AuditActionDeactivate), true},
		{"action valid RESERVE", "auditaction", string(model.AuditActionReserve), true},
		{"action valid CONFIRM", "auditaction", string(model.AuditActionConfirm), true},
		{"action valid RELEASE", "auditaction", string(model.AuditActionRelease), true},
		{"action valid EXPIRE", "auditaction", string(model.AuditActionExpire), true},
		{"action valid SKIP", "auditaction", string(model.AuditActionSkip), true},
		{"action rejects lowercase variant", "auditaction", "reserve", false},
		{"action rejects garbage", "auditaction", "FROBNICATE", false},

		// auditresult
		{"result valid SUCCESS", "auditresult", string(model.AuditResultSuccess), true},
		{"result valid REVIEW", "auditresult", string(model.AuditResultReview), true},
		{"result valid FAILED", "auditresult", string(model.AuditResultFailed), true},
		{"result valid ALLOW", "auditresult", string(model.AuditResultAllow), true},
		{"result valid DENY", "auditresult", string(model.AuditResultDeny), true},
		{"result rejects lowercase variant", "auditresult", "review", false},
		{"result rejects garbage", "auditresult", "MAYBE", false},

		// resourcetype
		{"resourcetype valid transaction", "resourcetype", string(model.ResourceTypeTransaction), true},
		{"resourcetype valid rule", "resourcetype", string(model.ResourceTypeRule), true},
		{"resourcetype valid limit", "resourcetype", string(model.ResourceTypeLimit), true},
		{"resourcetype valid reservation", "resourcetype", string(model.ResourceTypeReservation), true},
		{"resourcetype rejects uppercase variant", "resourcetype", "RESERVATION", false},
		{"resourcetype rejects garbage", "resourcetype", "account", false},

		// actortype
		{"actortype valid user", "actortype", string(model.ActorTypeUser), true},
		{"actortype valid system", "actortype", string(model.ActorTypeSystem), true},
		{"actortype valid api_key", "actortype", string(model.ActorTypeAPIKey), true},
		{"actortype rejects garbage", "actortype", "robot", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Var(tt.value, tt.tag)
			if tt.valid {
				assert.NoError(t, err, "%q should be a valid %s", tt.value, tt.tag)
			} else {
				assert.Error(t, err, "%q should be rejected by %s", tt.value, tt.tag)
			}
		})
	}
}

// TestAuditEventEnumValidators_ModelParity checks that every value in the
// shared *UnderTest lists is model-valid and accepted by its validator. The
// model exposes no enumeration of its values, so the lists are hand-maintained:
// a value added to a model enum is covered only once it is added to its list.
func TestAuditEventEnumValidators_ModelParity(t *testing.T) {
	t.Parallel()

	v, err := getValidator()
	require.NoError(t, err)

	for _, et := range auditEventTypesUnderTest() {
		require.True(t, et.IsValid(), "guard: %q must be model-valid", et)
		assert.NoError(t, v.Var(string(et), "auditeventtype"), "%q should pass auditeventtype", et)
	}

	for _, rt := range resourceTypesUnderTest() {
		require.True(t, rt.IsValid(), "guard: %q must be model-valid", rt)
		assert.NoError(t, v.Var(string(rt), "resourcetype"), "%q should pass resourcetype", rt)
	}

	for _, result := range auditResultsUnderTest() {
		require.True(t, result.IsValid(), "guard: %q must be model-valid", result)
		assert.NoError(t, v.Var(string(result), "auditresult"), "%q should pass auditresult", result)
	}

	for _, action := range auditActionsUnderTest() {
		require.True(t, action.IsValid(), "guard: %q must be model-valid", action)
		assert.NoError(t, v.Var(string(action), "auditaction"), "%q should pass auditaction", action)
	}

	for _, actorType := range actorTypesUnderTest() {
		require.True(t, actorType.IsValid(), "guard: %q must be model-valid", actorType)
		assert.NoError(t, v.Var(string(actorType), "actortype"), "%q should pass actortype", actorType)
	}
}

// The *UnderTest helpers list every value of each audit-event enum. They are
// shared by the validator and the published-doc tests and must be updated
// whenever the model enum gains or loses a value.

func auditEventTypesUnderTest() []model.AuditEventType {
	return []model.AuditEventType{
		model.AuditEventTransactionValidated,
		model.AuditEventRuleCreated, model.AuditEventRuleUpdated, model.AuditEventRuleActivated,
		model.AuditEventRuleDeactivated, model.AuditEventRuleDrafted, model.AuditEventRuleDeleted,
		model.AuditEventLimitCreated, model.AuditEventLimitUpdated, model.AuditEventLimitDeleted,
		model.AuditEventLimitActivated, model.AuditEventLimitDeactivated, model.AuditEventLimitDrafted,
		model.AuditEventReservationReserved, model.AuditEventReservationConfirmed,
		model.AuditEventReservationReleased, model.AuditEventReservationExpired,
		model.AuditEventReservationSkipped,
	}
}

func resourceTypesUnderTest() []model.ResourceType {
	return []model.ResourceType{
		model.ResourceTypeTransaction, model.ResourceTypeRule,
		model.ResourceTypeLimit, model.ResourceTypeReservation,
	}
}

func auditResultsUnderTest() []model.AuditResult {
	return []model.AuditResult{
		model.AuditResultSuccess, model.AuditResultFailed,
		model.AuditResultAllow, model.AuditResultDeny, model.AuditResultReview,
	}
}

func auditActionsUnderTest() []model.AuditAction {
	return []model.AuditAction{
		model.AuditActionValidate, model.AuditActionCreate, model.AuditActionUpdate,
		model.AuditActionDelete, model.AuditActionActivate, model.AuditActionDeactivate,
		model.AuditActionDraft, model.AuditActionReserve, model.AuditActionConfirm,
		model.AuditActionRelease, model.AuditActionExpire, model.AuditActionSkip,
	}
}

func actorTypesUnderTest() []model.ActorType {
	return []model.ActorType{model.ActorTypeUser, model.ActorTypeAPIKey, model.ActorTypeSystem}
}

// TestAuditEventEnumValidators_OmitemptyNilPointer verifies the pointer/nil
// short-circuit: when the validated field is a nil pointer, the validator must
// return valid (omitempty semantics) so an absent optional filter never fails.
func TestAuditEventEnumValidators_OmitemptyNilPointer(t *testing.T) {
	v, err := getValidator()
	require.NoError(t, err)

	// A struct with nil optional enum pointers + omitempty must validate clean.
	type filterProbe struct {
		EventType    *model.AuditEventType `validate:"omitempty,auditeventtype"`
		Action       *model.AuditAction    `validate:"omitempty,auditaction"`
		Result       *model.AuditResult    `validate:"omitempty,auditresult"`
		ResourceType *model.ResourceType   `validate:"omitempty,resourcetype"`
		ActorType    *model.ActorType      `validate:"omitempty,actortype"`
	}

	require.NoError(t, v.Struct(filterProbe{}), "all-nil optional enums must validate clean")

	// A populated-but-invalid pointer must still be rejected (proves the nil
	// short-circuit is not masking the value path).
	bad := model.AuditAction("FROBNICATE")
	require.Error(t, v.Struct(filterProbe{Action: &bad}))
}

// TestRuleAndTransactionTypeValidators exercises the rulestatus and
// transactiontype enum validators registered for rule query parameters.
func TestRuleAndTransactionTypeValidators(t *testing.T) {
	v, err := getValidator()
	require.NoError(t, err)

	t.Run("rulestatus accepts every model-valid status", func(t *testing.T) {
		for _, s := range []model.RuleStatus{
			model.RuleStatusDraft, model.RuleStatusActive,
			model.RuleStatusInactive, model.RuleStatusDeleted,
		} {
			require.True(t, s.IsValid(), "guard: %q must be model-valid", s)
			assert.NoError(t, v.Var(string(s), "rulestatus"), "%q should pass rulestatus", s)
		}
	})

	t.Run("rulestatus rejects unknown status", func(t *testing.T) {
		assert.Error(t, v.Var("ARCHIVED", "rulestatus"))
	})

	t.Run("transactiontype accepts a model-valid type", func(t *testing.T) {
		require.True(t, model.TransactionTypeCard.Valid())
		assert.NoError(t, v.Var(string(model.TransactionTypeCard), "transactiontype"))
	})

	t.Run("transactiontype rejects unknown type", func(t *testing.T) {
		assert.Error(t, v.Var("CARRIER_PIGEON", "transactiontype"))
	})
}
