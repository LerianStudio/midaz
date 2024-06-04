package metadata

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MetadataMongoDBModel represents the metadata into mongodb context
type MetadataMongoDBModel struct {
	ID         primitive.ObjectID `bson:"_id"`
	EntityID   string             `bson:"entity_id"`
	EntityName string             `bson:"entity_name"`
	Data       JSON               `bson:"metadata"`
	CreatedAt  time.Time          `bson:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"`
}

// Metadata is a struct designed to encapsulate payload data.
type Metadata struct {
	ID         primitive.ObjectID
	EntityID   string
	EntityName string
	Data       JSON
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// JSON document to save on mongodb
type JSON map[string]any

// Value return marshall value data
func (mj JSON) Value() (driver.Value, error) {
	return json.Marshal(mj)
}

// Scan unmarshall value data
func (mj *JSON) Scan(value any) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &mj)
}

// ToEntity converts an MetadataMongoDBModel to entity.Metadata
func (mmm *MetadataMongoDBModel) ToEntity() *Metadata {
	return &Metadata{
		ID:         mmm.ID,
		EntityID:   mmm.EntityID,
		EntityName: mmm.EntityName,
		Data:       mmm.Data,
		CreatedAt:  mmm.CreatedAt,
		UpdatedAt:  mmm.UpdatedAt,
	}
}

// FromEntity converts an entity.Metadata to MetadataMongoDBModel
func (mmm *MetadataMongoDBModel) FromEntity(md *Metadata) error {
	mmm.ID = md.ID
	mmm.EntityID = md.EntityID
	mmm.EntityName = md.EntityName
	mmm.Data = md.Data
	mmm.CreatedAt = md.CreatedAt
	mmm.UpdatedAt = md.UpdatedAt

	return nil
}
