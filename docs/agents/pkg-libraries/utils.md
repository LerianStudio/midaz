# pkg/utils - Validation & Pointer Utilities

**Location**: `pkg/utils/`
**Priority**: 🔥 High Use - Common utilities
**Status**: Production-ready validation and pointer helpers

Common utility functions for validation (ISO codes, formats) and pointer helpers.

## Validation Functions

### ValidateCountryAddress

Validate ISO 3166-1 alpha-2 country code:

```go
func ValidateCountryAddress(country string) error
```

**Returns**: `ErrInvalidCountryCode` if invalid

**Usage:**
```go
if err := utils.ValidateCountryAddress(address.Country); err != nil {
    return pkg.ValidateBusinessError(constant.ErrInvalidCountryCode, "organization")
}
```

**Supported Countries**: Full ISO 3166-1 alpha-2 list (AD, AE, AF, ..., ZW)

### ValidateAccountType

Validate account type enum:

```go
func ValidateAccountType(t string) error
```

**Valid Types**: deposit, savings, loans, marketplace, creditCard

**Returns**: `ErrInvalidAccountType` if invalid

**Usage:**
```go
if err := utils.ValidateAccountType(input.Type); err != nil {
    return pkg.ValidateBusinessError(constant.ErrInvalidAccountType, "account")
}
```

### ValidateType

Validate asset type enum:

```go
func ValidateType(t string) error
```

**Valid Types**: crypto, currency, commodity, others

**Returns**: `ErrInvalidAssetType` if invalid

**Usage:**
```go
if err := utils.ValidateType(asset.Type); err != nil {
    return pkg.ValidateBusinessError(constant.ErrInvalidType, "asset")
}
```

### ValidateCode

Validate code format (uppercase letters only):

```go
func ValidateCode(code string) error
```

**Returns**:
- `ErrCodeMustContainOnlyLetters` if contains non-letter characters
- `ErrCodeMustBeUppercase` if contains lowercase letters

**Usage:**
```go
if err := utils.ValidateCode(asset.Code); err != nil {
    return pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, "asset")
}
```

### ValidateCurrency

Validate ISO 4217 currency code:

```go
func ValidateCurrency(code string) error
```

**Returns**: `ErrInvalidCurrencyCode` if invalid

**Usage:**
```go
if asset.Type == "currency" {
    if err := utils.ValidateCurrency(asset.Code); err != nil {
        return pkg.ValidateBusinessError(constant.ErrCurrencyCodeStandardCompliance, "asset")
    }
}
```

**Supported Currencies**: Full ISO 4217 list (AED, AFN, ..., ZWL)

## Pointer Helpers

Create pointers to values (useful for optional fields):

```go
func StringPtr(s string) *string
func BoolPtr(b bool) *bool
func Float64Ptr(f64 float64) *float64
func IntPtr(i int) *int
```

**Usage:**
```go
input := &CreateAccountInput{
    Name: "Checking Account",
    Status: mmodel.Status{
        Code:        mmodel.ACTIVE,
        Description: utils.StringPtr("Active checking account"),
    },
    AllowSending:    utils.BoolPtr(true),
    AllowReceiving:  utils.BoolPtr(true),
}

// Check if pointer field is set
if input.Status.Description != nil {
    logger.Infof("Description: %s", *input.Status.Description)
}
```

## Common Patterns

### Use Case Input Validation

```go
func (uc *UseCase) CreateAccount(ctx context.Context, input *CreateAccountInput) (*Account, error) {
    // Validate required fields with assert
    assert.NotEmpty(input.Name, "name required")
    assert.NotEmpty(input.AssetCode, "asset code required")

    // Validate format with utils
    if err := utils.ValidateCode(input.AssetCode); err != nil {
        return nil, pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, "account")
    }

    // Create account
    // ...
}
```

### Asset Validation

```go
func (uc *UseCase) CreateAsset(ctx context.Context, input *CreateAssetInput) (*Asset, error) {
    // Validate type
    if err := utils.ValidateType(input.Type); err != nil {
        return nil, pkg.ValidateBusinessError(constant.ErrInvalidType, "asset")
    }

    // Validate code format
    if err := utils.ValidateCode(input.Code); err != nil {
        return nil, pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, "asset")
    }

    // If currency, validate against ISO 4217
    if input.Type == "currency" {
        if err := utils.ValidateCurrency(input.Code); err != nil {
            return nil, pkg.ValidateBusinessError(constant.ErrCurrencyCodeStandardCompliance, "asset")
        }
    }

    // Create asset
    // ...
}
```

### Organization Address Validation

```go
func (uc *UseCase) CreateOrganization(ctx context.Context, input *CreateOrganizationInput) (*Organization, error) {
    // Validate country code if address provided
    if input.Address != nil && input.Address.Country != "" {
        if err := utils.ValidateCountryAddress(input.Address.Country); err != nil {
            return nil, pkg.ValidateBusinessError(constant.ErrInvalidCountryCode, "organization")
        }
    }

    // Create organization
    // ...
}
```

### Optional Fields with Pointers

```go
// Creating entity with optional fields
account := &mmodel.Account{
    ID:          uuid.New(),
    Name:        input.Name,
    Description: utils.StringPtr(input.Description),  // Optional description
    Status: mmodel.Status{
        Code:        mmodel.ACTIVE,
        Description: utils.StringPtr("Active account"),
    },
}

// Checking optional field before use
if account.Description != nil {
    logger.Infof("Account description: %s", *account.Description)
}

// Using libCommons to check if pointer is nil or empty
if !libCommons.IsNilOrEmpty(account.Description) {
    logger.Infof("Account has description: %s", *account.Description)
}
```

## Error Constants

```go
var (
    ErrInvalidCountryCode         = errors.New("0032")
    ErrInvalidAccountType         = errors.New("0066")
    ErrInvalidAssetType           = errors.New("0040")
    ErrCodeMustContainOnlyLetters = errors.New("0033")
    ErrCodeMustBeUppercase        = errors.New("0004")
    ErrInvalidCurrencyCode        = errors.New("0005")
)
```

These should be used with `pkg.ValidateBusinessError()` to get user-facing messages.

## References

- **Source**: `pkg/utils/utils.go:1`, `pkg/utils/ptr.go:1`
- **Related**: [`errors.md`](./errors.md) for error handling
- **Related**: [`../libcommons.md`](../libcommons.md) for `IsNilOrEmpty()`

## Summary

`pkg/utils` provides common utilities:

1. **ISO validation** - Country codes (ISO 3166-1), currency codes (ISO 4217)
2. **Enum validation** - Account types, asset types
3. **Format validation** - Code format (uppercase letters only)
4. **Pointer helpers** - Create pointers for optional fields
5. **Use with pkg/errors** - Map validation errors to typed business errors

Always use these validators for consistent validation across the codebase.
