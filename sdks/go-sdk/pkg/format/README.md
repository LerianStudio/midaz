# Format Package

The format package provides formatting utilities for the Midaz SDK, making it easier to display data in a human-readable format.

## Usage

Import the package in your Go code:

```go
import "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/format"
```

## Formatting Functions

### FormatAmount

Converts a numeric amount and scale to a human-readable string representation:

```go
func FormatAmount(amount int64, scale int) string
```

This function handles:
- Proper decimal placement based on scale
- Negative amounts
- Leading zeros for decimal parts

Example:
```go
// Format 12345 with scale 2 (representing 123.45)
formattedAmount := format.FormatAmount(12345, 2)
// Result: "123.45"

// Format -500 with scale 2 (representing -5.00)
formattedAmount := format.FormatAmount(-500, 2)
// Result: "-5.00"

// Format 7 with scale 3 (representing 0.007)
formattedAmount := format.FormatAmount(7, 3)
// Result: "0.007"
```

## Best Practices

1. Use `FormatAmount` when displaying monetary values to users
2. Always include the appropriate scale for the asset being displayed
3. For USD and most fiat currencies, use scale 2
4. For cryptocurrencies, use the appropriate scale for the asset (e.g., 8 for BTC)
