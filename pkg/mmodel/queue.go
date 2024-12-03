package mmodel

import (
	"github.com/google/uuid"
)

type Queue struct {
	OrganizationID uuid.UUID   `json:"organizationId"`
	LedgerID       uuid.UUID   `json:"ledgerId"`
	AuditID        uuid.UUID   `json:"auditId"`
	QueueData      []QueueData `json:"queueData"`
}

type QueueData struct {
	ID    uuid.UUID `json:"id"`
	Value string    `json:"value"`
}
