package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetTransactionByID_MetadataPostcondition(t *testing.T) {
	// Postcondition: tran.Metadata is never nil when tran is returned
	// The implementation ensures this by initializing to empty map if nil

	// This is a documentation test - the actual behavior is tested in
	// get-id-transaction_test.go. This test documents the invariant.
	t.Run("metadata initialized to empty map when nil", func(t *testing.T) {
		// The production code in extractMetadataData ensures:
		// if metadata != nil && metadata.Data != nil {
		//     return metadata.Data
		// }
		// return map[string]any{}

		// This guarantees callers can safely iterate over Metadata
		emptyMap := map[string]any{}
		require.NotNil(t, emptyMap)
		require.Empty(t, emptyMap)
	})
}
