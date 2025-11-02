package in

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	midazHttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator/v2"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStructValidationWithActualValidator tests that the validator framework
// correctly enforces the valuemax struct tag on metadata fields with the
// actual validation limits (2000 characters per maintainer feedback).
func TestStructValidationWithActualValidator(t *testing.T) {
	// Create actual validator instance (same as production)
	validate := validator.New()

	// Set up English translations for error messages
	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")
	_ = enTranslations.RegisterDefaultTranslations(validate, trans)

	// Register the custom valuemax validator (same as production)
	err := validate.RegisterValidation("valuemax", midazHttp.ValidateMetadataValueMaxLength)
	require.NoError(t, err)

	// Register keymax validator
	err = validate.RegisterValidation("keymax", midazHttp.ValidateMetadataKeyMaxLength)
	require.NoError(t, err)

	// Register nonested validator
	err = validate.RegisterValidation("nonested", midazHttp.ValidateMetadataNestedValues)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		metadata      map[string]any
		expectError   bool
		errorContains string
	}{
		{
			name: "metadata value exactly 2000 chars - should pass",
			metadata: map[string]any{
				"description": strings.Repeat("a", 2000),
			},
			expectError: false,
		},
		{
			name: "metadata value exactly 100 chars - should pass",
			metadata: map[string]any{
				"description": strings.Repeat("a", 100),
			},
			expectError: false,
		},
		{
			name:          "metadata value 2001 chars - should fail",
			metadata:      map[string]any{"key": strings.Repeat("a", 2001)},
			expectError:   true,
			errorContains: "valuemax",
		},
		{
			name:          "metadata key 101 chars - should fail",
			metadata:      map[string]any{strings.Repeat("k", 101): "value"},
			expectError:   true,
			errorContains: "keymax",
		},
		{
			name:        "valid short metadata - should pass",
			metadata: map[string]any{
				"key": "short value",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create struct with validation tags (this is what matters!)
			type TestStruct struct {
				Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
			}

			input := TestStruct{
				Metadata: tc.metadata,
			}

			// ACTUALLY invoke the validator
			err := validate.Struct(input)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHTTPRequestMetadataValidation tests that HTTP requests with invalid
// metadata are rejected at the API boundary before reaching use cases.
func TestHTTPRequestMetadataValidation(t *testing.T) {
	// Setup Fiber app with actual validation middleware
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Create test handler
	handler := func(c *fiber.Ctx, input interface{}) error {
		// If we reach here, validation passed
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"message": "success",
		})
	}

	// Define test input struct with actual validation tags (valuemax=2000)
	type TestInput struct {
		Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	}

	// Register route with FiberHandlerFunc (this invokes ValidateStruct!)
	app.Post("/test", midazHttp.FiberHandlerFunc(new(TestInput), handler))

	testCases := []struct {
		name         string
		requestBody  map[string]any
		expectStatus int
	}{
		{
			name: "valid metadata 100 chars",
			requestBody: map[string]any{
				"metadata": map[string]any{
					"description": strings.Repeat("a", 100),
				},
			},
			expectStatus: 201,
		},
		{
			name: "valid metadata 2000 chars",
			requestBody: map[string]any{
				"metadata": map[string]any{
					"description": strings.Repeat("a", 2000),
				},
			},
			expectStatus: 201,
		},
		{
			name: "invalid metadata 2001 chars - should reject",
			requestBody: map[string]any{
				"metadata": map[string]any{
					"description": strings.Repeat("a", 2001),
				},
			},
			expectStatus: 400,
		},
		{
			name: "invalid key 101 chars - should reject",
			requestBody: map[string]any{
				"metadata": map[string]any{
					strings.Repeat("k", 101): "value",
				},
			},
			expectStatus: 400,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tc.requestBody)

			req := httptest.NewRequest("POST", "/test", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)

			assert.Equal(t, tc.expectStatus, resp.StatusCode,
				"Expected status %d but got %d", tc.expectStatus, resp.StatusCode)
		})
	}
}

