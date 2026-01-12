package in

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
)

// ErrorCodeTransformer is a Fiber middleware that intercepts error responses
// and transforms generic Midaz error codes to CRM-specific error codes.
//
// This middleware ensures backward compatibility for CRM API clients after
// the migration from a standalone repository to the Midaz monorepo.
//
// The middleware:
//  1. Executes the next handler in the chain
//  2. Checks if the response status is an error (>= 400)
//  3. Parses the JSON response body
//  4. Transforms the "code" field if a mapping exists
//  5. Re-serializes the response with the transformed code
//
// Non-JSON responses and successful responses are passed through unchanged.
func ErrorCodeTransformer() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Execute the next handler
		err := c.Next()

		// Only transform error responses (4xx and 5xx)
		statusCode := c.Response().StatusCode()
		if statusCode < 400 {
			return err
		}

		// Get the response body
		body := c.Response().Body()
		if len(body) == 0 {
			return err
		}

		// Try to parse as JSON with a "code" field
		transformed, ok := transformResponseCode(body)
		if ok {
			c.Response().SetBody(transformed)
		}

		return err
	}
}

// transformResponseCode attempts to parse the body as JSON and transform
// the error code. Returns the transformed body and true if successful,
// or the original body and false if transformation is not applicable.
func transformResponseCode(body []byte) ([]byte, bool) {
	// Parse as generic map to preserve all fields
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return body, false
	}

	// Check if there's a "code" field
	code, ok := response["code"].(string)
	if !ok {
		return body, false
	}

	// Transform the code
	newCode := TransformErrorCode(code)
	if newCode == code {
		// No transformation needed
		return body, false
	}

	// Update the code and re-serialize
	response["code"] = newCode

	transformed, err := json.Marshal(response)
	if err != nil {
		return body, false
	}

	return transformed, true
}
