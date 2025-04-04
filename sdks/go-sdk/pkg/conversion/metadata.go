// Package conversion provides utilities for converting between different data formats
// and creating human-readable representations of Midaz SDK models.
package conversion

import (
	"strings"
)

// ConvertMetadataToTags extracts tags from transaction metadata.
// By convention, tags are stored in metadata as a "tags" key with a comma-separated value.
//
// Example:
//
//	// Extract tags from a transaction's metadata
//	tx := &models.Transaction{
//	    ID: "tx_123456",
//	    Metadata: map[string]any{
//	        "reference": "INV-789",
//	        "tags": "payment,recurring,automated",
//	    },
//	}
//	tags := conversion.ConvertMetadataToTags(tx.Metadata)
//	// Result: []string{"payment", "recurring", "automated"}
func ConvertMetadataToTags(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}

	// Check if there's a tags field
	tagsValue, ok := metadata["tags"]

	if !ok {
		return nil
	}

	// Convert to string
	tagsStr, ok := tagsValue.(string)

	if !ok {
		return nil
	}

	// Handle empty tags string
	if tagsStr == "" {
		return []string{}
	}

	// Split by comma
	tags := strings.Split(tagsStr, ",")

	// Trim whitespace
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}

	// Filter out empty tags
	result := []string{}

	for _, tag := range tags {
		if tag != "" {
			result = append(result, tag)
		}
	}

	return result
}

// ConvertTagsToMetadata adds tags to transaction metadata.
// By convention, tags are stored in metadata as a "tags" key with a comma-separated value.
//
// Example:
//
//	// Adding tags to a transaction
//	txInput := &models.TransactionDSLInput{
//	    Description: "Monthly subscription payment",
//	    Metadata: map[string]any{
//	        "reference": "INV-123",
//	        "customerId": "CUST-456",
//	    },
//	}
//	tags := []string{"payment", "recurring", "subscription"}
//	txInput.Metadata = conversion.ConvertTagsToMetadata(txInput.Metadata, tags)
//	// txInput.Metadata now contains:
//	// map[string]any{
//	//   "reference": "INV-123",
//	//   "customerId": "CUST-456",
//	//   "tags": "payment,recurring,subscription",
//	// }
func ConvertTagsToMetadata(metadata map[string]any, tags []string) map[string]any {
	if len(tags) == 0 {
		return metadata
	}

	// Create metadata if nil
	if metadata == nil {
		metadata = make(map[string]any)
	}

	// Clean tags
	var cleanTags []string
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			cleanTags = append(cleanTags, trimmed)
		}
	}

	// Join tags with comma
	tagsStr := strings.Join(cleanTags, ",")

	// Add to metadata
	metadata["tags"] = tagsStr

	return metadata
}
