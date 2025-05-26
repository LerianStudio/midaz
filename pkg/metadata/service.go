package metadata

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// Metadata represents the metadata structure
type Metadata struct {
	EntityID   string                 `json:"entity_id" bson:"entity_id"`
	EntityName string                 `json:"entity_name" bson:"entity_name"`
	Data       map[string]interface{} `json:"data" bson:"data"`
	CreatedAt  time.Time              `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at" bson:"updated_at"`
}

// Repository defines the interface for metadata storage operations
type Repository interface {
	// Create stores metadata for an entity
	Create(ctx context.Context, entityName string, metadata *Metadata) error

	// Update modifies existing metadata for an entity
	Update(ctx context.Context, entityName string, entityID string, data map[string]interface{}) error

	// Find retrieves metadata for a specific entity
	Find(ctx context.Context, entityName string, entityID string) (*Metadata, error)

	// FindByQuery retrieves metadata matching specific criteria
	FindByQuery(ctx context.Context, entityName string, query map[string]interface{}) ([]*Metadata, error)

	// Delete removes metadata for an entity
	Delete(ctx context.Context, entityName string, entityID string) error
}

// Service provides high-level metadata operations with validation and error handling
type Service struct {
	repo                Repository
	maxKeyLength        int
	maxValueLength      int
	allowedEntityTypes  map[string]bool
}

// NewService creates a new metadata service
func NewService(repo Repository, maxKeyLength, maxValueLength int) *Service {
	return &Service{
		repo:           repo,
		maxKeyLength:   maxKeyLength,
		maxValueLength: maxValueLength,
		allowedEntityTypes: map[string]bool{
			"Transaction":  true,
			"Operation":    true,
			"Balance":      true,
			"Account":      true,
			"Organization": true,
			"Ledger":       true,
			"Asset":        true,
			"Portfolio":    true,
			"Segment":      true,
		},
	}
}

// CreateMetadata creates metadata for an entity with validation
func (s *Service) CreateMetadata(ctx context.Context, entityType interface{}, entityID string, data map[string]interface{}) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata.create")
	defer span.End()

	// Get entity type name
	entityName := s.getEntityTypeName(entityType)
	
	// Validate entity type
	if !s.isAllowedEntityType(entityName) {
		err := errors.Errorf("entity type %s is not allowed", entityName)
		libOpentelemetry.HandleSpanError(&span, "Invalid entity type", err)
		return err
	}

	// Validate metadata
	if err := s.validateMetadata(data); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Invalid metadata", err)
		return err
	}

	// Create metadata object
	metadata := &Metadata{
		EntityID:   entityID,
		EntityName: entityName,
		Data:       data,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Store metadata
	if err := s.repo.Create(ctx, entityName, metadata); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create metadata", err)
		logger.Errorf("Error creating metadata for %s %s: %v", entityName, entityID, err)
		return err
	}

	logger.Infof("Created metadata for %s %s", entityName, entityID)
	return nil
}

// UpdateMetadata updates metadata for an entity with validation
func (s *Service) UpdateMetadata(ctx context.Context, entityType interface{}, entityID string, data map[string]interface{}) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata.update")
	defer span.End()

	// Get entity type name
	entityName := s.getEntityTypeName(entityType)
	
	// Validate entity type
	if !s.isAllowedEntityType(entityName) {
		err := errors.Errorf("entity type %s is not allowed", entityName)
		libOpentelemetry.HandleSpanError(&span, "Invalid entity type", err)
		return err
	}

	// Validate metadata
	if err := s.validateMetadata(data); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Invalid metadata", err)
		return err
	}

	// Update metadata
	if err := s.repo.Update(ctx, entityName, entityID, data); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata", err)
		logger.Errorf("Error updating metadata for %s %s: %v", entityName, entityID, err)
		return err
	}

	logger.Infof("Updated metadata for %s %s", entityName, entityID)
	return nil
}

// GetMetadata retrieves metadata for an entity
func (s *Service) GetMetadata(ctx context.Context, entityType interface{}, entityID string) (*Metadata, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata.get")
	defer span.End()

	// Get entity type name
	entityName := s.getEntityTypeName(entityType)

	// Retrieve metadata
	metadata, err := s.repo.Find(ctx, entityName, entityID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get metadata", err)
		return nil, err
	}

	return metadata, nil
}

// DeleteMetadata removes metadata for an entity
func (s *Service) DeleteMetadata(ctx context.Context, entityType interface{}, entityID string) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata.delete")
	defer span.End()

	// Get entity type name
	entityName := s.getEntityTypeName(entityType)

	// Delete metadata
	if err := s.repo.Delete(ctx, entityName, entityID); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete metadata", err)
		logger.Errorf("Error deleting metadata for %s %s: %v", entityName, entityID, err)
		return err
	}

	logger.Infof("Deleted metadata for %s %s", entityName, entityID)
	return nil
}

// SearchMetadata searches for metadata matching specific criteria
func (s *Service) SearchMetadata(ctx context.Context, entityType interface{}, query map[string]interface{}) ([]*Metadata, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata.search")
	defer span.End()

	// Get entity type name
	entityName := s.getEntityTypeName(entityType)

	// Search metadata
	results, err := s.repo.FindByQuery(ctx, entityName, query)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to search metadata", err)
		return nil, err
	}

	return results, nil
}

// validateMetadata validates metadata keys and values
func (s *Service) validateMetadata(metadata map[string]interface{}) error {
	if metadata == nil {
		return nil
	}

	for key, value := range metadata {
		// Check key length
		if len(key) > s.maxKeyLength {
			return errors.Errorf("metadata key '%s' exceeds maximum length of %d", key, s.maxKeyLength)
		}

		// Check value length for strings
		if str, ok := value.(string); ok && len(str) > s.maxValueLength {
			return errors.Errorf("metadata value for key '%s' exceeds maximum length of %d", key, s.maxValueLength)
		}
	}

	return nil
}

// getEntityTypeName gets the type name from an entity
func (s *Service) getEntityTypeName(entityType interface{}) string {
	t := reflect.TypeOf(entityType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// isAllowedEntityType checks if the entity type is allowed
func (s *Service) isAllowedEntityType(entityName string) bool {
	return s.allowedEntityTypes[entityName]
}

// SetAllowedEntityTypes updates the allowed entity types
func (s *Service) SetAllowedEntityTypes(types []string) {
	s.allowedEntityTypes = make(map[string]bool)
	for _, t := range types {
		s.allowedEntityTypes[t] = true
	}
}

// Helper function to create metadata with validation
func CreateWithValidation(ctx context.Context, service *Service, entityType interface{}, entityID uuid.UUID, metadata map[string]interface{}) error {
	if metadata == nil || len(metadata) == 0 {
		return nil
	}

	return service.CreateMetadata(ctx, entityType, entityID.String(), metadata)
}

// Helper function to merge metadata
func MergeMetadata(existing, new map[string]interface{}) map[string]interface{} {
	if existing == nil {
		return new
	}
	if new == nil {
		return existing
	}

	merged := make(map[string]interface{})
	
	// Copy existing metadata
	for k, v := range existing {
		merged[k] = v
	}
	
	// Override with new metadata
	for k, v := range new {
		merged[k] = v
	}
	
	return merged
}