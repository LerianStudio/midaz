// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import "time"

// validMetadataIndexEntities contains the valid entity names for metadata indexes
var validMetadataIndexEntities = map[string]bool{
	"transaction":       true,
	"operation":         true,
	"operation_route":   true,
	"transaction_route": true,
}

// IsValidMetadataIndexEntity checks if the entity name is valid for metadata index operations
func IsValidMetadataIndexEntity(entityName string) bool {
	return validMetadataIndexEntities[entityName]
}

// GetValidMetadataIndexEntities returns a slice of valid entity names for metadata indexes
func GetValidMetadataIndexEntities() []string {
	entities := make([]string, 0, len(validMetadataIndexEntities))
	for entity := range validMetadataIndexEntities {
		entities = append(entities, entity)
	}

	return entities
}

// CreateMetadataIndexInput is a struct designed to store CreateMetadataIndexInput data.
//
// swagger:model CreateMetadataIndexInput
// @Description CreateMetadataIndexInput payload
type CreateMetadataIndexInput struct {
	// The metadata key to index (without "metadata." prefix)
	// required: true
	// maxLength: 100
	MetadataKey string `json:"metadataKey" validate:"required,max=100,metadatakeyformat" example:"tier"`
	// Whether the index should enforce uniqueness
	// required: false
	// default: false
	Unique bool `json:"unique" example:"false"`
	// Whether the index should be sparse (only include documents with the field)
	// required: false
	// default: true
	Sparse *bool `json:"sparse" example:"true"`
} // @name CreateMetadataIndexInput

// IndexStats represents usage statistics for a MongoDB index
// @Description Usage statistics collected by MongoDB for an index
type IndexStats struct {
	// Number of operations that have used this index
	// example: 1523
	Accesses int64 `json:"accesses" example:"1523"`
	// Timestamp since when the statistics are being collected
	// example: 2024-12-01T10:30:00Z
	// format: date-time
	StatsSince *time.Time `json:"statsSince,omitempty" format:"date-time" example:"2024-12-01T10:30:00Z"`
} // @name IndexStats

// MetadataIndex represents a metadata index entity
// @Description Represents a custom MongoDB index on a metadata field
type MetadataIndex struct {
	// The name of the index in MongoDB
	IndexName string `json:"indexName" example:"metadata.tier_1"`
	// The entity/collection name where the index exists
	EntityName string `json:"entityName" example:"transaction"`
	// The metadata key that is indexed
	MetadataKey string `json:"metadataKey" example:"tier"`
	// Whether the index enforces uniqueness
	Unique bool `json:"unique" example:"false"`
	// Whether the index is sparse
	Sparse bool `json:"sparse" example:"true"`
	// Usage statistics for this index (only available on GET, not on CREATE)
	Stats *IndexStats `json:"stats,omitempty"`
} // @name MetadataIndex

// MetadataIndexes represents a paginated list of metadata indexes
// @Description Paginated list of metadata indexes
type MetadataIndexes struct {
	// Array of metadata index records returned in this page
	// example: [{"indexName":"metadata.tier_1","entityName":"transaction","metadataKey":"tier"}]
	Items []MetadataIndex `json:"items"`
	// Current page number in the pagination
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`
	// Maximum number of items per page
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name MetadataIndexes

// MetadataIndexResponse represents a success response containing a single metadata index.
//
// swagger:response MetadataIndexResponse
// @Description Successful response containing a single metadata index entity.
type MetadataIndexResponse struct {
	// in: body
	Body MetadataIndex
}

// MetadataIndexesResponse represents a success response containing a paginated list of metadata indexes.
//
// swagger:response MetadataIndexesResponse
// @Description Successful response containing a paginated list of metadata indexes.
type MetadataIndexesResponse struct {
	// in: body
	Body MetadataIndexes
}

// MetadataIndexErrorResponse represents an error response for metadata index operations.
//
// swagger:response MetadataIndexErrorResponse
// @Description Error response for metadata index operations with error code and message.
type MetadataIndexErrorResponse struct {
	// in: body
	Body struct {
		// Error code identifying the specific error
		// example: 400001
		Code int `json:"code"`
		// Human-readable error message
		// example: Invalid input: field 'metadataKey' is required
		Message string `json:"message"`
		// Additional error details if available
		// example: {"field": "metadataKey", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
