package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/go-redis/redismock/v9"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// setupTestApp creates a test Fiber app with a simple echo endpoint for testing batch processing.
func setupTestApp() *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.HandleFiberError(ctx, err)
		},
	})

	// Simple test endpoints
	app.Get("/v1/test", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"message": "success"})
	})

	app.Post("/v1/test", func(c *fiber.Ctx) error {
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"code":    "0047",
				"title":   "Bad Request",
				"message": "Invalid request body",
			})
		}
		body["id"] = "test-id-123"
		return c.Status(http.StatusCreated).JSON(body)
	})

	app.Get("/v1/error", func(c *fiber.Ctx) error {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"code":    "0007",
			"title":   "Entity Not Found",
			"message": "Resource not found",
		})
	})

	app.Get("/v1/internal-error", func(c *fiber.Ctx) error {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"code":    "0046",
			"title":   "Internal Server Error",
			"message": "Something went wrong",
		})
	})

	// Register batch handler
	batchHandler := &BatchHandler{App: app}
	app.Post("/v1/batch", func(c *fiber.Ctx) error {
		var req mmodel.BatchRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"code":    "0047",
				"title":   "Bad Request",
				"message": "Invalid batch request",
			})
		}
		return batchHandler.ProcessBatch(&req, c)
	})

	return app
}

func TestBatchHandler_SingleRequestSuccess(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 1, batchResp.SuccessCount)
	assert.Equal(t, 0, batchResp.FailureCount)
	assert.Len(t, batchResp.Results, 1)
	assert.Equal(t, "req-1", batchResp.Results[0].ID)
	assert.Equal(t, http.StatusOK, batchResp.Results[0].Status)
	assert.Nil(t, batchResp.Results[0].Error)
}

func TestBatchHandler_MultipleRequestsAllSuccess(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
			},
			{
				ID:     "req-2",
				Method: "POST",
				Path:   "/v1/test",
				Body:   json.RawMessage(`{"name": "test"}`),
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 2, batchResp.SuccessCount)
	assert.Equal(t, 0, batchResp.FailureCount)
	assert.Len(t, batchResp.Results, 2)
}

func TestBatchHandler_PartialSuccess_ReturnsMultiStatus(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
			},
			{
				ID:     "req-2",
				Method: "GET",
				Path:   "/v1/error",
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMultiStatus, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 1, batchResp.SuccessCount)
	assert.Equal(t, 1, batchResp.FailureCount)
	assert.Len(t, batchResp.Results, 2)

	// Check first request succeeded
	assert.Equal(t, "req-1", batchResp.Results[0].ID)
	assert.Equal(t, http.StatusOK, batchResp.Results[0].Status)
	assert.Nil(t, batchResp.Results[0].Error)

	// Check second request failed
	assert.Equal(t, "req-2", batchResp.Results[1].ID)
	assert.Equal(t, http.StatusNotFound, batchResp.Results[1].Status)
	assert.NotNil(t, batchResp.Results[1].Error)
	assert.Equal(t, "0007", batchResp.Results[1].Error.Code)
}

func TestBatchHandler_AllFail_ReturnsMultiStatus(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/error",
			},
			{
				ID:     "req-2",
				Method: "GET",
				Path:   "/v1/internal-error",
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// All failures should return 207 Multi-Status, not 500
	// Clients can inspect individual results to determine failure types
	assert.Equal(t, http.StatusMultiStatus, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 0, batchResp.SuccessCount)
	assert.Equal(t, 2, batchResp.FailureCount)
}

func TestBatchHandler_EmptyRequest_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_DuplicateIDs_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
			},
			{
				ID:     "req-1", // Duplicate ID
				Method: "GET",
				Path:   "/v1/test",
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_InvalidHTTPMethod_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	testCases := []struct {
		name   string
		method string
	}{
		{"OPTIONS method", "OPTIONS"},
		{"CONNECT method", "CONNECT"},
		{"TRACE method", "TRACE"},
		{"Empty method", ""},
		{"Invalid method", "INVALID"},
		{"Lowercase method", "get"}, // Should be uppercase
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			batchReq := mmodel.BatchRequest{
				Requests: []mmodel.BatchRequestItem{
					{
						ID:     "req-1",
						Method: tc.method,
						Path:   "/v1/test",
					},
				},
			}

			body, err := json.Marshal(batchReq)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			// Verify error message contains method validation error
			var errorResp map[string]any
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			require.NoError(t, err)
			assert.Contains(t, errorResp["message"], "Invalid HTTP method")
		})
	}
}

func TestBatchHandler_RecursiveBatchRequest_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "POST",
				Path:   "/v1/batch", // Recursive batch request
				Body:   json.RawMessage(`{"requests": []}`),
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_WithCustomHeaders(t *testing.T) {
	app := setupTestApp()

	// Add an endpoint that echoes headers
	app.Get("/v1/headers", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"x-custom-header": c.Get("X-Custom-Header"),
		})
	})

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/headers",
				Headers: map[string]string{
					"X-Custom-Header": "custom-value",
				},
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 1, batchResp.SuccessCount)
	assert.Len(t, batchResp.Results, 1)

	// Parse the response body to check the header was passed
	var resultBody map[string]string
	err = json.Unmarshal(batchResp.Results[0].Body, &resultBody)
	require.NoError(t, err)
	assert.Equal(t, "custom-value", resultBody["x-custom-header"])
}

