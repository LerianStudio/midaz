package audit

import (
	"time"
)

const TreeCollection = "trees"

// AuditMongoDBModel represents the Audit into mongodb context
type AuditMongoDBModel struct {
	ID         AuditID           `bson:"_id"`
	TreeID     int64             `bson:"tree_id"`
	CreatedAt  time.Time         `bson:"created_at"`
	Operations map[string]string `bson:"operations"`
}

// Audit is a struct designed to encapsulate payload data.
type Audit struct {
	ID         AuditID `json:"-"`
	TreeID     int64
	Operations map[string]string
	CreatedAt  time.Time `json:"createdAt"`
}

type AuditID struct {
	OrganizationID string `json:"organizationId" bson:"organization_id"`
	LedgerID       string `json:"ledgerId" bson:"ledger_id"`
	TransactionID  string `json:"transactionId" bson:"transaction_id"`
}

// ToEntity converts an AuditMongoDBModel to entity.Audit
func (mar *AuditMongoDBModel) ToEntity() *Audit {

	AuditID := AuditID{
		OrganizationID: mar.ID.OrganizationID,
		LedgerID:       mar.ID.LedgerID,
		TransactionID:  mar.ID.TransactionID,
	}

	return &Audit{
		ID:         AuditID,
		TreeID:     mar.TreeID,
		CreatedAt:  mar.CreatedAt,
		Operations: mar.Operations,
	}
}

// FromEntity converts an entity.Audit to AuditMongoDBModel
func (mar *AuditMongoDBModel) FromEntity(md *Audit) {
	mar.ID = md.ID
	mar.TreeID = md.TreeID
	mar.CreatedAt = md.CreatedAt
	mar.Operations = md.Operations
}
