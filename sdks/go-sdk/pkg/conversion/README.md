# Conversion Package

The conversion package provides utilities for converting between different data formats and creating human-readable representations of Midaz SDK models.

## Usage

Import the package in your Go code:

```go
import "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/conversion"
```

## Date Conversion

### ConvertToISODate

Formats a `time.Time` as an ISO date string (YYYY-MM-DD):

```go
func ConvertToISODate(t time.Time) string
```

Example:
```go
isoDate := conversion.ConvertToISODate(time.Now())
// Result: "2025-04-02"
```

### ConvertToISODateTime

Formats a `time.Time` as an ISO date-time string (YYYY-MM-DDThh:mm:ssZ):

```go
func ConvertToISODateTime(t time.Time) string
```

Example:
```go
isoDateTime := conversion.ConvertToISODateTime(time.Now())
// Result: "2025-04-02T15:04:05Z"
```

## Metadata Conversion

### ConvertMetadataToTags

Extracts tags from transaction metadata. By convention, tags are stored in metadata as a "tags" key with a comma-separated value:

```go
func ConvertMetadataToTags(metadata map[string]any) []string
```

Example:
```go
// Extract tags from a transaction's metadata
metadata := map[string]any{
    "reference": "INV-789",
    "tags": "payment,recurring,automated",
}
tags := conversion.ConvertMetadataToTags(metadata)
// Result: []string{"payment", "recurring", "automated"}
```

### ConvertTagsToMetadata

Adds tags to transaction metadata. By convention, tags are stored in metadata as a "tags" key with a comma-separated value:

```go
func ConvertTagsToMetadata(metadata map[string]any, tags []string) map[string]any
```

Example:
```go
// Adding tags to transaction metadata
metadata := map[string]any{
    "reference": "INV-123",
    "customerId": "CUST-456",
}
tags := []string{"payment", "recurring", "subscription"}
updatedMetadata := conversion.ConvertTagsToMetadata(metadata, tags)
// Result: map[string]any{
//   "reference": "INV-123",
//   "customerId": "CUST-456",
//   "tags": "payment,recurring,subscription",
// }
```

## Transaction Conversion

The package also includes utilities for converting between different transaction formats and representations.

## Best Practices

1. Use date conversion functions to ensure consistent date formatting across your application
2. Use metadata conversion functions to work with transaction tags in a standardized way
3. Follow the convention of storing tags as a comma-separated string in the "tags" metadata field
