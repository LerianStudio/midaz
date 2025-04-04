# Midaz SDK Packages

This directory contains the core packages that make up the Midaz Go SDK. Each package is designed with a specific purpose, following Go's idiomatic practices and ensuring clean separation of concerns.

## Package Overview

| Package | Description |
|---------|-------------|
| [accounts](./accounts/) | Account management utilities |
| [api](./api/) | API interaction and error handling |
| [config](./config/) | SDK configuration |
| [conversion](./conversion/) | Data format conversion utilities |
| [errors](./errors/) | Error handling utilities |
| [format](./format/) | Data formatting utilities |
| [validation](./validation/) | Data validation utilities |

## Packages in Detail

### [accounts](./accounts/)

The accounts package provides utilities for working with Midaz accounts, making it easier to manage, filter, and display account information.

Key features:
- Account finding and filtering functions
- Balance formatting and summarization
- Account identification helpers

### [api](./api/)

The API package provides error handling utilities specifically for API interactions in the Midaz SDK.

Key features:
- Comprehensive error type hierarchy
- API response parsing
- Structured error formatting

### [config](./config/)

The config package provides configuration utilities for the Midaz SDK, allowing you to customize how the SDK interacts with the Midaz API.

Key features:
- Functional options pattern for configuration
- Default configurations for common scenarios
- Timeout and retry settings

### [conversion](./conversion/)

The conversion package provides utilities for converting between different data formats and creating human-readable representations of Midaz SDK models.

Key features:
- Date and time formatting
- Metadata and tag conversion
- Transaction format conversion

### [errors](./errors/)

The errors package provides standardized error handling utilities for the Midaz SDK, making it easier to create, check, and format errors consistently.

Key features:
- Standardized error codes
- Error checking functions
- User-friendly error formatting

### [format](./format/)

The format package provides formatting utilities for the Midaz SDK, making it easier to display data in a human-readable format.

Key features:
- Amount formatting with proper decimal handling
- Support for different scale factors

### [validation](./validation/)

The validation package provides utilities for validating various aspects of Midaz data before sending it to the API.

Key features:
- Transaction validation
- Asset code and type validation
- Account validation
- Metadata validation

## Usage Patterns

The packages in this directory are designed to work together to provide a comprehensive SDK for interacting with the Midaz API. Here are some common usage patterns:

1. **Configuration**: Use the `config` package to configure the SDK for your environment.
2. **Validation**: Use the `validation` package to validate data before sending it to the API.
3. **Error Handling**: Use the `errors` package to handle errors returned by the API.
4. **Account Management**: Use the `accounts` package to work with account data.
5. **Formatting**: Use the `format` package to format data for display.
6. **Conversion**: Use the `conversion` package to convert between different data formats.

## Best Practices

When working with the Midaz SDK, follow these best practices:

1. **Always validate inputs**: Use the validation package to validate data before sending it to the API.
2. **Handle errors appropriately**: Use the error checking functions to handle specific error types.
3. **Use the functional options pattern**: When configuring the SDK, use the functional options pattern to set only the options you need.
4. **Format data for display**: Use the formatting functions to ensure consistent display of data.
5. **Follow idiomatic Go practices**: The SDK is designed to follow Go's idiomatic practices, so follow them in your code as well.