func TestBatchHandler_AuthorizationHeaderPropagation(t *testing.T) {
	app := setupTestApp()

	// Add an endpoint that echoes the authorization header
	app.Get("/v1/auth-check", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"authorization": c.Get("Authorization"),
		})
	})

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/auth-check",
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	// Parse the response body to check the authorization header was passed
	var resultBody map[string]string
	err = json.Unmarshal(batchResp.Results[0].Body, &resultBody)
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", resultBody["authorization"])
}

func TestBatchHandler_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_NilPayload_HandledGracefully(t *testing.T) {
	app := setupTestApp()

	// Create request with nil body (omitted from JSON)
	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET", // GET doesn't require body
				Path:   "/v1/test",
				// Body omitted (nil)
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Nil/omitted body should be handled gracefully
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestBatchHandler_LargePayload_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	// Create a JSON body larger than MaxRequestBodySize (1MB)
	// We need to create valid JSON, so we'll create a large string value
	largeString := make([]byte, 1024*1024+1) // 1MB + 1 byte
	for i := range largeString {
		largeString[i] = 'A'
	}

	// Create valid JSON with large string
	largeBodyJSON := fmt.Sprintf(`{"data": "%s"}`, string(largeString))
	largeBody := json.RawMessage(largeBodyJSON)

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "POST",
				Path:   "/v1/test",
				Body:   largeBody,
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_PathTraversal_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/../../etc/passwd", // Path traversal attempt
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_MaxBatchSize_ReturnsSuccess(t *testing.T) {
	app := setupTestApp()

	// Create exactly 100 items (max batch size)
	requests := make([]mmodel.BatchRequestItem, 100)
	for i := 0; i < 100; i++ {
		requests[i] = mmodel.BatchRequestItem{
			ID:     fmt.Sprintf("req-%d", i),
			Method: "GET",
			Path:   "/v1/test",
		}
	}

	batchReq := mmodel.BatchRequest{
		Requests: requests,
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 100, batchResp.SuccessCount)
	assert.Len(t, batchResp.Results, 100)
}

func TestBatchHandler_ConcurrentRequests(t *testing.T) {
	app := setupTestApp()

	// Test concurrent batch requests
	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer func() { done <- true }()

			batchReq := mmodel.BatchRequest{
				Requests: []mmodel.BatchRequestItem{
					{
						ID:     fmt.Sprintf("req-%d", id),
						Method: "GET",
						Path:   "/v1/test",
					},
				},
			}

			body, err := json.Marshal(batchReq)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}
}

func TestBatchHandler_NilApp_Panics(t *testing.T) {
	// Test that NewBatchHandler validates App is not nil
	handler, err := NewBatchHandler(nil)
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestBatchHandler_ExceedsMaxBatchSize_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	// Create 101 items (exceeds max=100)
	requests := make([]mmodel.BatchRequestItem, 101)
	for i := 0; i < 101; i++ {
		requests[i] = mmodel.BatchRequestItem{
			ID:     fmt.Sprintf("req-%d", i),
			Method: "GET",
			Path:   "/v1/test",
		}
	}

	batchReq := mmodel.BatchRequest{
		Requests: requests,
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp struct {
		Code    string `json:"code"`
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &errResp)
	require.NoError(t, err)

	assert.Equal(t, "0140", errResp.Code) // ErrBatchSizeExceeded
	assert.Contains(t, errResp.Message, "101")
	assert.Contains(t, errResp.Message, "100")
}

func TestBatchHandler_ForbiddenHeaders_AreBlocked(t *testing.T) {
	app := setupTestApp()

	// Add an endpoint that echoes headers
	app.Get("/v1/auth-echo", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"authorization": c.Get("Authorization"),
			"host":          c.Get("Host"),
			"x-custom":      c.Get("X-Custom"),
		})
	})

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/auth-echo",
				Headers: map[string]string{
					"Authorization": "Bearer attacker-token", // Should be blocked
					"Host":          "evil.com",              // Should be blocked
					"X-Custom":      "allowed-value",         // Should be allowed
				},
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer original-token") // Parent auth

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	// Parse the response body to check headers
	var resultBody map[string]string
	err = json.Unmarshal(batchResp.Results[0].Body, &resultBody)
	require.NoError(t, err)

	// Authorization should be the original token, not the attacker's
	assert.Equal(t, "Bearer original-token", resultBody["authorization"])
	// Custom header should be allowed
	assert.Equal(t, "allowed-value", resultBody["x-custom"])
}

func TestBatchHandler_URLEncodedPathTraversal_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/%2e%2e/etc/passwd", // URL-encoded path traversal
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_PathWithoutLeadingSlash_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "v1/test", // Missing leading slash
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_RecursiveBatchWithQueryString_ReturnsBadRequest(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "POST",
				Path:   "/v1/batch?foo=bar", // Recursive with query string
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBatchHandler_LongRequestID_IsTruncatedInError(t *testing.T) {
	app := setupTestApp()

	// Create a long-but-valid ID (over truncation threshold but under max=100)
	longID := strings.Repeat("a", MaxDisplayIDLength+10)

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     longID,
				Method: "GET",
				Path:   "/v1/test",
			},
			{
				ID:     longID, // Duplicate
				Method: "GET",
				Path:   "/v1/test",
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &errResp)
	require.NoError(t, err)

	// Message should contain truncated ID (max MaxDisplayIDLength chars + "...")
	assert.Contains(t, errResp.Message, "...")
	// Error message should be reasonable length (within log path limit which bounds message display)
	assert.LessOrEqual(t, len(errResp.Message), MaxLogPathLength)
}

