package audit

import (
	"time"
)

const TreeCollection = "trees"

// AuditMongoDBModel represents the Audit into mongodb context
type AuditMongoDBModel struct {
	ID        AuditID           `bson:"_id"`
	TreeID    int64             `bson:"tree_id"`
	CreatedAt time.Time         `bson:"created_at"`
	Leaves    map[string]string `bson:"leaves"`
}

// Audit is a struct designed to encapsulate payload data.
type Audit struct {
	ID        AuditID `json:"-"`
	TreeID    int64
	Leaves    map[string]string
	CreatedAt time.Time `json:"createdAt"`
}

// AuditID is a struct that represents a composite audit key
type AuditID struct {
	OrganizationID string `json:"organizationId" bson:"organization_id"`
	LedgerID       string `json:"ledgerId" bson:"ledger_id"`
	ID             string `json:"auditId" bson:"audit_id"`
}

// ToEntity converts an AuditMongoDBModel to entity.Audit
func (mar *AuditMongoDBModel) ToEntity() *Audit {
	id := AuditID{
		OrganizationID: mar.ID.OrganizationID,
		LedgerID:       mar.ID.LedgerID,
		ID:             mar.ID.ID,
	}

	return &Audit{
		ID:        id,
		TreeID:    mar.TreeID,
		CreatedAt: mar.CreatedAt,
		Leaves:    mar.Leaves,
	}
}

// FromEntity converts an entity.Audit to AuditMongoDBModel
func (mar *AuditMongoDBModel) FromEntity(md *Audit) {
	mar.ID = md.ID
	mar.TreeID = md.TreeID
	mar.CreatedAt = md.CreatedAt
	mar.Leaves = md.Leaves
}
