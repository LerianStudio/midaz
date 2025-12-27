// Package holderlink provides MongoDB repository implementation for holder link entities.
package holderlink

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// MongoDBModel represents the MongoDB document structure for a holder link.
type MongoDBModel struct {
	ID        *uuid.UUID     `bson:"_id,omitempty"`
	HolderID  *uuid.UUID     `bson:"holder_id,omitempty"`
	AliasID   *uuid.UUID     `bson:"alias_id,omitempty"`
	LinkType  *string        `bson:"link_type,omitempty"`
	Metadata  map[string]any `bson:"metadata,omitempty"`
	CreatedAt *time.Time     `bson:"created_at,omitempty"`
	UpdatedAt *time.Time     `bson:"updated_at,omitempty"`
	DeletedAt *time.Time     `bson:"deleted_at,omitempty"`
}

// FromEntity maps a holder link entity to a MongoDB model
func (hmm *MongoDBModel) FromEntity(hl *mmodel.HolderLink) {
	*hmm = MongoDBModel{
		ID:        hl.ID,
		HolderID:  hl.HolderID,
		AliasID:   hl.AliasID,
		LinkType:  hl.LinkType,
		CreatedAt: &hl.CreatedAt,
		UpdatedAt: &hl.UpdatedAt,
		DeletedAt: hl.DeletedAt,
	}

	if hl.Metadata == nil {
		hmm.Metadata = make(map[string]any)
	} else {
		hmm.Metadata = hl.Metadata
	}
}

// ToEntity maps a MongoDB model to a holder link entity
func (hmm *MongoDBModel) ToEntity() *mmodel.HolderLink {
	// Required fields must be present in stored documents.
	// Nil values here indicate data corruption or schema mismatch.
	assert.NotNil(hmm.ID, "HolderLink ID must not be nil in stored document",
		"model", "HolderLinkMongoDBModel")
	assert.NotNil(hmm.HolderID, "HolderLink HolderID must not be nil in stored document",
		"model", "HolderLinkMongoDBModel",
		"id", hmm.ID)
	assert.NotNil(hmm.AliasID, "HolderLink AliasID must not be nil in stored document",
		"model", "HolderLinkMongoDBModel",
		"id", hmm.ID)
	assert.NotNil(hmm.LinkType, "HolderLink LinkType must not be nil in stored document",
		"model", "HolderLinkMongoDBModel",
		"id", hmm.ID)

	var createdAt, updatedAt time.Time
	if hmm.CreatedAt != nil {
		createdAt = *hmm.CreatedAt
	}

	if hmm.UpdatedAt != nil {
		updatedAt = *hmm.UpdatedAt
	}

	holderLink := &mmodel.HolderLink{
		ID:        hmm.ID,
		HolderID:  hmm.HolderID,
		AliasID:   hmm.AliasID,
		LinkType:  hmm.LinkType,
		Metadata:  hmm.Metadata,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		DeletedAt: hmm.DeletedAt,
	}

	return holderLink
}
