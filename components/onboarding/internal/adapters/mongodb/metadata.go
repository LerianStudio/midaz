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

// MetadataIndexMongoDBModel represents the metadata index into mongodb context
type MetadataIndexMongoDBModel struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	EntityName  string             `bson:"entity_name"`
	MetadataKey string             `bson:"metadata_key"`
	Unique      bool               `bson:"unique"`
	Sparse      bool               `bson:"sparse"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}

// MongoDBIndexInfo represents the native MongoDB index structure returned by Indexes().List()
type MongoDBIndexInfo struct {
	Key    primitive.D `bson:"key"`
	Name   string      `bson:"name"`
	Unique bool        `bson:"unique"`
	Sparse bool        `bson:"sparse"`
}

// MetadataIndex is a struct designed to encapsulate payload data.
type MetadataIndex struct {
	ID          primitive.ObjectID
	EntityName  string
	MetadataKey string
	Unique      bool
	Sparse      bool
	CreatedAt   time.Time
}

// ToEntity converts an MetadataIndexMongoDBModel to entity.MetadataIndex
func (mim *MetadataIndexMongoDBModel) ToEntity() *MetadataIndex {
	return &MetadataIndex{
		ID:          mim.ID,
		EntityName:  mim.EntityName,
		MetadataKey: mim.MetadataKey,
		Unique:      mim.Unique,
		Sparse:      mim.Sparse,
		CreatedAt:   mim.CreatedAt,
	}
}

// FromEntity converts an entity.MetadataIndex to MetadataIndexMongoDBModel
func (mim *MetadataIndexMongoDBModel) FromEntity(mi *MetadataIndex) error {
	mim.ID = mi.ID
	mim.EntityName = mi.EntityName
	mim.MetadataKey = mi.MetadataKey
	mim.Unique = mi.Unique
	mim.Sparse = mi.Sparse
	mim.CreatedAt = mi.CreatedAt

	return nil
}
