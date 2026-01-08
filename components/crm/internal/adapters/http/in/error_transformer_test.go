package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "transforms InvalidMetadataNesting to CRM code",
			input:    constant.ErrInvalidMetadataNesting.Error(),
			expected: constant.ErrInvalidMetadataNestingCRM.Error(),
		},
		{
			name:     "transforms MetadataKeyLengthExceeded to CRM code",
			input:    constant.ErrMetadataKeyLengthExceeded.Error(),
			expected: constant.ErrMetadataKeyLengthExceededCRM.Error(),
		},
		{
			name:     "transforms MissingFieldsInRequest to CRM code",
			input:    constant.ErrMissingFieldsInRequest.Error(),
			expected: constant.ErrMissingFieldsInRequestCRM.Error(),
		},
		{
			name:     "transforms BadRequest to CRM code",
			input:    constant.ErrBadRequest.Error(),
			expected: constant.ErrBadRequestCRM.Error(),
		},
		{
			name:     "transforms UnexpectedFieldsInTheRequest to CRM code",
			input:    constant.ErrUnexpectedFieldsInTheRequest.Error(),
			expected: constant.ErrUnexpectedFieldsInTheRequestCRM.Error(),
		},
		{
			name:     "transforms InvalidRequestBody to CRM code",
			input:    constant.ErrInvalidRequestBody.Error(),
			expected: constant.ErrInvalidFieldTypeInRequest.Error(),
		},
		{
			name:     "returns original code when no mapping exists",
			input:    "UNKNOWN-CODE",
			expected: "UNKNOWN-CODE",
		},
		{
			name:     "returns CRM code unchanged (already transformed)",
			input:    constant.ErrHolderNotFound.Error(),
			expected: constant.ErrHolderNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformErrorCode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformResponseCode(t *testing.T) {
	tests := []struct {
		name            string
		inputBody       []byte
		expectedChanged bool
		expectedCode    string
	}{
		{
			name:            "transforms generic code in JSON response",
			inputBody:       []byte(`{"code":"0067","title":"Invalid Metadata","message":"error"}`),
			expectedChanged: true,
			expectedCode:    "CRM-0001",
		},
		{
			name:            "preserves other fields when transforming",
			inputBody:       []byte(`{"code":"0009","title":"Missing Fields","message":"Please provide required fields","fields":{"name":"required"}}`),
			expectedChanged: true,
			expectedCode:    "CRM-0003",
		},
		{
			name:            "returns unchanged for unmapped codes",
			inputBody:       []byte(`{"code":"9999","title":"Unknown","message":"error"}`),
			expectedChanged: false,
			expectedCode:    "9999",
		},
		{
			name:            "returns unchanged for non-JSON body",
			inputBody:       []byte(`not a json`),
			expectedChanged: false,
			expectedCode:    "",
		},
		{
			name:            "returns unchanged when code field is missing",
			inputBody:       []byte(`{"title":"Error","message":"something went wrong"}`),
			expectedChanged: false,
			expectedCode:    "",
		},
		{
			name:            "returns unchanged when code is not a string",
			inputBody:       []byte(`{"code":123,"title":"Error","message":"error"}`),
			expectedChanged: false,
			expectedCode:    "",
		},
		{
			name:            "returns unchanged for empty body",
			inputBody:       []byte{},
			expectedChanged: false,
			expectedCode:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changed := transformResponseCode(tt.inputBody)
			assert.Equal(t, tt.expectedChanged, changed)

			if changed {
				var response map[string]any
				err := json.Unmarshal(result, &response)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedCode, response["code"])
			}
		})
	}
}

func TestErrorCodeTransformerMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		handlerStatus  int
		handlerBody    map[string]any
		expectedStatus int
		expectedCode   string
	}{
		{
			name:          "transforms error code on 400 response",
			handlerStatus: http.StatusBadRequest,
			handlerBody: map[string]any{
				"code":    "0009",
				"title":   "Missing Fields",
				"message": "Required fields missing",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "CRM-0003",
		},
		{
			name:          "transforms error code on 422 response",
			handlerStatus: http.StatusUnprocessableEntity,
			handlerBody: map[string]any{
				"code":    "0067",
				"title":   "Invalid Metadata",
				"message": "Nested metadata not allowed",
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   "CRM-0001",
		},
		{
			name:          "transforms error code on 500 response",
			handlerStatus: http.StatusInternalServerError,
			handlerBody: map[string]any{
				"code":    "0046",
				"title":   "Internal Error",
				"message": "Something went wrong",
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "CRM-0014",
		},
		{
			name:          "does not transform on 200 response",
			handlerStatus: http.StatusOK,
			handlerBody: map[string]any{
				"code": "0009", // This should NOT be transformed
				"data": "success",
			},
			expectedStatus: http.StatusOK,
			expectedCode:   "0009", // Unchanged
		},
		{
			name:          "does not transform on 201 response",
			handlerStatus: http.StatusCreated,
			handlerBody: map[string]any{
				"id":   "123",
				"code": "0009", // This should NOT be transformed
			},
			expectedStatus: http.StatusCreated,
			expectedCode:   "0009", // Unchanged
		},
		{
			name:          "preserves CRM codes that are already correct",
			handlerStatus: http.StatusNotFound,
			handlerBody: map[string]any{
				"code":    "CRM-0006",
				"title":   "Holder Not Found",
				"message": "The holder does not exist",
			},
			expectedStatus: http.StatusNotFound,
			expectedCode:   "CRM-0006", // Already CRM code, unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()

			// Apply the middleware
			app.Use(ErrorCodeTransformer())

			// Create a test handler that returns the specified status and body
			app.Get("/test", func(c *fiber.Ctx) error {
				return c.Status(tt.handlerStatus).JSON(tt.handlerBody)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Parse response body
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response map[string]any
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)

			// Verify code transformation
			assert.Equal(t, tt.expectedCode, response["code"])
		})
	}
}

func TestErrorCodeTransformerPreservesAllFields(t *testing.T) {
	app := fiber.New()
	app.Use(ErrorCodeTransformer())

	app.Get("/test", func(c *fiber.Ctx) error {
		return c.Status(http.StatusBadRequest).JSON(map[string]any{
			"code":    "0009",
			"title":   "Missing Fields in Request",
			"message": "Your request is missing required fields",
			"fields": map[string]string{
				"name":     "required",
				"document": "required",
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response map[string]any
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// Verify code was transformed
	assert.Equal(t, "CRM-0003", response["code"])

	// Verify other fields are preserved
	assert.Equal(t, "Missing Fields in Request", response["title"])
	assert.Equal(t, "Your request is missing required fields", response["message"])

	fields, ok := response["fields"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "required", fields["name"])
	assert.Equal(t, "required", fields["document"])
}
