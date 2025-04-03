package mmodel

import (
	"encoding/json"

	"github.com/google/uuid"
)

// \1 represents an entity
type Queue struct {
	OrganizationID uuid.UUID   `json:"organizationId"`
	LedgerID       uuid.UUID   `json:"ledgerId"`
	AuditID        uuid.UUID   `json:"auditId"`
	AccountID      uuid.UUID   `json:"accountId"`
	QueueData      []QueueData `json:"queueData"`
}

// \1 represents an entity
type QueueData struct {
	ID    uuid.UUID       `json:"id"`
	Value json.RawMessage `json:"value"`
}
