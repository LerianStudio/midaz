package outbox

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetadataOutbox_Success(t *testing.T) {
	entityID := uuid.New().String()
	metadata := map[string]any{"key": "value"}

	entry, err := NewMetadataOutbox(entityID, EntityTypeTransaction, metadata)

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, entry.ID)
	assert.Equal(t, entityID, entry.EntityID)
	assert.Equal(t, EntityTypeTransaction, entry.EntityType)
	assert.Equal(t, StatusPending, entry.Status)
	assert.Equal(t, 0, entry.RetryCount)
	assert.Equal(t, DefaultMaxRetries, entry.MaxRetries)
}

func TestNewMetadataOutbox_EmptyEntityID(t *testing.T) {
	_, err := NewMetadataOutbox("", EntityTypeTransaction, map[string]any{"key": "value"})
	assert.ErrorIs(t, err, ErrEntityIDEmpty)
}

func TestNewMetadataOutbox_EntityIDTooLong(t *testing.T) {
	longID := strings.Repeat("a", MaxEntityIDLength+1)
	_, err := NewMetadataOutbox(longID, EntityTypeTransaction, map[string]any{"key": "value"})
	assert.ErrorIs(t, err, ErrEntityIDTooLong)
}

func TestNewMetadataOutbox_InvalidEntityType(t *testing.T) {
	_, err := NewMetadataOutbox(uuid.New().String(), "InvalidType", map[string]any{"key": "value"})
	assert.ErrorIs(t, err, ErrInvalidEntityType)
}

func TestNewMetadataOutbox_NilMetadata(t *testing.T) {
	_, err := NewMetadataOutbox(uuid.New().String(), EntityTypeTransaction, nil)
	assert.ErrorIs(t, err, ErrMetadataNil)
}

func TestNewMetadataOutbox_MetadataTooLarge(t *testing.T) {
	largeValue := strings.Repeat("A", MaxMetadataSize+1)
	_, err := NewMetadataOutbox(uuid.New().String(), EntityTypeOperation, map[string]any{"data": largeValue})
	assert.ErrorIs(t, err, ErrMetadataTooLarge)
}

func TestMetadataOutboxPostgreSQLModel_RoundTrip(t *testing.T) {
	entry, err := NewMetadataOutbox("test-id", EntityTypeTransaction, map[string]any{"foo": "bar"})
	require.NoError(t, err)

	model := &MetadataOutboxPostgreSQLModel{}
	err = model.FromEntity(entry)
	require.NoError(t, err)

	restored, err := model.ToEntity()
	require.NoError(t, err)

	assert.Equal(t, entry.ID, restored.ID)
	assert.Equal(t, entry.EntityID, restored.EntityID)
	assert.Equal(t, entry.EntityType, restored.EntityType)
	assert.Equal(t, entry.Status, restored.Status)
}

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		contains    string
		notContains string
	}{
		{"email", "Error for user@example.com", "[REDACTED]", "user@example.com"},
		{"phone", "Contact: 555-123-4567", "[REDACTED]", "555-123-4567"},
		{"ip", "From IP: 192.168.1.100", "[REDACTED]", "192.168.1.100"},
		{"truncate", strings.Repeat("A", 600), "...[truncated]", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeErrorMessage(tt.input)
			if tt.contains != "" {
				assert.Contains(t, result, tt.contains)
			}
			if tt.notContains != "" {
				assert.NotContains(t, result, tt.notContains)
			}
		})
	}
}

func TestSecureRandomFloat64_Range(t *testing.T) {
	for i := 0; i < 100; i++ {
		val := SecureRandomFloat64()
		assert.GreaterOrEqual(t, val, 0.0)
		assert.Less(t, val, 1.0)
	}
}

func TestOutboxStatus_Values(t *testing.T) {
	assert.Equal(t, OutboxStatus("PENDING"), StatusPending)
	assert.Equal(t, OutboxStatus("PROCESSING"), StatusProcessing)
	assert.Equal(t, OutboxStatus("PUBLISHED"), StatusPublished)
	assert.Equal(t, OutboxStatus("FAILED"), StatusFailed)
	assert.Equal(t, OutboxStatus("DLQ"), StatusDLQ)
}