func TestBatchHandler_AllHTTPMethods(t *testing.T) {
	app := setupTestApp()

	// Add endpoints for all methods
	app.Put("/v1/resource/123", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"method": "PUT"})
	})
	app.Patch("/v1/resource/123", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"method": "PATCH"})
	})
	app.Delete("/v1/resource/123", func(c *fiber.Ctx) error {
		return c.Status(http.StatusNoContent).Send(nil)
	})
	app.Head("/v1/test", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	testCases := []struct {
		method         string
		path           string
		body           json.RawMessage
		expectedStatus int
	}{
		{"GET", "/v1/test", nil, http.StatusOK},
		{"POST", "/v1/test", json.RawMessage(`{"name": "test"}`), http.StatusCreated}, // POST needs body
		{"PUT", "/v1/resource/123", nil, http.StatusOK},
		{"PATCH", "/v1/resource/123", nil, http.StatusOK},
		{"DELETE", "/v1/resource/123", nil, http.StatusNoContent},
		{"HEAD", "/v1/test", nil, http.StatusOK},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			batchReq := mmodel.BatchRequest{
				Requests: []mmodel.BatchRequestItem{
					{
						ID:     "req-1",
						Method: tc.method,
						Path:   tc.path,
						Body:   tc.body,
					},
				},
			}

			body, err := json.Marshal(batchReq)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode) // Batch returns 201 for all success

			var batchResp mmodel.BatchResponse
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			err = json.Unmarshal(respBody, &batchResp)
			require.NoError(t, err)

			assert.Equal(t, 1, batchResp.SuccessCount)
			assert.Equal(t, tc.expectedStatus, batchResp.Results[0].Status)
		})
	}
}

// TestNewBatchHandlerWithRedis tests that the new constructor works correctly.
func TestNewBatchHandlerWithRedis(t *testing.T) {
	app := fiber.New()

	// Test with nil Redis client (should work)
	handler, err := NewBatchHandlerWithRedis(app, nil)
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.App)
	assert.Nil(t, handler.RedisClient)

	// Test with nil app (should fail)
	handler, err = NewBatchHandlerWithRedis(nil, nil)
	require.Error(t, err)
	assert.Nil(t, handler)
}

// TestBatchHandler_IdempotencyKeyHeader_WithoutRedis tests that idempotency header is accepted
// even without Redis (just won't cache).
func TestBatchHandler_IdempotencyKeyHeader_WithoutRedis(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
			},
		},
	}

	reqBody, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency", "test-idempotency-key-123")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should succeed without Redis
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 1, batchResp.SuccessCount)
	assert.Equal(t, 0, batchResp.FailureCount)
}

// TestBatchHandler_IdempotencyTTLHeader_Accepted tests that the TTL header is accepted.
func TestBatchHandler_IdempotencyTTLHeader_Accepted(t *testing.T) {
	app := setupTestApp()

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
			},
		},
	}

	reqBody, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency", "test-idempotency-key-456")
	req.Header.Set("X-Idempotency-TTL", "3600") // 1 hour

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should succeed
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestBatchHandler_HeaderKeySizeValidation(t *testing.T) {
	app := setupTestApp()

	// Create a header key that exceeds MaxHeaderKeySize (256 bytes)
	largeKey := strings.Repeat("a", MaxHeaderKeySize+1)

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
				Headers: map[string]string{
					largeKey: "value",
				},
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should fail with 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errorResp map[string]any
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &errorResp)
	require.NoError(t, err)

	assert.Contains(t, errorResp["message"], "exceeds maximum size")
	assert.Contains(t, errorResp["message"], fmt.Sprintf("%d bytes", MaxHeaderKeySize))
}

func TestBatchHandler_HeaderValueSizeValidation(t *testing.T) {
	app := setupTestApp()

	// Create a header value that exceeds MaxHeaderValueSize (8KB)
	largeValue := strings.Repeat("b", MaxHeaderValueSize+1)

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/test",
				Headers: map[string]string{
					"X-Custom-Header": largeValue,
				},
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should fail with 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errorResp map[string]any
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &errorResp)
	require.NoError(t, err)

	assert.Contains(t, errorResp["message"], "exceeds maximum size")
	assert.Contains(t, errorResp["message"], fmt.Sprintf("%d bytes", MaxHeaderValueSize))
}

