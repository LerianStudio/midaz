package mmodel

import (
	"encoding/json"
	"github.com/google/uuid"
)

// Queue is a struct designed for internal message queueing.
//
// swagger:model Queue
// @Description Internal structure for message queue data transfer between services. Contains entity identifiers and a collection of queue data items.
type Queue struct {
	// Organization identifier for the queue message
	// format: uuid
	// example: 00000000-0000-0000-0000-000000000000
	OrganizationID uuid.UUID `json:"organizationId" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Ledger identifier for the queue message
	// format: uuid
	// example: 00000000-0000-0000-0000-000000000000
	LedgerID uuid.UUID `json:"ledgerId" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Audit trail identifier for tracking queue operations
	// format: uuid
	// example: 00000000-0000-0000-0000-000000000000
	AuditID uuid.UUID `json:"auditId" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Account identifier for the queue message
	// format: uuid
	// example: 00000000-0000-0000-0000-000000000000
	AccountID uuid.UUID `json:"accountId" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Array of data items contained in this queue message
	// required: true
	QueueData []QueueData `json:"queueData"`
} // @name Queue

// QueueData is a struct representing a single data item in a queue message.
//
// swagger:model QueueData
// @Description Individual data item within a queue message, containing a unique identifier and a JSON payload.
type QueueData struct {
	// Unique identifier for this queue data item
	// format: uuid
	// example: 00000000-0000-0000-0000-000000000000
	ID uuid.UUID `json:"id" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Raw JSON payload data
	// example: {"type": "transaction", "amount": 1000}
	Value json.RawMessage `json:"value"`
} // @name QueueData
