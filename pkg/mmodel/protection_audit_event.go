// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// AuditEventType classifies the kind of protection operation being audited.
// It is an open string alias so additional event types can be added later
// without changing the model.
type AuditEventType string

// AuditAction names the specific protection action that produced the event.
// It is an open string alias for forward extensibility.
type AuditAction string

// AuditOutcome records the result of the audited action.
// It is an open string alias for forward extensibility.
type AuditOutcome string

// defaultActorType is assigned when an event input omits ActorType.
const defaultActorType = "service"

// Phase-1 protection audit constants.
//
// The outcome set is limited to what ProvisioningService.Provision() actually
// produces: success, failure, and the idempotent already_exists.
const (
	AuditEventTypeProvisioning AuditEventType = "provisioning"

	AuditActionProvision AuditAction = "provision"

	AuditOutcomeSuccess       AuditOutcome = "success"
	AuditOutcomeFailure       AuditOutcome = "failure"
	AuditOutcomeAlreadyExists AuditOutcome = "already_exists"
)

// AuditDetails carries non-sensitive contextual details for an audit event.
// It MUST NOT hold plaintext, keysets, wrapped keysets, DEK/KEK material,
// credentials, balances, financial values, or PII.
type AuditDetails struct {
	PreviousStatus    string
	NewStatus         string
	AffectedKeyIDs    []uint32
	ProviderReference string
	ErrorCode         string
}

// ProtectionAuditEvent is an immutable record of a protection-related action.
// It MUST NOT hold plaintext, keysets, wrapped keysets, DEK/KEK material,
// credentials, balances, financial values, or PII.
type ProtectionAuditEvent struct {
	ID             uuid.UUID
	TenantID       string
	OrganizationID string
	EventType      AuditEventType
	Action         AuditAction
	Outcome        AuditOutcome
	ActorID        string
	ActorType      string
	Reason         string
	Timestamp      time.Time
	RequestID      string
	Details        *AuditDetails
}

// ProtectionAuditEventInput holds the caller-supplied fields used to build a
// ProtectionAuditEvent. ID and Timestamp are assigned by the constructor.
type ProtectionAuditEventInput struct {
	TenantID       string
	OrganizationID string
	EventType      AuditEventType
	Action         AuditAction
	Outcome        AuditOutcome
	ActorID        string
	ActorType      string
	Reason         string
	RequestID      string
	Details        *AuditDetails
}

// NewProtectionAuditEvent builds a ProtectionAuditEvent from the input,
// assigning a fresh ID and a single UTC timestamp. EventType, Action, and
// OrganizationID are required; a missing one yields ErrAuditEventRequired.
// A nil Details is allowed, and an empty ActorType defaults to "service".
func NewProtectionAuditEvent(input ProtectionAuditEventInput) (*ProtectionAuditEvent, error) {
	if strings.TrimSpace(string(input.EventType)) == "" {
		return nil, pkg.ValidateBusinessError(constant.ErrAuditEventRequired, constant.EntityProtectionAuditEvent)
	}

	if strings.TrimSpace(string(input.Action)) == "" {
		return nil, pkg.ValidateBusinessError(constant.ErrAuditEventRequired, constant.EntityProtectionAuditEvent)
	}

	if strings.TrimSpace(input.OrganizationID) == "" {
		return nil, pkg.ValidateBusinessError(constant.ErrAuditEventRequired, constant.EntityProtectionAuditEvent)
	}

	actorType := input.ActorType
	if actorType == "" {
		actorType = defaultActorType
	}

	id, err := libCommons.GenerateUUIDv7()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	return &ProtectionAuditEvent{
		ID:             id,
		TenantID:       input.TenantID,
		OrganizationID: input.OrganizationID,
		EventType:      input.EventType,
		Action:         input.Action,
		Outcome:        input.Outcome,
		ActorID:        input.ActorID,
		ActorType:      actorType,
		Reason:         input.Reason,
		Timestamp:      now,
		RequestID:      input.RequestID,
		Details:        input.Details,
	}, nil
}

// SafeLogFields returns the subset of fields that are safe to emit in logs.
// It deliberately excludes Reason, which may carry operator free-text, along
// with any actor, tenant, or detail data that could be sensitive.
func (e *ProtectionAuditEvent) SafeLogFields() map[string]any {
	return map[string]any{
		"organization_id": e.OrganizationID,
		"event_type":      string(e.EventType),
		"action":          string(e.Action),
		"outcome":         string(e.Outcome),
		"request_id":      e.RequestID,
		"actor_type":      e.ActorType,
	}
}
