// Package mmodel defines domain models for the Midaz platform.
// This file contains queue message models for async processing.
package mmodel

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Queue represents an internal message queue structure for data transfer between services.
//
// swagger:model Queue
// @Description Internal structure for message queue data transfer between services. Contains entity identifiers and a collection of queue data items.
type Queue struct {
	// The organization identifier for the queue message.
	// format: uuid
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	OrganizationID uuid.UUID `json:"organizationId" format:"uuid" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`

	// The ledger identifier for the queue message.
	// format: uuid
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	LedgerID uuid.UUID `json:"ledgerId" format:"uuid" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`

	// The audit trail identifier for tracking queue operations.
	// format: uuid
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	AuditID uuid.UUID `json:"auditId" format:"uuid" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`

	// The account identifier for the queue message.
	// format: uuid
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	AccountID uuid.UUID `json:"accountId" format:"uuid" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`

	// An array of data items contained in this queue message.
	// required: true
	QueueData []QueueData `json:"queueData"`
} // @name Queue

// QueueData represents a single data item within a queue message.
//
// swagger:model QueueData
// @Description Individual data item within a queue message, containing a unique identifier and a JSON payload.
type QueueData struct {
	// The unique identifier for this queue data item.
	// format: uuid
	// example: 01965ed9-7fa4-75b2-8872-fc9e8509ab0a
	ID uuid.UUID `json:"id" format:"uuid" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`

	// The raw JSON payload data.
	// example: {"type": "transaction", "amount": 1000}
	Value json.RawMessage `json:"value"`
} // @name QueueData

// Event represents a single data event within a queue message's JSON payload.
//
// swagger:model Event
// @Description Individual struct event within json payload.
type Event struct {
	Source         string          `json:"source" example:"midaz"`
	EventType      string          `json:"eventType" example:"transaction"`
	Action         string          `json:"action" example:"APPROVED"`
	TimeStamp      time.Time       `json:"timestamp" example:"2025-06-26T16:00:00Z"`
	Version        string          `json:"version" example:"v2.2.2"`
	OrganizationID string          `json:"organizationId" format:"uuid" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	LedgerID       string          `json:"ledgerId" format:"uuid" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	Payload        json.RawMessage `json:"payload" format:"json"`
}
