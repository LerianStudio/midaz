package holderlink

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

type MongoDBModel struct {
	ID        *uuid.UUID     `bson:"_id,omitempty"`
	HolderID  *uuid.UUID     `bson:"holder_id,omitempty"`
	AliasID   *uuid.UUID     `bson:"alias_id,omitempty"`
	LinkType  *string        `bson:"link_type,omitempty"`
	TpVinc    *int           `bson:"tp_vinc,omitempty"`
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
		TpVinc:    hl.TpVinc,
		CreatedAt: &hl.CreatedAt,
		UpdatedAt: &hl.UpdatedAt,
		DeletedAt: hl.DeletedAt,
	}

	if hl.LinkType != nil && hmm.TpVinc == nil {
		linkType := mmodel.LinkType(*hl.LinkType)
		if tpVinc, ok := mmodel.GetTpVincValue(linkType); ok {
			hmm.TpVinc = &tpVinc
		}
	}

	if hl.Metadata == nil {
		hmm.Metadata = make(map[string]any)
	} else {
		hmm.Metadata = hl.Metadata
	}
}

// ToEntity maps a MongoDB model to a holder link entity
func (hmm *MongoDBModel) ToEntity() *mmodel.HolderLink {
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
		TpVinc:    hmm.TpVinc,
		Metadata:  hmm.Metadata,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		DeletedAt: hmm.DeletedAt,
	}

	if holderLink.LinkType != nil && holderLink.TpVinc == nil {
		linkType := mmodel.LinkType(*holderLink.LinkType)
		if tpVinc, ok := mmodel.GetTpVincValue(linkType); ok {
			holderLink.TpVinc = &tpVinc
		}
	}

	return holderLink
}
