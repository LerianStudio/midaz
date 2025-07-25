package mongodb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// MetadataMongoDBModel represents the metadata into mongodb context
type MetadataMongoDBModel struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
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

// ToDTO converts an MetadataMongoDBModel entity to Metadata dto.
func (mmm *MetadataMongoDBModel) ToDTO() *Metadata {
	return &Metadata{
		ID:         mmm.ID,
		EntityID:   mmm.EntityID,
		EntityName: mmm.EntityName,
		Data:       mmm.Data,
		CreatedAt:  mmm.CreatedAt,
		UpdatedAt:  mmm.UpdatedAt,
	}
}

// ToEntity is a func that convert Metadata dto to MetadataMongoDBModel entity.
func (metadata *Metadata) ToEntity() *MetadataMongoDBModel {
	return &MetadataMongoDBModel{
		ID:         metadata.ID,
		EntityID:   metadata.EntityID,
		EntityName: metadata.EntityName,
		Data:       metadata.Data,
		CreatedAt:  metadata.CreatedAt,
		UpdatedAt:  metadata.UpdatedAt,
	}
}
