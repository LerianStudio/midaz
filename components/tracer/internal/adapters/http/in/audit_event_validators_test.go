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
// registered for audit-event query parameters. Each validator accepts only a
// specific allow-list of enum values; values outside the list (including model
// constants that belong to a different surface, e.g. RESERVE actions) MUST be
// rejected. The validators are driven through the shared getValidator()
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
		{"eventtype rejects reservation type", "auditeventtype", string(model.AuditEventReservationConfirmed), false},
		{"eventtype rejects garbage", "auditeventtype", "NOT_AN_EVENT", false},

		// auditaction
		{"action valid VALIDATE", "auditaction", string(model.AuditActionValidate), true},
		{"action valid DEACTIVATE", "auditaction", string(model.AuditActionDeactivate), true},
		{"action rejects RESERVE (reservation surface)", "auditaction", string(model.AuditActionReserve), false},
		{"action rejects CONFIRM (reservation surface)", "auditaction", string(model.AuditActionConfirm), false},
		{"action rejects garbage", "auditaction", "FROBNICATE", false},

		// auditresult
		{"result valid SUCCESS", "auditresult", string(model.AuditResultSuccess), true},
		{"result valid REVIEW", "auditresult", string(model.AuditResultReview), true},
		{"result rejects garbage", "auditresult", "MAYBE", false},

		// resourcetype
		{"resourcetype valid transaction", "resourcetype", string(model.ResourceTypeTransaction), true},
		{"resourcetype valid rule", "resourcetype", string(model.ResourceTypeRule), true},
		{"resourcetype valid limit", "resourcetype", string(model.ResourceTypeLimit), true},
		{"resourcetype rejects reservation", "resourcetype", string(model.ResourceTypeReservation), false},
		{"resourcetype rejects garbage", "resourcetype", "account", false},

		// actortype
		{"actortype valid user", "actortype", string(model.ActorTypeUser), true},
		{"actortype valid system", "actortype", string(model.ActorTypeSystem), true},
		{"actortype rejects api_key (not in audit allow-list)", "actortype", string(model.ActorTypeAPIKey), false},
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
		require.True(t, model.TransactionTypeCard.IsValid())
		assert.NoError(t, v.Var(string(model.TransactionTypeCard), "transactiontype"))
	})

	t.Run("transactiontype rejects unknown type", func(t *testing.T) {
		assert.Error(t, v.Var("CARRIER_PIGEON", "transactiontype"))
	})
}