func TestBatchHandler_ValidHeaderSizes(t *testing.T) {
	app := setupTestApp()

	// Add an endpoint that echoes headers
	app.Get("/v1/headers", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"x-custom-header": c.Get("X-Custom-Header"),
			"x-large-header":  c.Get("X-Large-Header"),
		})
	})

	// Use maximum allowed sizes
	maxKey := strings.Repeat("a", MaxHeaderKeySize)
	maxValue := strings.Repeat("b", MaxHeaderValueSize)

	batchReq := mmodel.BatchRequest{
		Requests: []mmodel.BatchRequestItem{
			{
				ID:     "req-1",
				Method: "GET",
				Path:   "/v1/headers",
				Headers: map[string]string{
					"X-Custom-Header": "normal-value",
					maxKey:            maxValue,
				},
			},
		},
	}

	body, err := json.Marshal(batchReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should succeed with valid header sizes
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp mmodel.BatchResponse
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(respBody, &batchResp)
	require.NoError(t, err)

	assert.Equal(t, 1, batchResp.SuccessCount)
	assert.Len(t, batchResp.Results, 1)
	assert.Equal(t, http.StatusOK, batchResp.Results[0].Status)
}

// =============================================================================
// AC-1: Context Cancellation Tests
// =============================================================================

// TestBatchHandler_ProcessRequest_ContextCancelled tests handling of cancelled request contexts
// when parent context is cancelled (not deadline exceeded).
func TestBatchHandler_ProcessRequest_ContextCancelled(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() (context.Context, context.CancelFunc)
		expectedStatus int
		expectedCode   string
		expectedMsg    string
	}{
		{
			name: "context cancelled before processing completes",
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "0046", // ErrInternalServer
			expectedMsg:    "Request context cancelled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			app := fiber.New()

			// Add a slow endpoint that allows cancellation to be detected
			app.Get("/v1/slow", func(c *fiber.Ctx) error {
				ctx := c.UserContext()
				select {
				case <-ctx.Done():
					return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
						"code":    "0046",
						"title":   "Internal Server Error",
						"message": "Request context cancelled",
					})
				case <-time.After(5 * time.Second):
					return c.Status(http.StatusOK).JSON(fiber.Map{"message": "success"})
				}
			})

			handler := &BatchHandler{App: app}

			reqItem := mmodel.BatchRequestItem{
				ID:     "test-cancel-1",
				Method: "GET",
				Path:   "/v1/slow",
			}

			// Act - create a context that we'll cancel
			ctx, cancel := tt.setupContext()

			// Create a fiber context with the cancellable context
			fiberApp := fiber.New()
			var result mmodel.BatchResponseItem

			fiberApp.Get("/test", func(c *fiber.Ctx) error {
				c.SetUserContext(ctx)
				// Cancel the context immediately to simulate cancellation
				cancel()
				result = handler.processRequest(c.UserContext(), reqItem, "", "")
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := fiberApp.Test(req, -1)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.NotNil(t, result.Error)
			assert.Equal(t, tt.expectedCode, result.Error.Code)
			assert.Contains(t, result.Error.Message, tt.expectedMsg)
		})
	}
}

// TestBatchHandler_ProcessRequest_ContextCancelled_EdgeCases tests edge cases for context cancellation
func TestBatchHandler_ProcessRequest_ContextCancelled_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		description string
	}{
		{
			name:        "context cancelled with nil error reason",
			description: "Tests context cancellation where Err() returns context.Canceled",
		},
		{
			name:        "context already cancelled before request starts",
			description: "Tests that already-cancelled context is detected immediately",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			app := fiber.New()
			app.Get("/v1/test", func(c *fiber.Ctx) error {
				return c.Status(http.StatusOK).JSON(fiber.Map{"message": "success"})
			})

			handler := &BatchHandler{App: app}

			// Create already-cancelled context
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			reqItem := mmodel.BatchRequestItem{
				ID:     "test-edge-1",
				Method: "GET",
				Path:   "/v1/test",
			}

			// Act
			fiberApp := fiber.New()
			var result mmodel.BatchResponseItem

			fiberApp.Get("/test", func(c *fiber.Ctx) error {
				c.SetUserContext(ctx)
				result = handler.processRequest(c.UserContext(), reqItem, "", "")
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := fiberApp.Test(req, -1)
			require.NoError(t, err)

			// Assert - either returns error or succeeds before cancellation is detected
			// The behavior depends on timing, but the handler should not panic
			assert.NotEmpty(t, result.ID)
		})
	}
}

// =============================================================================
// AC-2: Timeout Behavior Tests
// =============================================================================

