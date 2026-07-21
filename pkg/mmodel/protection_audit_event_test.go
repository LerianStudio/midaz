// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectionAuditEvent_Constants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "event type provisioning", got: string(AuditEventTypeProvisioning), want: "provisioning"},
		{name: "action provision", got: string(AuditActionProvision), want: "provision"},
		{name: "outcome success", got: string(AuditOutcomeSuccess), want: "success"},
		{name: "outcome failure", got: string(AuditOutcomeFailure), want: "failure"},
		{name: "outcome already exists", got: string(AuditOutcomeAlreadyExists), want: "already_exists"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

func TestProtectionAuditEvent_NewProtectionAuditEvent(t *testing.T) {
	t.Parallel()

	validInput := func() ProtectionAuditEventInput {
		return ProtectionAuditEventInput{
			TenantID:       "tenant-1",
			OrganizationID: "org-1",
			EventType:      AuditEventTypeProvisioning,
			Action:         AuditActionProvision,
			Outcome:        AuditOutcomeSuccess,
			ActorID:        "actor-1",
			ActorType:      "user",
			Reason:         "operator note",
			RequestID:      "req-1",
		}
	}

	tests := []struct {
		name        string
		mutate      func(in *ProtectionAuditEventInput)
		wantErr     error
		assertEvent func(t *testing.T, ev *ProtectionAuditEvent)
	}{
		{
			name:   "valid input returns event",
			mutate: func(in *ProtectionAuditEventInput) {},
			assertEvent: func(t *testing.T, ev *ProtectionAuditEvent) {
				assert.NotEqual(t, uuid.Nil, ev.ID)
				assert.False(t, ev.Timestamp.IsZero())
				assert.Equal(t, "UTC", ev.Timestamp.Location().String())
				assert.Equal(t, "tenant-1", ev.TenantID)
				assert.Equal(t, "org-1", ev.OrganizationID)
				assert.Equal(t, AuditEventTypeProvisioning, ev.EventType)
				assert.Equal(t, AuditActionProvision, ev.Action)
				assert.Equal(t, AuditOutcomeSuccess, ev.Outcome)
				assert.Equal(t, "user", ev.ActorType)
				assert.Nil(t, ev.Details)
			},
		},
		{
			name: "nil details allowed",
			mutate: func(in *ProtectionAuditEventInput) {
				in.Details = nil
			},
			assertEvent: func(t *testing.T, ev *ProtectionAuditEvent) {
				assert.Nil(t, ev.Details)
			},
		},
		{
			name: "details preserved",
			mutate: func(in *ProtectionAuditEventInput) {
				in.Details = &AuditDetails{
					PreviousStatus:    "legacy",
					NewStatus:         "envelope",
					AffectedKeyIDs:    []uint32{1, 2},
					ProviderReference: "transit/org-1",
					ErrorCode:         "",
				}
			},
			assertEvent: func(t *testing.T, ev *ProtectionAuditEvent) {
				require.NotNil(t, ev.Details)
				assert.Equal(t, []uint32{1, 2}, ev.Details.AffectedKeyIDs)
				assert.Equal(t, "transit/org-1", ev.Details.ProviderReference)
			},
		},
		{
			name: "empty actor type defaults to service",
			mutate: func(in *ProtectionAuditEventInput) {
				in.ActorType = ""
			},
			assertEvent: func(t *testing.T, ev *ProtectionAuditEvent) {
				assert.Equal(t, defaultActorType, ev.ActorType)
			},
		},
		{
			name: "empty event type returns error",
			mutate: func(in *ProtectionAuditEventInput) {
				in.EventType = ""
			},
			wantErr: constant.ErrAuditEventRequired,
		},
		{
			name: "whitespace-only event type returns error",
			mutate: func(in *ProtectionAuditEventInput) {
				in.EventType = "   "
			},
			wantErr: constant.ErrAuditEventRequired,
		},
		{
			name: "empty action returns error",
			mutate: func(in *ProtectionAuditEventInput) {
				in.Action = ""
			},
			wantErr: constant.ErrAuditEventRequired,
		},
		{
			name: "whitespace-only action returns error",
			mutate: func(in *ProtectionAuditEventInput) {
				in.Action = "   "
			},
			wantErr: constant.ErrAuditEventRequired,
		},
		{
			name: "empty organization id returns error",
			mutate: func(in *ProtectionAuditEventInput) {
				in.OrganizationID = ""
			},
			wantErr: constant.ErrAuditEventRequired,
		},
		{
			name: "whitespace-only organization id returns error",
			mutate: func(in *ProtectionAuditEventInput) {
				in.OrganizationID = "   "
			},
			wantErr: constant.ErrAuditEventRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validInput()
			tt.mutate(&in)

			ev, err := NewProtectionAuditEvent(in)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, ev)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, ev)
			tt.assertEvent(t, ev)
		})
	}
}

func TestProtectionAuditEvent_SafeLogFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(in *ProtectionAuditEventInput)
		asserts func(t *testing.T, fields map[string]any)
	}{
		{
			name:   "includes loggable fields and excludes reason",
			mutate: func(in *ProtectionAuditEventInput) {},
			asserts: func(t *testing.T, fields map[string]any) {
				assert.Equal(t, "org-1", fields["organization_id"])
				assert.Equal(t, string(AuditEventTypeProvisioning), fields["event_type"])
				assert.Equal(t, string(AuditActionProvision), fields["action"])
				assert.Equal(t, string(AuditOutcomeSuccess), fields["outcome"])
				assert.Equal(t, "req-1", fields["request_id"])
				assert.Equal(t, "user", fields["actor_type"])

				_, hasReason := fields["reason"]
				assert.False(t, hasReason, "reason must be excluded from safe log fields")
			},
		},
		{
			name: "actor type defaulted is reflected in fields",
			mutate: func(in *ProtectionAuditEventInput) {
				in.ActorType = ""
			},
			asserts: func(t *testing.T, fields map[string]any) {
				assert.Equal(t, "service", fields["actor_type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := ProtectionAuditEventInput{
				TenantID:       "tenant-1",
				OrganizationID: "org-1",
				EventType:      AuditEventTypeProvisioning,
				Action:         AuditActionProvision,
				Outcome:        AuditOutcomeSuccess,
				ActorID:        "actor-1",
				ActorType:      "user",
				Reason:         "operator free-text note",
				RequestID:      "req-1",
			}
			tt.mutate(&in)

			ev, err := NewProtectionAuditEvent(in)
			require.NoError(t, err)

			fields := ev.SafeLogFields()
			tt.asserts(t, fields)
		})
	}
}

func TestProtectionAuditEvent_ErrAuditEventRequiredIsSentinel(t *testing.T) {
	t.Parallel()

	_, err := NewProtectionAuditEvent(ProtectionAuditEventInput{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrAuditEventRequired))
}
