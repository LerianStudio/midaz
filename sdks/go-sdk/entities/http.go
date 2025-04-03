package entities

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/LerianStudio/midaz/sdks/go-sdk/internal/utils"
)

// httpClient encapsulates the HTTP functionality used by entities.
// It provides methods for sending HTTP requests, handling responses,
// and managing common headers and error handling.
type httpClient struct {
	client    *http.Client
	authToken string
	userAgent string
	debug     bool
}

// newHTTPClient creates a new HTTP client with the provided configuration.
func newHTTPClient(client *http.Client, authToken string, userAgent string, debug bool) *httpClient {
	return &httpClient{
		client:    client,
		authToken: authToken,
		userAgent: userAgent,
		debug:     debug,
	}
}

// sendRequest sends an HTTP request and decodes the response JSON into the target.
func (h *httpClient) sendRequest(req *http.Request, target any) error {
	// Set common headers
	h.setCommonHeaders(req)

	// Log request if debug is enabled
	if h.debug {
		if err := h.logRequest(req); err != nil {
			return err
		}
	}

	// Send request
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("internal error: %w", err)
	}
	defer resp.Body.Close()

	// Log response if debug is enabled
	if h.debug {
		if err := h.logResponse(resp); err != nil {
			return err
		}
	}

	// Check for error response
	if resp.StatusCode >= 400 {
		return h.handleErrorResponse(resp)
	}

	// If no target is provided, we're done
	if target == nil {
		return nil
	}

	// Decode response body into target
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("internal error: %w", err)
	}

	return nil
}

// setCommonHeaders sets common headers on the request.
func (h *httpClient) setCommonHeaders(req *http.Request) {
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.authToken))
	}

	if req.Header.Get("User-Agent") == "" && h.userAgent != "" {
		req.Header.Set("User-Agent", h.userAgent)
	}
}

// logRequest logs the request details if debug is enabled.
func (h *httpClient) logRequest(req *http.Request) error {
	var bodyStr string

	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("internal error: %w", err)
		}

		req.Body = io.NopCloser(strings.NewReader(string(body)))
		bodyStr = string(body)
	}

	fmt.Printf("[DEBUG] Request: %s %s\n", req.Method, req.URL.String())
	// fmt.Printf("[DEBUG] Headers: %v\n", headers)

	if bodyStr != "" {
		fmt.Printf("[DEBUG] Body: %s\n", bodyStr)
	}

	return nil
}

// logResponse logs the response details if debug is enabled.
func (h *httpClient) logResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("internal error: %w", err)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))

	fmt.Printf("[DEBUG] Response: %d %s\n", resp.StatusCode, resp.Status)
	// fmt.Printf("[DEBUG] Headers: %v\n", resp.Header)
	fmt.Printf("[DEBUG] Body: %s\n", string(body))

	return nil
}

// handleErrorResponse handles error responses from the API.
func (h *httpClient) handleErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read error response: %v", err)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))

	var errorResp struct {
		Error   string `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &errorResp); err == nil {
		errorMsg := errorResp.Message
		if errorMsg == "" {
			errorMsg = errorResp.Error
		}

		if errorResp.Code != "" {
			return h.mapErrorFromCode(errorResp.Code, errorMsg)
		}

		return h.mapErrorFromStatus(resp.StatusCode, errorMsg)
	}

	return utils.NewMidazError(utils.CodeInternal, fmt.Errorf("HTTP error: %d %s - %s", resp.StatusCode, resp.Status, string(body)))
}

// mapErrorFromCode maps API error codes to SDK error types.
func (h *httpClient) mapErrorFromCode(code, message string) error {
	switch code {
	case "validation_error":
		return utils.NewMidazError(utils.CodeValidation, fmt.Errorf("%s", message))
	case "not_found":
		return utils.NewMidazError(utils.CodeNotFound, fmt.Errorf("%s", message))
	case "already_exists":
		return utils.NewMidazError(utils.CodeNotFound, fmt.Errorf("%s", message))
	case "unauthorized":
		return utils.NewMidazError(utils.CodeAuthentication, fmt.Errorf("%s", message))
	case "forbidden":
		return utils.NewMidazError(utils.CodePermission, fmt.Errorf("%s", message))
	case "internal_error":
		return utils.NewMidazError(utils.CodeInternal, fmt.Errorf("%s", message))
	default:
		return utils.NewMidazError(utils.CodeInternal, fmt.Errorf("%s", message))
	}
}

// mapErrorFromStatus maps HTTP status codes to error types.
func (h *httpClient) mapErrorFromStatus(statusCode int, message string) error {
	switch statusCode {
	case http.StatusNotFound:
		return utils.NewMidazError(utils.CodeNotFound, fmt.Errorf("%s", message))
	case http.StatusBadRequest:
		return utils.NewMidazError(utils.CodeValidation, fmt.Errorf("%s", message))
	case http.StatusUnauthorized:
		return utils.NewMidazError(utils.CodeAuthentication, fmt.Errorf("%s", message))
	case http.StatusForbidden:
		return utils.NewMidazError(utils.CodePermission, fmt.Errorf("%s", message))
	case http.StatusTooManyRequests:
		return utils.NewMidazError(utils.CodeRateLimit, fmt.Errorf("%s", message))
	case http.StatusGatewayTimeout:
		return utils.NewMidazError(utils.CodeTimeout, fmt.Errorf("%s", message))
	default:
		return utils.NewMidazError(utils.CodeInternal, fmt.Errorf("%s", message))
	}
}