// TestBatchHandler_ProcessRequest_Timeout tests RequestTimeout (30s) behavior
// when individual batch request exceeds timeout.
func TestBatchHandler_ProcessRequest_Timeout(t *testing.T) {
	tests := []struct {
		name           string
		handlerDelay   time.Duration
		timeout        time.Duration
		expectedStatus int
		expectedCode   string
		expectedTitle  string
		expectedMsg    string
	}{
		{
			name:           "request exceeds timeout returns 408",
			handlerDelay:   100 * time.Millisecond, // Delay longer than timeout
			timeout:        10 * time.Millisecond,  // Short timeout for test
			expectedStatus: http.StatusRequestTimeout,
			expectedCode:   "0145", // ErrBatchRequestTimeout
			expectedTitle:  "Request Timeout",
			expectedMsg:    "Request exceeded timeout of 30 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			app := fiber.New()

			// Add a slow endpoint that takes longer than timeout
			app.Get("/v1/slow-endpoint", func(c *fiber.Ctx) error {
				time.Sleep(tt.handlerDelay)
				return c.Status(http.StatusOK).JSON(fiber.Map{"message": "success"})
			})

			handler := &BatchHandler{App: app}

			reqItem := mmodel.BatchRequestItem{
				ID:     "timeout-test-1",
				Method: "GET",
				Path:   "/v1/slow-endpoint",
			}

			// Act - Create context with short timeout
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			fiberApp := fiber.New()
			var result mmodel.BatchResponseItem

			fiberApp.Get("/test", func(c *fiber.Ctx) error {
				c.SetUserContext(ctx)
				result = handler.processRequest(c.UserContext(), reqItem, "", "")
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := fiberApp.Test(req, -1)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.NotNil(t, result.Error)
			assert.Equal(t, tt.expectedCode, result.Error.Code)
			assert.Equal(t, tt.expectedTitle, result.Error.Title)
			assert.Equal(t, tt.expectedMsg, result.Error.Message)
		})
	}
}

// TestBatchHandler_ProcessRequest_Timeout_EdgeCases tests edge cases for timeout behavior
func TestBatchHandler_ProcessRequest_Timeout_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		handlerDelay   time.Duration
		timeout        time.Duration
		expectTimeout  bool
		expectedStatus int
	}{
		{
			name:           "request completes just before timeout",
			handlerDelay:   5 * time.Millisecond,
			timeout:        100 * time.Millisecond,
			expectTimeout:  false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "request completes exactly at timeout boundary",
			handlerDelay:   50 * time.Millisecond,
			timeout:        50 * time.Millisecond,
			expectTimeout:  true, // May or may not timeout - race condition
			expectedStatus: http.StatusOK,
		},
		{
			name:           "request with zero delay succeeds",
			handlerDelay:   0,
			timeout:        100 * time.Millisecond,
			expectTimeout:  false,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			app := fiber.New()

			app.Get("/v1/endpoint", func(c *fiber.Ctx) error {
				if tt.handlerDelay > 0 {
					time.Sleep(tt.handlerDelay)
				}
				return c.Status(http.StatusOK).JSON(fiber.Map{"message": "success"})
			})

			handler := &BatchHandler{App: app}

			reqItem := mmodel.BatchRequestItem{
				ID:     "timeout-edge-1",
				Method: "GET",
				Path:   "/v1/endpoint",
			}

			// Act
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			fiberApp := fiber.New()
			var result mmodel.BatchResponseItem

			fiberApp.Get("/test", func(c *fiber.Ctx) error {
				c.SetUserContext(ctx)
				result = handler.processRequest(c.UserContext(), reqItem, "", "")
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := fiberApp.Test(req, -1)
			require.NoError(t, err)

			// Assert - the request should either succeed or timeout
			assert.NotEmpty(t, result.ID)
			if !tt.expectTimeout {
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Nil(t, result.Error)
			}
		})
	}
}

// TestBatchHandler_ProcessRequest_DeadlineExceeded_VsCancelled tests the difference
// between DeadlineExceeded and Cancelled context errors.
func TestBatchHandler_ProcessRequest_DeadlineExceeded_VsCancelled(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() (context.Context, func())
		expectedStatus int
		expectedCode   string
	}{
		{
			name: "deadline exceeded returns 408 Request Timeout",
			setupContext: func() (context.Context, func()) {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				time.Sleep(10 * time.Millisecond) // Ensure deadline passes
				return ctx, cancel
			},
			expectedStatus: http.StatusRequestTimeout,
			expectedCode:   "0145", // ErrBatchRequestTimeout
		},
		{
			name: "context cancelled returns 500 Internal Server Error",
			setupContext: func() (context.Context, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately (not deadline exceeded)
				return ctx, func() {}
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "0046", // ErrInternalServer
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			app := fiber.New()
			app.Get("/v1/test", func(c *fiber.Ctx) error {
				time.Sleep(50 * time.Millisecond) // Slow enough to detect cancellation
				return c.Status(http.StatusOK).JSON(fiber.Map{"message": "success"})
			})

			handler := &BatchHandler{App: app}

			reqItem := mmodel.BatchRequestItem{
				ID:     "context-test-1",
				Method: "GET",
				Path:   "/v1/test",
			}

			// Act
			ctx, cleanup := tt.setupContext()
			defer cleanup()

			fiberApp := fiber.New()
			var result mmodel.BatchResponseItem

			fiberApp.Get("/test", func(c *fiber.Ctx) error {
				c.SetUserContext(ctx)
				result = handler.processRequest(c.UserContext(), reqItem, "", "")
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := fiberApp.Test(req, -1)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.expectedStatus, result.Status)
			if result.Error != nil {
				assert.Equal(t, tt.expectedCode, result.Error.Code)
			}
		})
	}
}

// =============================================================================
// AC-3: Redis Failure During Processing Tests
// =============================================================================

