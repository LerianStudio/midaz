package query

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/stretchr/testify/require"
)

func TestExtractMetadataData_Postcondition(t *testing.T) {
	// Postcondition: extractMetadataData must never return nil

	t.Run("returns non-nil map when metadata is nil", func(t *testing.T) {
		result := extractMetadataData(nil)
		require.NotNil(t, result, "postcondition: must return non-nil map")
		require.Empty(t, result)
	})

	t.Run("returns non-nil map when metadata.Data is nil", func(t *testing.T) {
		meta := &mongodb.Metadata{Data: nil}
		result := extractMetadataData(meta)
		require.NotNil(t, result, "postcondition: must return non-nil map")
		require.Empty(t, result)
	})

	t.Run("returns actual data when present", func(t *testing.T) {
		expected := map[string]any{"key": "value"}
		meta := &mongodb.Metadata{Data: expected}
		result := extractMetadataData(meta)
		require.Equal(t, expected, result)
	})
}