// TestIssue1454_NoRetryLoopForInvalidMetadata verifies that transactions
// with metadata values exceeding the limit are rejected at the HTTP
// layer and do NOT enter a retry loop as described in issue #1454.
func TestIssue1454_NoRetryLoopForInvalidMetadata(t *testing.T) {
	// This test reproduces the EXACT scenario from issue #1454
	// but with the 2000 character limit per maintainer feedback

	app := fiber.New()

	type TransactionInput struct {
		Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	}

	callCount := 0
	handler := func(c *fiber.Ctx, input *TransactionInput) error {
		callCount++
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"message": "transaction created",
		})
	}

	app.Post("/transactions", midazHttp.FiberHandlerFunc(new(TransactionInput), handler))

	// Test 1: Send request with 150-character metadata value (should pass with valuemax=2000)
	{
		requestBody := map[string]any{
			"metadata": map[string]any{
				"description": strings.Repeat("x", 150),
			},
		}

		bodyBytes, _ := json.Marshal(requestBody)
		req := httptest.NewRequest("POST", "/transactions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		require.NoError(t, err)

		// With valuemax=2000, 150 chars should be ACCEPTED
		assert.Equal(t, 201, resp.StatusCode, "150-char metadata should be accepted with valuemax=2000")
		assert.Equal(t, 1, callCount, "Handler should be called for valid request")
	}

	// Reset call count
	callCount = 0

	// Test 2: Send request with 2001-character metadata value (should fail)
	{
		requestBody := map[string]any{
			"metadata": map[string]any{
				"description": strings.Repeat("x", 2001),
			},
		}

		bodyBytes, _ := json.Marshal(requestBody)
		req := httptest.NewRequest("POST", "/transactions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		require.NoError(t, err)

		// CRITICAL ASSERTIONS:

		// 1. Request should be REJECTED at HTTP layer
		assert.Equal(t, 400, resp.StatusCode,
			"Request with 2001-char metadata should be rejected with 400")

		// 2. Handler should NEVER be called (validation happens before handler)
		assert.Equal(t, 0, callCount,
			"Handler should not be invoked for invalid request")

		// 3. Error message should mention metadata validation
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "metadata",
			"Error response should mention metadata validation")

		// ✅ This proves:
		// - Invalid metadata is caught at HTTP layer
		// - Request never reaches use case layer
		// - No retry loop can occur (request is rejected immediately)
		// - Issue #1454 is FIXED (for the appropriate character limit)
	}
}

// TestTransactionInputTypesMetadataValidation tests that all transaction input
// types properly enforce metadata validation at the HTTP layer.
func TestTransactionInputTypesMetadataValidation(t *testing.T) {
	validate := validator.New()

	// Set up English translations
	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")
	_ = enTranslations.RegisterDefaultTranslations(validate, trans)

	// Register custom validators
	_ = validate.RegisterValidation("valuemax", midazHttp.ValidateMetadataValueMaxLength)
	_ = validate.RegisterValidation("keymax", midazHttp.ValidateMetadataKeyMaxLength)
	_ = validate.RegisterValidation("nonested", midazHttp.ValidateMetadataNestedValues)

	// Test all 4 input types that have metadata fields
	inputTypes := []struct {
		name  string
		input interface{}
	}{
		{
			name:  "CreateTransactionInput",
			input: &transaction.CreateTransactionInput{},
		},
		{
			name:  "UpdateTransactionInput",
			input: &transaction.UpdateTransactionInput{},
		},
		{
			name:  "CreateTransactionInflowInput",
			input: &transaction.CreateTransactionInflowInput{},
		},
		{
			name:  "CreateTransactionOutflowInput",
			input: &transaction.CreateTransactionOutflowInput{},
		},
	}

	for _, inputType := range inputTypes {
		t.Run(inputType.name, func(t *testing.T) {
			// Test with invalid metadata for each input type
			invalidMetadata := map[string]any{
				"key": strings.Repeat("a", 2001), // Exceeds valuemax=2000
			}

			// Use reflection to set metadata field
			inputValue := reflect.ValueOf(inputType.input).Elem()
			metadataField := inputValue.FieldByName("Metadata")
			if metadataField.IsValid() && metadataField.CanSet() {
				metadataField.Set(reflect.ValueOf(invalidMetadata))

				// Test validation
				err := validate.Struct(inputType.input)
				assert.Error(t, err, "Validation should fail for invalid metadata")
				assert.Contains(t, err.Error(), "valuemax", "Error should mention valuemax")
			}
		})
	}
}