package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateCountryAddress(t *testing.T) {
	tests := []struct {
		name        string
		country     string
		expectError bool
		errorCode   string
	}{
		{
			name:        "valid country - US",
			country:     "US",
			expectError: false,
		},
		{
			name:        "valid country - BR",
			country:     "BR",
			expectError: false,
		},
		{
			name:        "valid country - GB",
			country:     "GB",
			expectError: false,
		},
		{
			name:        "valid country - JP",
			country:     "JP",
			expectError: false,
		},
		{
			name:        "valid country - DE",
			country:     "DE",
			expectError: false,
		},
		{
			name:        "invalid country - lowercase",
			country:     "us",
			expectError: true,
			errorCode:   "0032",
		},
		{
			name:        "invalid country - three letter code",
			country:     "USA",
			expectError: true,
			errorCode:   "0032",
		},
		{
			name:        "invalid country - empty string",
			country:     "",
			expectError: true,
			errorCode:   "0032",
		},
		{
			name:        "invalid country - non-existent code",
			country:     "XX",
			expectError: true,
			errorCode:   "0032",
		},
		{
			name:        "invalid country - single character",
			country:     "U",
			expectError: true,
			errorCode:   "0032",
		},
		{
			name:        "invalid country - numeric",
			country:     "12",
			expectError: true,
			errorCode:   "0032",
		},
		{
			name:        "invalid country - special characters",
			country:     "U$",
			expectError: true,
			errorCode:   "0032",
		},
		{
			name:        "invalid country - mixed case",
			country:     "Us",
			expectError: true,
			errorCode:   "0032",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCountryAddress(tt.country)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorCode, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAccountType(t *testing.T) {
	tests := []struct {
		name        string
		accountType string
		expectError bool
		errorCode   string
	}{
		{
			name:        "valid type - deposit",
			accountType: "deposit",
			expectError: false,
		},
		{
			name:        "valid type - savings",
			accountType: "savings",
			expectError: false,
		},
		{
			name:        "valid type - loans",
			accountType: "loans",
			expectError: false,
		},
		{
			name:        "valid type - marketplace",
			accountType: "marketplace",
			expectError: false,
		},
		{
			name:        "valid type - creditCard",
			accountType: "creditCard",
			expectError: false,
		},
		{
			name:        "invalid type - empty string",
			accountType: "",
			expectError: true,
			errorCode:   "0066",
		},
		{
			name:        "invalid type - uppercase",
			accountType: "DEPOSIT",
			expectError: true,
			errorCode:   "0066",
		},
		{
			name:        "invalid type - non-existent",
			accountType: "checking",
			expectError: true,
			errorCode:   "0066",
		},
		{
			name:        "invalid type - partial match",
			accountType: "save",
			expectError: true,
			errorCode:   "0066",
		},
		{
			name:        "invalid type - with spaces",
			accountType: "credit Card",
			expectError: true,
			errorCode:   "0066",
		},
		{
			name:        "invalid type - camelCase variation",
			accountType: "CreditCard",
			expectError: true,
			errorCode:   "0066",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountType(tt.accountType)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorCode, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateType(t *testing.T) {
	tests := []struct {
		name        string
		assetType   string
		expectError bool
		errorCode   string
	}{
		{
			name:        "valid type - crypto",
			assetType:   "crypto",
			expectError: false,
		},
		{
			name:        "valid type - currency",
			assetType:   "currency",
			expectError: false,
		},
		{
			name:        "valid type - commodity",
			assetType:   "commodity",
			expectError: false,
		},
		{
			name:        "valid type - others",
			assetType:   "others",
			expectError: false,
		},
		{
			name:        "invalid type - empty string",
			assetType:   "",
			expectError: true,
			errorCode:   "0040",
		},
		{
			name:        "invalid type - uppercase",
			assetType:   "CRYPTO",
			expectError: true,
			errorCode:   "0040",
		},
		{
			name:        "invalid type - non-existent",
			assetType:   "stock",
			expectError: true,
			errorCode:   "0040",
		},
		{
			name:        "invalid type - partial match",
			assetType:   "curr",
			expectError: true,
			errorCode:   "0040",
		},
		{
			name:        "invalid type - with spaces",
			assetType:   " crypto",
			expectError: true,
			errorCode:   "0040",
		},
		{
			name:        "invalid type - mixed case",
			assetType:   "Crypto",
			expectError: true,
			errorCode:   "0040",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateType(tt.assetType)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorCode, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCode(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectError bool
		errorCode   string
	}{
		{
			name:        "valid code - USD",
			code:        "USD",
			expectError: false,
		},
		{
			name:        "valid code - BRL",
			code:        "BRL",
			expectError: false,
		},
		{
			name:        "invalid code - single letter (too short)",
			code:        "A",
			expectError: true,
			errorCode:   "0143",
		},
		{
			name:        "valid code - long uppercase (10 chars)",
			code:        "ABCDEFGHIJ",
			expectError: false,
		},
		{
			name:        "invalid code - too long (11 chars)",
			code:        "ABCDEFGHIJK",
			expectError: true,
			errorCode:   "0143",
		},
		{
			name:        "invalid code - empty string",
			code:        "",
			expectError: true,
			errorCode:   "0143",
		},
		{
			name:        "valid code - minimum length (2 chars)",
			code:        "AB",
			expectError: false,
		},
		{
			name:        "invalid code - lowercase",
			code:        "usd",
			expectError: true,
			errorCode:   "0004",
		},
		{
			name:        "invalid code - mixed case",
			code:        "Usd",
			expectError: true,
			errorCode:   "0004",
		},
		{
			name:        "invalid code - with numbers",
			code:        "USD1",
			expectError: true,
			errorCode:   "0033",
		},
		{
			name:        "invalid code - only numbers",
			code:        "123",
			expectError: true,
			errorCode:   "0033",
		},
		{
			name:        "invalid code - with special characters",
			code:        "US$",
			expectError: true,
			errorCode:   "0033",
		},
		{
			name:        "invalid code - with spaces",
			code:        "US D",
			expectError: true,
			errorCode:   "0033",
		},
		{
			name:        "invalid code - with hyphen",
			code:        "US-D",
			expectError: true,
			errorCode:   "0033",
		},
		{
			name:        "invalid code - with underscore",
			code:        "US_D",
			expectError: true,
			errorCode:   "0033",
		},
		{
			name:        "invalid code - lowercase in middle",
			code:        "UsD",
			expectError: true,
			errorCode:   "0004",
		},
		{
			name:        "invalid code - unicode letters lowercase",
			code:        "ÁBC",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCode(tt.code)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorCode, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCurrency(t *testing.T) {
	tests := []struct {
		name        string
		currency    string
		expectError bool
		errorCode   string
	}{
		{
			name:        "valid currency - USD",
			currency:    "USD",
			expectError: false,
		},
		{
			name:        "valid currency - BRL",
			currency:    "BRL",
			expectError: false,
		},
		{
			name:        "valid currency - EUR",
			currency:    "EUR",
			expectError: false,
		},
		{
			name:        "valid currency - JPY",
			currency:    "JPY",
			expectError: false,
		},
		{
			name:        "valid currency - GBP",
			currency:    "GBP",
			expectError: false,
		},
		{
			name:        "valid currency - CNY",
			currency:    "CNY",
			expectError: false,
		},
		{
			name:        "valid currency - CHF",
			currency:    "CHF",
			expectError: false,
		},
		{
			name:        "valid currency - AED",
			currency:    "AED",
			expectError: false,
		},
		{
			name:        "valid currency - ZWL",
			currency:    "ZWL",
			expectError: false,
		},
		{
			name:        "invalid currency - lowercase",
			currency:    "usd",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - empty string",
			currency:    "",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - non-existent",
			currency:    "XXX",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - two characters",
			currency:    "US",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - four characters",
			currency:    "USDD",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - with numbers",
			currency:    "US1",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - mixed case",
			currency:    "Usd",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - with special characters",
			currency:    "US$",
			expectError: true,
			errorCode:   "0005",
		},
		{
			name:        "invalid currency - deprecated code",
			currency:    "XYZ",
			expectError: true,
			errorCode:   "0005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCurrency(tt.currency)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorCode, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSafeTimePtr_NilInput(t *testing.T) {
	result := SafeTimePtr(nil)

	assert.True(t, result.IsZero())
	assert.Equal(t, time.Time{}, result)
}

func TestSafeTimePtr_ValidTimeInput(t *testing.T) {
	testTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)

	result := SafeTimePtr(&testTime)

	assert.False(t, result.IsZero())
	assert.Equal(t, testTime, result)
	assert.Equal(t, 2024, result.Year())
	assert.Equal(t, time.June, result.Month())
	assert.Equal(t, 15, result.Day())
}

func TestSafeTimePtr_ZeroTimeInput(t *testing.T) {
	zeroTime := time.Time{}

	result := SafeTimePtr(&zeroTime)

	assert.True(t, result.IsZero())
	assert.Equal(t, zeroTime, result)
}

func TestSafeTimePtr_PreservesTimezone(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	testTime := time.Date(2024, 12, 25, 14, 30, 0, 0, loc)

	result := SafeTimePtr(&testTime)

	assert.Equal(t, testTime, result)
	assert.Equal(t, loc.String(), result.Location().String())
}

func TestSafeTimePtr_PreservesNanoseconds(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 123456789, time.UTC)

	result := SafeTimePtr(&testTime)

	assert.Equal(t, 123456789, result.Nanosecond())
}