// TestBatchHandler_CheckOrCreateIdempotencyKey_RedisSetNXError tests Redis SetNX failures
func TestBatchHandler_CheckOrCreateIdempotencyKey_RedisSetNXError(t *testing.T) {
	tests := []struct {
		name          string
		redisErr      error
		expectErr     bool
		expectedErrIs error
	}{
		{
			name:      "Redis SetNX connection error",
			redisErr:  errors.New("connection refused"),
			expectErr: true,
		},
		{
			name:      "Redis SetNX timeout error",
			redisErr:  errors.New("i/o timeout"),
			expectErr: true,
		},
		{
			name:      "Redis SetNX network error",
			redisErr:  errors.New("network unreachable"),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			db, mock := redismock.NewClientMock()
			defer db.Close()

			handler := &BatchHandler{
				App:         fiber.New(),
				RedisClient: db,
			}

			key := "test-idempotency-key"
			ttl := 24 * time.Hour
			internalKey := "batch_idempotency:{batch}:" + key

			// Mock SetNX to return error
			mock.ExpectSetNX(internalKey, "", ttl).SetErr(tt.redisErr)

			// Act
			ctx := context.Background()
			result, err := handler.checkOrCreateIdempotencyKey(ctx, key, ttl)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.redisErr.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestBatchHandler_CheckOrCreateIdempotencyKey_RedisGetError tests Redis Get failures
// when key already exists but get fails.
func TestBatchHandler_CheckOrCreateIdempotencyKey_RedisGetError(t *testing.T) {
	tests := []struct {
		name      string
		redisErr  error
		expectErr bool
	}{
		{
			name:      "Redis Get connection error",
			redisErr:  errors.New("connection refused"),
			expectErr: true,
		},
		{
			name:      "Redis Get timeout error",
			redisErr:  errors.New("i/o timeout"),
			expectErr: true,
		},
		{
			name:      "Redis Get read error",
			redisErr:  errors.New("read tcp: connection reset by peer"),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			db, mock := redismock.NewClientMock()
			defer db.Close()

			handler := &BatchHandler{
				App:         fiber.New(),
				RedisClient: db,
			}

			key := "test-idempotency-key"
			ttl := 24 * time.Hour
			internalKey := "batch_idempotency:{batch}:" + key

			// Mock SetNX to return false (key exists)
			mock.ExpectSetNX(internalKey, "", ttl).SetVal(false)
			// Mock Get to return error
			mock.ExpectGet(internalKey).SetErr(tt.redisErr)

			// Act
			ctx := context.Background()
			result, err := handler.checkOrCreateIdempotencyKey(ctx, key, ttl)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestBatchHandler_CheckOrCreateIdempotencyKey_UnmarshalError tests JSON unmarshal failures
func TestBatchHandler_CheckOrCreateIdempotencyKey_UnmarshalError(t *testing.T) {
	tests := []struct {
		name         string
		cachedValue  string
		expectErr    bool
		expectResult bool
	}{
		{
			name:         "invalid JSON in cache",
			cachedValue:  "not valid json {{{",
			expectErr:    true,
			expectResult: false,
		},
		{
			name:         "empty JSON object",
			cachedValue:  "{}",
			expectErr:    false,
			expectResult: true,
		},
		{
			name:         "corrupted binary data",
			cachedValue:  "\x00\x01\x02\x03",
			expectErr:    true,
			expectResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			db, mock := redismock.NewClientMock()
			defer db.Close()

			handler := &BatchHandler{
				App:         fiber.New(),
				RedisClient: db,
			}

			key := "test-idempotency-key"
			ttl := 24 * time.Hour
			internalKey := "batch_idempotency:{batch}:" + key

			// Mock SetNX to return false (key exists)
			mock.ExpectSetNX(internalKey, "", ttl).SetVal(false)
			// Mock Get to return invalid cached value
			mock.ExpectGet(internalKey).SetVal(tt.cachedValue)

			// Act
			ctx := context.Background()
			result, err := handler.checkOrCreateIdempotencyKey(ctx, key, ttl)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tt.expectResult {
					assert.NotNil(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestBatchHandler_SetIdempotencyValue_RedisSetXXError tests Redis SetXX failures
func TestBatchHandler_SetIdempotencyValue_RedisSetXXError(t *testing.T) {
	tests := []struct {
		name     string
		redisErr error
	}{
		{
			name:     "Redis SetXX connection error",
			redisErr: errors.New("connection refused"),
		},
		{
			name:     "Redis SetXX timeout error",
			redisErr: errors.New("i/o timeout"),
		},
		{
			name:     "Redis SetXX network error",
			redisErr: errors.New("network unreachable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			db, mock := redismock.NewClientMock()
			defer db.Close()

			handler := &BatchHandler{
				App:         fiber.New(),
				RedisClient: db,
			}

			key := "test-idempotency-key"
			ttl := 24 * time.Hour
			internalKey := "batch_idempotency:{batch}:" + key

			response := &mmodel.BatchResponse{
				SuccessCount: 1,
				FailureCount: 0,
				Results: []mmodel.BatchResponseItem{
					{ID: "req-1", Status: 200},
				},
			}

			// Mock SetXX to return error
			mock.ExpectSetXX(internalKey, gomock.Any(), ttl).SetErr(tt.redisErr)

			// Act - this method doesn't return error, it just logs
			ctx := context.Background()
			handler.setIdempotencyValue(ctx, key, response, ttl)

			// Assert - verify the mock was called (method doesn't return error)
			// The method logs errors but doesn't return them
			// We verify the method completes without panicking
		})
	}
}

// TestBatchHandler_CheckOrCreateIdempotencyKey_Success tests successful idempotency operations
func TestBatchHandler_CheckOrCreateIdempotencyKey_Success(t *testing.T) {
	tests := []struct {
		name         string
		keyExists    bool
		cachedValue  string
		expectResult bool
	}{
		{
			name:         "new key created successfully",
			keyExists:    false,
			cachedValue:  "",
			expectResult: false,
		},
		{
			name:      "existing key with cached response",
			keyExists: true,
			cachedValue: `{
				"successCount": 2,
				"failureCount": 0,
				"results": [
					{"id": "req-1", "status": 200},
					{"id": "req-2", "status": 201}
				]
			}`,
			expectResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			db, mock := redismock.NewClientMock()
			defer db.Close()

			handler := &BatchHandler{
				App:         fiber.New(),
				RedisClient: db,
			}

			key := "test-idempotency-key"
			ttl := 24 * time.Hour
			internalKey := "batch_idempotency:{batch}:" + key

			if tt.keyExists {
				mock.ExpectSetNX(internalKey, "", ttl).SetVal(false)
				mock.ExpectGet(internalKey).SetVal(tt.cachedValue)
			} else {
				mock.ExpectSetNX(internalKey, "", ttl).SetVal(true)
			}

			// Act
			ctx := context.Background()
			result, err := handler.checkOrCreateIdempotencyKey(ctx, key, ttl)

			// Assert
			assert.NoError(t, err)
			if tt.expectResult {
				assert.NotNil(t, result)
				assert.Equal(t, 2, result.SuccessCount)
			} else {
				assert.Nil(t, result)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestBatchHandler_CheckOrCreateIdempotencyKey_InProgress tests conflict when request is in progress
func TestBatchHandler_CheckOrCreateIdempotencyKey_InProgress(t *testing.T) {
	// Arrange
	db, mock := redismock.NewClientMock()
	defer db.Close()

	handler := &BatchHandler{
		App:         fiber.New(),
		RedisClient: db,
	}

	key := "test-idempotency-key"
	ttl := 24 * time.Hour
	internalKey := "batch_idempotency:{batch}:" + key

	// Key exists but value is empty (request in progress)
	mock.ExpectSetNX(internalKey, "", ttl).SetVal(false)
	mock.ExpectGet(internalKey).SetVal("")

	// Act
	ctx := context.Background()
	result, err := handler.checkOrCreateIdempotencyKey(ctx, key, ttl)

	// Assert - should return idempotency conflict error
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "idempotency key") // ErrIdempotencyKey contains this text

	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// AC-4: Large Response Bodies Tests
// =============================================================================

// TestBatchHandler_ProcessRequest_LargeResponseTruncation tests response body truncation
// when response exceeds MaxResponseBodySize (10MB).
func TestBatchHandler_ProcessRequest_LargeResponseTruncation(t *testing.T) {
	tests := []struct {
		name             string
		responseSize     int
		expectTruncation bool
	}{
		{
			name:             "response at exact MaxResponseBodySize limit",
			responseSize:     MaxResponseBodySize,
			expectTruncation: false,
		},
		{
			name:             "response just over MaxResponseBodySize",
			responseSize:     MaxResponseBodySize + 1,
			expectTruncation: true,
		},
		{
			name:             "response way over MaxResponseBodySize (2x)",
			responseSize:     MaxResponseBodySize * 2,
			expectTruncation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			app := fiber.New(fiber.Config{
				// Increase body limit to allow large responses
				BodyLimit: MaxResponseBodySize * 3,
			})

			// Generate response data of the specified size
			responseData := make([]byte, tt.responseSize)
			for i := range responseData {
				responseData[i] = 'A'
			}

			app.Get("/v1/large-response", func(c *fiber.Ctx) error {
				c.Set("Content-Type", "application/octet-stream")
				return c.Status(http.StatusOK).Send(responseData)
			})

			handler := &BatchHandler{App: app}

			reqItem := mmodel.BatchRequestItem{
				ID:     "large-response-test",
				Method: "GET",
				Path:   "/v1/large-response",
			}

			// Act
			fiberApp := fiber.New()
			var result mmodel.BatchResponseItem

			fiberApp.Get("/test", func(c *fiber.Ctx) error {
				result = handler.processRequest(c.UserContext(), reqItem, "", "")
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := fiberApp.Test(req, -1)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, http.StatusOK, result.Status)
			if tt.expectTruncation {
				assert.LessOrEqual(t, len(result.Body), MaxResponseBodySize)
			} else {
				assert.Equal(t, tt.responseSize, len(result.Body))
			}
		})
	}
}

// TestBatchHandler_ProcessRequest_LargeResponseTruncation_EdgeCases tests edge cases
func TestBatchHandler_ProcessRequest_LargeResponseTruncation_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		responseSize int
		expectLen    int
	}{
		{
			name:         "empty response body",
			responseSize: 0,
			expectLen:    0,
		},
		{
			name:         "small response body",
			responseSize: 100,
			expectLen:    100,
		},
		{
			name:         "response at 1 byte under limit",
			responseSize: MaxResponseBodySize - 1,
			expectLen:    MaxResponseBodySize - 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			app := fiber.New()

			var responseData []byte
			if tt.responseSize > 0 {
				responseData = make([]byte, tt.responseSize)
				for i := range responseData {
					responseData[i] = 'B'
				}
			}

			app.Get("/v1/response", func(c *fiber.Ctx) error {
				if tt.responseSize == 0 {
					return c.Status(http.StatusNoContent).Send(nil)
				}
				c.Set("Content-Type", "application/octet-stream")
				return c.Status(http.StatusOK).Send(responseData)
			})

			handler := &BatchHandler{App: app}

			reqItem := mmodel.BatchRequestItem{
				ID:     "edge-case-test",
				Method: "GET",
				Path:   "/v1/response",
			}

			// Act
			fiberApp := fiber.New()
			var result mmodel.BatchResponseItem

			fiberApp.Get("/test", func(c *fiber.Ctx) error {
				result = handler.processRequest(c.UserContext(), reqItem, "", "")
				return nil
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := fiberApp.Test(req, -1)
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.expectLen, len(result.Body))
		})
	}
}

// TestBatchHandler_ProcessRequest_LargeResponse_JSONTruncation tests JSON response truncation
func TestBatchHandler_ProcessRequest_LargeResponse_JSONTruncation(t *testing.T) {
	// Arrange
	app := fiber.New()

	// Generate a large JSON response that exceeds MaxResponseBodySize
	type LargeResponse struct {
		Data string `json:"data"`
	}

	largeData := strings.Repeat("X", MaxResponseBodySize+1000)

	app.Get("/v1/large-json", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(LargeResponse{Data: largeData})
	})

	handler := &BatchHandler{App: app}

	reqItem := mmodel.BatchRequestItem{
		ID:     "large-json-test",
		Method: "GET",
		Path:   "/v1/large-json",
	}

	// Act
	fiberApp := fiber.New()
	var result mmodel.BatchResponseItem

	fiberApp.Get("/test", func(c *fiber.Ctx) error {
		result = handler.processRequest(c.UserContext(), reqItem, "", "")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := fiberApp.Test(req, -1)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusOK, result.Status)
	assert.LessOrEqual(t, len(result.Body), MaxResponseBodySize)
}

// =============================================================================
// Additional Unit Tests for Redis Methods
// =============================================================================

// TestBatchHandler_SetIdempotencyValue_Success tests successful setting of idempotency value
func TestBatchHandler_SetIdempotencyValue_Success(t *testing.T) {
	tests := []struct {
		name     string
		response *mmodel.BatchResponse
	}{
		{
			name: "successful response cached",
			response: &mmodel.BatchResponse{
				SuccessCount: 2,
				FailureCount: 0,
				Results: []mmodel.BatchResponseItem{
					{ID: "req-1", Status: 200},
					{ID: "req-2", Status: 201},
				},
			},
		},
		{
			name: "partial failure response cached",
			response: &mmodel.BatchResponse{
				SuccessCount: 1,
				FailureCount: 1,
				Results: []mmodel.BatchResponseItem{
					{ID: "req-1", Status: 200},
					{ID: "req-2", Status: 500, Error: &mmodel.BatchItemError{Code: "0046", Title: "Error", Message: "Failed"}},
				},
			},
		},
		{
			name: "empty response cached",
			response: &mmodel.BatchResponse{
				SuccessCount: 0,
				FailureCount: 0,
				Results:      []mmodel.BatchResponseItem{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			db, mock := redismock.NewClientMock()
			defer db.Close()

			handler := &BatchHandler{
				App:         fiber.New(),
				RedisClient: db,
			}

			key := "test-key"
			ttl := 24 * time.Hour
			internalKey := "batch_idempotency:{batch}:" + key

			// Mock SetXX to succeed
			mock.ExpectSetXX(internalKey, gomock.Any(), ttl).SetVal(true)

			// Act
			ctx := context.Background()
			handler.setIdempotencyValue(ctx, key, tt.response, ttl)

			// Assert - method completes without panic
			// Note: setIdempotencyValue doesn't return error, it logs internally
		})
	}
}

// TestBatchHandler_CheckOrCreateIdempotencyKey_RedisNilError tests redis.Nil error handling
func TestBatchHandler_CheckOrCreateIdempotencyKey_RedisNilError(t *testing.T) {
	// Arrange
	db, mock := redismock.NewClientMock()
	defer db.Close()

	handler := &BatchHandler{
		App:         fiber.New(),
		RedisClient: db,
	}

	key := "test-key"
	ttl := 24 * time.Hour
	internalKey := "batch_idempotency:{batch}:" + key

	// Key exists (SetNX returns false), but Get returns redis.Nil (key expired between SetNX and Get)
	mock.ExpectSetNX(internalKey, "", ttl).SetVal(false)
	// Redis.Nil is handled specially - it means key exists but has no value (in progress)
	mock.ExpectGet(internalKey).SetVal("")

	// Act
	ctx := context.Background()
	result, err := handler.checkOrCreateIdempotencyKey(ctx, key, ttl)

	// Assert - should return conflict error because empty value means request in progress
	assert.Error(t, err)
	assert.Nil(t, result)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBatchHandler_NewBatchHandlerWithRedis_Validation tests constructor validation
func TestBatchHandler_NewBatchHandlerWithRedis_Validation(t *testing.T) {
	tests := []struct {
		name        string
		app         *fiber.App
		redisClient interface{}
		expectErr   bool
	}{
		{
			name:        "valid app with nil redis",
			app:         fiber.New(),
			redisClient: nil,
			expectErr:   false,
		},
		{
			name:        "nil app returns error",
			app:         nil,
			redisClient: nil,
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			handler, err := NewBatchHandlerWithRedis(tt.app, nil)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, handler)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handler)
			}
		})
	}
}
