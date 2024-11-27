package audit

import (
	"time"
)

const TreeCollection = "tree"

// AuditMongoDBModel represents the Audit into mongodb context
type AuditMongoDBModel struct {
	ID        AuditID   `bson:"_id"`
	TreeID    int64     `bson:"audit_id"`
	CreatedAt time.Time `bson:"created_at"`
}

// Audit is a struct designed to encapsulate payload data.
type Audit struct {
	ID        AuditID
	TreeID    int64
	CreatedAt time.Time
}

type AuditID struct {
	OrganizationID string `bson:"organization_id"`
	LedgerID       string `bson:"ledger_id"`
}

// AuditToEntity converts an AuditMongoDBModel to entity.Audit
func (mmm *AuditMongoDBModel) AuditToEntity() *Audit {

	AuditID := AuditID{
		OrganizationID: mmm.ID.OrganizationID,
		LedgerID:       mmm.ID.LedgerID,
	}

	return &Audit{
		ID:        AuditID,
		TreeID:    mmm.TreeID,
		CreatedAt: mmm.CreatedAt,
	}
}

// AuditFromEntity converts an entity.Audit to AuditMongoDBModel
func (mmm *AuditMongoDBModel) AuditFromEntity(md *Audit) {
	mmm.ID = md.ID
	mmm.TreeID = md.TreeID
	mmm.CreatedAt = md.CreatedAt
}

type LeafMongoDBModel struct {
	ID        LeafID    `bson:"_id"`
	TreeID    int64     `bson:"audit_id"`
	CreatedAt time.Time `bson:"created_at"`
}

type LeafID struct {
	OrganizationID string `bson:"organization_id"`
	LedgerID       string `bson:"ledger_id"`
	TransactionID  string `bson:"transaction_id"`
}
