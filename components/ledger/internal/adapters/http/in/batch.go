package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkghttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
)

const (
	// MaxRequestBodySize is the maximum size for a single request body in a batch (1MB)
	MaxRequestBodySize = 1024 * 1024
	// MaxResponseBodySize is the maximum size for a single response body in a batch (10MB)
	MaxResponseBodySize = 10 * 1024 * 1024
	// RequestTimeout is the timeout for individual batch requests (30 seconds)
	RequestTimeout = 30 * time.Second
	// MaxBatchItems is the maximum number of items allowed in a single batch request
	MaxBatchItems = 100
	// MaxBatchWorkers is the maximum number of concurrent workers for parallel batch processing
	MaxBatchWorkers = 10
	// MaxHeaderKeySize is the maximum size for an HTTP header key (256 bytes)
	MaxHeaderKeySize = 256
	// MaxHeaderValueSize is the maximum size for an HTTP header value (8KB)
	MaxHeaderValueSize = 8 * 1024
	// MaxDisplayIDLength is the maximum length for IDs displayed in error messages.
	// Longer IDs are truncated with "..." to prevent log injection attacks.
	MaxDisplayIDLength = 50
	// MaxLogPathLength is the maximum length for paths in log messages.
	// Longer paths are truncated with "..." to prevent log injection or storage issues.
	MaxLogPathLength = 200
)

// forbiddenHeaders contains headers that cannot be overridden by batch request items.
// These headers are security-critical and must be inherited from the parent batch request.
var forbiddenHeaders = map[string]bool{
	"authorization":     true,
	"host":              true,
	"content-length":    true,
	"transfer-encoding": true,
	"connection":        true,
	"x-forwarded-for":   true,
	"x-forwarded-host":  true,
	"x-forwarded-proto": true,
	"x-real-ip":         true,
	"x-request-id":      true, // Already set from parent request with item ID appended
	"cookie":            true,
	"set-cookie":        true,
}

// isValidMethod checks if the provided HTTP method is valid for batch requests.
// Valid methods are: GET, POST, PUT, PATCH, DELETE, HEAD
func isValidMethod(method string) bool {
	validMethods := map[string]bool{
		"GET":    true,
		"POST":   true,
		"PUT":    true,
		"PATCH":  true,
		"DELETE": true,
		"HEAD":   true,
	}
	return validMethods[method]
}

// propagateRequestID sets the X-Request-Id header in the response if it exists in the request.
// This ensures error responses include request ID for tracing and debugging.
func propagateRequestID(c *fiber.Ctx) {
	if requestID := c.Get("X-Request-Id"); requestID != "" {
		c.Set("X-Request-Id", requestID)
	}
}

// BatchHandler handles batch API requests.
type BatchHandler struct {
	// App is a reference to the Fiber app for internal routing
	App *fiber.App
	// RedisClient for idempotency support (optional)
	RedisClient *redis.Client
}

// NewBatchHandler creates a new BatchHandler with validation.
func NewBatchHandler(app *fiber.App) (*BatchHandler, error) {
	if app == nil {
		return nil, pkg.ValidateInternalError(fiber.ErrBadRequest, "Fiber app cannot be nil")
	}

	return &BatchHandler{App: app}, nil
}

// NewBatchHandlerWithRedis creates a new BatchHandler with Redis support for idempotency.
func NewBatchHandlerWithRedis(app *fiber.App, redisClient *redis.Client) (*BatchHandler, error) {
	if app == nil {
		return nil, pkg.ValidateInternalError(fiber.ErrBadRequest, "Fiber app cannot be nil")
	}

	return &BatchHandler{
		App:         app,
		RedisClient: redisClient,
	}, nil
}

// ProcessBatch processes a batch of API requests.
//
//	@Summary		Process batch API requests
//	@Description	Processes multiple API requests in a single HTTP call. Returns 201 if all succeed, 207 Multi-Status for any failures (all fail or mixed). Clients should inspect individual results to determine failure types. Supports idempotency via X-Idempotency header to prevent duplicate processing.
//	@Tags			Batch
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header		string					false	"Request ID for tracing"
//	@Param			X-Idempotency		header		string					false	"Idempotency key to prevent duplicate batch processing"
//	@Param			X-Idempotency-TTL	header		int						false	"TTL in seconds for idempotency cache (default: 86400)"
//	@Param			batch				body		mmodel.BatchRequest		true	"Batch request containing multiple API requests"
//	@Success		201					{object}	mmodel.BatchResponse	"All requests succeeded"
//	@Success		207					{object}	mmodel.BatchResponse	"Any failures occurred (all fail or mixed). Inspect individual results for failure details."
//	@Failure		400					{object}	mmodel.Error			"Invalid batch request"
//	@Failure		401					{object}	mmodel.Error			"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error			"Forbidden access"
//	@Failure		409					{object}	mmodel.Error			"Idempotency key conflict - request already in progress"
//	@Failure		429					{object}	mmodel.Error			"Rate limit exceeded"
//	@Router			/v1/batch [post]
func (h *BatchHandler) ProcessBatch(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.process_batch")
	defer span.End()

	// Validate handler state
	if h.App == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "BatchHandler.App is nil", constant.ErrInternalServer)
		propagateRequestID(c)

		return pkghttp.InternalServerError(c, constant.ErrInternalServer.Error(), "Internal Server Error", "Batch handler not properly initialized")
	}

	// Safe type assertion with ok check for defense in depth
	payload, ok := p.(*mmodel.BatchRequest)
	if !ok {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid payload type", constant.ErrInternalServer)
		propagateRequestID(c)

		return pkghttp.InternalServerError(c, constant.ErrInternalServer.Error(), "Internal Server Error", "Invalid payload type received")
	}

	logger.Infof("Processing batch request with %d items", len(payload.Requests))

	// Handle idempotency if Redis is available and idempotency key is provided
	idempotencyKey, idempotencyTTL := pkghttp.GetIdempotencyKeyAndTTL(c)
	if idempotencyKey != "" && h.RedisClient != nil {
		ctxIdempotency, spanIdempotency := tracer.Start(ctx, "handler.process_batch_idempotency")

		cachedResponse, err := h.checkOrCreateIdempotencyKey(ctxIdempotency, idempotencyKey, idempotencyTTL)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanIdempotency, "Idempotency key conflict", err)
			spanIdempotency.End()

			logger.Warnf("Idempotency key conflict for key: %s - %v", idempotencyKey, err)
			propagateRequestID(c)

			return pkghttp.WithError(c, err)
		}

		if cachedResponse != nil {
			// Return cached response
			logger.Infof("Returning cached batch response for idempotency key: %s", idempotencyKey)
			spanIdempotency.End()

			c.Set(libConstants.IdempotencyReplayed, "true")
			propagateRequestID(c)

			// Determine status code from cached response
			statusCode := http.StatusCreated
			if cachedResponse.FailureCount > 0 {
				statusCode = http.StatusMultiStatus
			}

			return c.Status(statusCode).JSON(cachedResponse)
		}

		spanIdempotency.End()
		c.Set(libConstants.IdempotencyReplayed, "false")
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.batch_size", len(payload.Requests))
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set span attributes", err)
	}

	// Validate batch request
	if len(payload.Requests) == 0 {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Empty batch request", constant.ErrInvalidBatchRequest)
		propagateRequestID(c)

		return pkghttp.BadRequest(c, pkg.ValidationError{
			Code:    constant.ErrInvalidBatchRequest.Error(),
			Title:   "Invalid Batch Request",
			Message: "Batch request must contain at least one request item",
		})
	}

	// Validate max batch size (defense in depth - rate limiter may be disabled)
	if len(payload.Requests) > MaxBatchItems {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Batch size exceeded", constant.ErrBatchSizeExceeded)
		propagateRequestID(c)

		return pkghttp.BadRequest(c, pkg.ValidationError{
			Code:    constant.ErrBatchSizeExceeded.Error(),
			Title:   "Invalid Batch Request",
			Message: fmt.Sprintf("Batch size %d exceeds maximum allowed size of %d", len(payload.Requests), MaxBatchItems),
		})
	}

	// Check for duplicate IDs
	idSet := make(map[string]bool)

	for _, req := range payload.Requests {
		if idSet[req.ID] {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Duplicate request ID", constant.ErrDuplicateBatchRequestID)

			// Truncate ID for error message to prevent log injection
			displayID := req.ID
			if len(displayID) > MaxDisplayIDLength {
				displayID = displayID[:MaxDisplayIDLength] + "..."
			}
			propagateRequestID(c)

			return pkghttp.BadRequest(c, pkg.ValidationError{
				Code:    constant.ErrDuplicateBatchRequestID.Error(),
				Title:   "Invalid Batch Request",
				Message: fmt.Sprintf("Duplicate request ID found: %s", displayID),
			})
		}

		idSet[req.ID] = true
	}

	// Validate paths don't include the batch endpoint itself (prevent recursion)
	// Also validate for path traversal attacks
	for _, req := range payload.Requests {
		// Validate HTTP method
		if !isValidMethod(req.Method) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid HTTP method", constant.ErrInvalidBatchRequest)
			propagateRequestID(c)

			return pkghttp.BadRequest(c, pkg.ValidationError{
				Code:    constant.ErrInvalidBatchRequest.Error(),
				Title:   "Invalid Batch Request",
				Message: fmt.Sprintf("Invalid HTTP method: %s", req.Method),
			})
		}
		// Parse path to handle query strings for recursive batch check
		pathWithoutQuery := strings.Split(req.Path, "?")[0]
		pathWithoutQuery = strings.TrimSuffix(pathWithoutQuery, "/")

		// Prevent recursive batch requests
		if strings.HasSuffix(pathWithoutQuery, "/batch") {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Recursive batch request", constant.ErrRecursiveBatchRequest)
			propagateRequestID(c)

			return pkghttp.BadRequest(c, pkg.ValidationError{
				Code:    constant.ErrRecursiveBatchRequest.Error(),
				Title:   "Invalid Batch Request",
				Message: "Batch requests cannot contain nested batch requests",
			})
		}

		// URL-decode the path before checking for traversal (prevents %2e%2e bypass)
		decodedPath, err := url.PathUnescape(req.Path)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid URL encoding in path", constant.ErrInvalidBatchRequest)
			propagateRequestID(c)

			return pkghttp.BadRequest(c, pkg.ValidationError{
				Code:    constant.ErrInvalidBatchRequest.Error(),
				Title:   "Invalid Batch Request",
				Message: "Invalid URL encoding in path",
			})
		}

		// Prevent path traversal attacks (check both original and decoded)
		cleanPath := filepath.Clean(decodedPath)
		if strings.Contains(decodedPath, "..") || strings.Contains(req.Path, "..") ||
			(cleanPath != decodedPath && strings.Contains(cleanPath, "..")) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Path traversal attempt", constant.ErrInvalidBatchRequest)
			propagateRequestID(c)

			return pkghttp.BadRequest(c, pkg.ValidationError{
				Code:    constant.ErrInvalidBatchRequest.Error(),
				Title:   "Invalid Batch Request",
				Message: "Invalid path: path traversal detected",
			})
		}

		// Ensure path starts with /
		if !strings.HasPrefix(req.Path, "/") {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid path format", constant.ErrInvalidBatchRequest)
			propagateRequestID(c)

			return pkghttp.BadRequest(c, pkg.ValidationError{
				Code:    constant.ErrInvalidBatchRequest.Error(),
				Title:   "Invalid Batch Request",
				Message: "Path must start with /",
			})
		}

		// Validate request body size
		if len(req.Body) > MaxRequestBodySize {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Request body too large", constant.ErrInvalidBatchRequest)
			propagateRequestID(c)

			return pkghttp.BadRequest(c, pkg.ValidationError{
				Code:    constant.ErrInvalidBatchRequest.Error(),
				Title:   "Invalid Batch Request",
				Message: "Request body exceeds maximum size of 1MB",
			})
		}

		// Validate header sizes to prevent memory exhaustion
		for key, value := range req.Headers {
			if len(key) > MaxHeaderKeySize {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Header key too large", constant.ErrInvalidBatchRequest)
				propagateRequestID(c)

				// Truncate key for error message to prevent log injection
				displayKey := key
				if len(displayKey) > MaxDisplayIDLength {
					displayKey = displayKey[:MaxDisplayIDLength] + "..."
				}

				return pkghttp.BadRequest(c, pkg.ValidationError{
					Code:    constant.ErrInvalidBatchRequest.Error(),
					Title:   "Invalid Batch Request",
					Message: fmt.Sprintf("Header key '%s' exceeds maximum size of %d bytes", displayKey, MaxHeaderKeySize),
				})
			}

			if len(value) > MaxHeaderValueSize {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Header value too large", constant.ErrInvalidBatchRequest)
				propagateRequestID(c)

				// Truncate key for error message to prevent log injection
				displayKey := key
				if len(displayKey) > MaxDisplayIDLength {
					displayKey = displayKey[:MaxDisplayIDLength] + "..."
				}

				return pkghttp.BadRequest(c, pkg.ValidationError{
					Code:    constant.ErrInvalidBatchRequest.Error(),
					Title:   "Invalid Batch Request",
					Message: fmt.Sprintf("Header value for '%s' exceeds maximum size of %d bytes", displayKey, MaxHeaderValueSize),
				})
			}
		}
	}

	// Process requests in parallel using worker pool pattern
	results := make([]mmodel.BatchResponseItem, len(payload.Requests))

	// Determine number of workers (min of MaxBatchWorkers and request count)
	workers := MaxBatchWorkers
	if len(payload.Requests) < workers {
		workers = len(payload.Requests)
	}

	// Semaphore channel to limit concurrent workers
	sem := make(chan struct{}, workers)

	var wg sync.WaitGroup
	var mu sync.Mutex // Protects writes to results slice for defensive programming

	// Extract headers before parallel processing to avoid race conditions on Fiber context.
	// Fiber context is NOT thread-safe for concurrent reads from c.Get().
	authHeader := c.Get("Authorization")
	parentRequestID := c.Get("X-Request-Id")

	for i, reqItem := range payload.Requests {
		// Truncate path for logging to prevent log injection or storage issues
		truncatedPath := reqItem.Path
		if len(truncatedPath) > MaxLogPathLength {
			truncatedPath = truncatedPath[:MaxLogPathLength] + "..."
		}

		logger.Infof("Queuing batch item %d/%d: %s %s", i+1, len(payload.Requests), reqItem.Method, truncatedPath)

		// Acquire semaphore slot
		sem <- struct{}{}

		wg.Add(1)

		go func(idx int, item mmodel.BatchRequestItem) {
			defer func() {
				// Release semaphore slot
				<-sem
				wg.Done()
			}()

			// Recover from panics to prevent one request from crashing the entire batch
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic recovered while processing batch item %s: %v", item.ID, r)

					mu.Lock()
					results[idx] = mmodel.BatchResponseItem{
						ID:     item.ID,
						Status: http.StatusInternalServerError,
						Error: &mmodel.BatchItemError{
							Code:    constant.ErrInternalServer.Error(),
							Title:   "Internal Server Error",
							Message: "Unexpected error during request processing",
						},
					}
					mu.Unlock()
				}
			}()

			result := h.processRequest(ctx, item, authHeader, parentRequestID)

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, reqItem)
	}

	// Wait for all workers to complete
	wg.Wait()

	// Count successes and failures
	successCount := 0
	failureCount := 0

	for _, result := range results {
		if result.Status >= 200 && result.Status < 300 {
			successCount++
		} else {
			failureCount++
		}
	}

	response := mmodel.BatchResponse{
		SuccessCount: successCount,
		FailureCount: failureCount,
		Results:      results,
	}

	logger.Infof("Batch processing complete: %d success, %d failure", successCount, failureCount)

	// Store response in idempotency cache asynchronously if idempotency key was provided
	// Use a background context with timeout instead of request context, which may be
	// cancelled after the HTTP response is sent
	if idempotencyKey != "" && h.RedisClient != nil {
		idempCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		go func() {
			defer cancel()
			h.setIdempotencyValue(idempCtx, idempotencyKey, &response, idempotencyTTL)
		}()
	}

	// Determine response status code
	// 201 if all success, 207 Multi-Status for any failures (all fail or mixed)
	// Clients can inspect individual results to determine the nature of failures
	var statusCode int

	switch {
	case failureCount == 0:
		statusCode = http.StatusCreated
	default:
		statusCode = http.StatusMultiStatus
	}

	// Propagate request ID for tracing
	propagateRequestID(c)

	return c.Status(statusCode).JSON(response)
}

// processRequest processes a single request item within the batch.
// Parameters authHeader and parentRequestID are pre-extracted from the Fiber context
// to avoid race conditions since Fiber context is not thread-safe for concurrent reads.
func (h *BatchHandler) processRequest(ctx context.Context, reqItem mmodel.BatchRequestItem, authHeader, parentRequestID string) mmodel.BatchResponseItem {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.process_batch_item")
	defer span.End()

	// Validate handler state
	if h.App == nil {
		logger.Errorf("BatchHandler.App is nil for batch item %s", reqItem.ID)

		return mmodel.BatchResponseItem{
			ID:     reqItem.ID,
			Status: http.StatusInternalServerError,
			Error: &mmodel.BatchItemError{
				Code:    constant.ErrInternalServer.Error(),
				Title:   "Internal Server Error",
				Message: "Batch handler not properly initialized",
			},
		}
	}

	// Build the internal request with timeout context
	reqCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, reqItem.Method, reqItem.Path, bytes.NewReader(reqItem.Body))
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create internal request", err)
		logger.Errorf("Failed to create request for batch item %s: %v", reqItem.ID, err)

		return mmodel.BatchResponseItem{
			ID:     reqItem.ID,
			Status: http.StatusInternalServerError,
			Error: &mmodel.BatchItemError{
				Code:    constant.ErrInternalServer.Error(),
				Title:   "Internal Server Error",
				Message: "Failed to create internal request",
			},
		}
	}

	// Copy headers from the original request
	req.Header.Set("Content-Type", "application/json")

	// Copy authorization header from original request (pre-extracted for thread safety)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	// Copy X-Request-Id for tracing (pre-extracted for thread safety)
	if parentRequestID != "" {
		req.Header.Set("X-Request-Id", parentRequestID+"-"+reqItem.ID)
	}

	// Copy only allowed custom headers from the batch item
	// Security-critical headers cannot be overridden to prevent authorization bypass
	// Header sizes are validated during batch request validation, but we validate again here as defense in depth
	for key, value := range reqItem.Headers {
		if forbiddenHeaders[strings.ToLower(key)] {
			logger.Warnf("Blocked forbidden header in batch request item %s: %s", reqItem.ID, key)

			continue
		}

		// Defense in depth: validate header sizes again (already validated in ProcessBatch)
		if len(key) > MaxHeaderKeySize || len(value) > MaxHeaderValueSize {
			logger.Warnf("Invalid header size in batch request item %s: key=%d bytes, value=%d bytes", reqItem.ID, len(key), len(value))

			return mmodel.BatchResponseItem{
				ID:     reqItem.ID,
				Status: http.StatusBadRequest,
				Error: &mmodel.BatchItemError{
					Code:    constant.ErrInvalidBatchRequest.Error(),
					Title:   "Invalid Batch Request",
					Message: "Header key or value exceeds maximum size",
				},
			}
		}

		req.Header.Set(key, value)
	}

	// Use Fiber's internal routing for production (not Test method which creates TCP connections)
	// Create a fasthttp RequestCtx for internal routing
	fasthttpCtx := &fasthttp.RequestCtx{}

	// Set method and URI
	fasthttpCtx.Request.Header.SetMethod(reqItem.Method)
	fasthttpCtx.Request.SetRequestURI(reqItem.Path)

	// Copy body
	if len(reqItem.Body) > 0 {
		fasthttpCtx.Request.SetBody(reqItem.Body)
	}

	// Copy headers from http.Request to fasthttp request
	for key, values := range req.Header {
		for _, value := range values {
			fasthttpCtx.Request.Header.Set(key, value)
		}
	}

	// Propagate the deadline-aware context to Fiber handlers.
	// This allows handlers calling c.UserContext() to receive the context with timeout,
	// enabling cooperative cancellation when the deadline is exceeded.
	// Key "__local_user_ctx__" is Fiber v2's internal key for user context.
	fasthttpCtx.SetUserValue("__local_user_ctx__", reqCtx)

	// Create a channel to handle timeout and response
	type handlerResult struct {
		err error
	}

	resultChan := make(chan handlerResult, 1)

	// Execute handler in goroutine with timeout.
	// Note: If the handler doesn't check context.Done(), the goroutine may outlive
	// this function when timeout occurs. The buffered channel prevents blocking,
	// and the goroutine will terminate when the handler eventually completes.
	go func() {
		// Call Fiber's handler directly (internal routing without TCP)
		// Handler expects *fasthttp.RequestCtx
		h.App.Handler()(fasthttpCtx)
		resultChan <- handlerResult{err: nil}
	}()

	// Wait for handler completion or timeout
	select {
	case <-reqCtx.Done():
		if reqCtx.Err() == context.DeadlineExceeded {
			return mmodel.BatchResponseItem{
				ID:     reqItem.ID,
				Status: http.StatusRequestTimeout,
				Error: &mmodel.BatchItemError{
					Code:    constant.ErrBatchRequestTimeout.Error(),
					Title:   "Request Timeout",
					Message: "Request exceeded timeout of 30 seconds",
				},
			}
		}

		return mmodel.BatchResponseItem{
			ID:     reqItem.ID,
			Status: http.StatusInternalServerError,
			Error: &mmodel.BatchItemError{
				Code:    constant.ErrInternalServer.Error(),
				Title:   "Internal Server Error",
				Message: "Request context cancelled",
			},
		}
	case res := <-resultChan:
		if res.err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to execute internal request", res.err)
			logger.Errorf("Failed to execute request for batch item %s: %v", reqItem.ID, res.err)

			return mmodel.BatchResponseItem{
				ID:     reqItem.ID,
				Status: http.StatusInternalServerError,
				Error: &mmodel.BatchItemError{
					Code:    constant.ErrInternalServer.Error(),
					Title:   "Internal Server Error",
					Message: "Failed to execute internal request",
				},
			}
		}
	}

	// Extract response status code
	statusCode := fasthttpCtx.Response.StatusCode()

	// Capture response headers
	headers := make(map[string]string)
	fasthttpCtx.Response.Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})

	// Read response body with size limit to prevent memory exhaustion
	// Copy body first to avoid referencing fasthttp internal buffer
	body := fasthttpCtx.Response.Body()
	bodyCopy := make([]byte, len(body))
	copy(bodyCopy, body)
	if len(bodyCopy) > MaxResponseBodySize {
		bodyCopy = bodyCopy[:MaxResponseBodySize]
		logger.Warnf("Response body truncated for batch item %s (exceeded %d bytes)", reqItem.ID, MaxResponseBodySize)
	}

	result := mmodel.BatchResponseItem{
		ID:      reqItem.ID,
		Status:  statusCode,
		Headers: headers,
	}

	// If success, include body; if error, parse error structure
	if statusCode >= 200 && statusCode < 300 {
		if len(bodyCopy) > 0 {
			result.Body = bodyCopy
		}
	} else {
		// Try to parse error response
		var errResp struct {
			Code    string `json:"code"`
			Title   string `json:"title"`
			Message string `json:"message"`
		}

		if err := json.Unmarshal(bodyCopy, &errResp); err == nil && errResp.Code != "" {
			result.Error = &mmodel.BatchItemError{
				Code:    errResp.Code,
				Title:   errResp.Title,
				Message: errResp.Message,
			}
		} else {
			// If we can't parse the error, include raw body
			result.Body = bodyCopy
		}
	}

	logger.Infof("Batch item %s completed with status %d", reqItem.ID, statusCode)

	return result
}

// checkOrCreateIdempotencyKey checks if an idempotency key exists in Redis.
// If it exists and has a value, it returns the cached response.
// If it exists but is empty (in progress), it returns an error.
// If it doesn't exist, it creates the key with an empty value and returns nil.
func (h *BatchHandler) checkOrCreateIdempotencyKey(ctx context.Context, key string, ttl time.Duration) (*mmodel.BatchResponse, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.batch_idempotency_check")
	defer span.End()

	logger.Infof("Checking idempotency key for batch request: %s", key)

	internalKey := utils.BatchIdempotencyKey(key)

	// Use ttl directly since it's already time.Duration
	// Try to acquire the lock using SetNX
	success, err := h.RedisClient.SetNX(ctx, internalKey, "", ttl).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error setting idempotency key in Redis", err)

		logger.Errorf("Error setting idempotency key in Redis: %v", err)

		return nil, err
	}

	if !success {
		// Key already exists - check if it has a value
		value, err := h.RedisClient.Get(ctx, internalKey).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			libOpentelemetry.HandleSpanError(&span, "Error getting idempotency key from Redis", err)

			logger.Errorf("Error getting idempotency key from Redis: %v", err)

			return nil, err
		}

		if value != "" {
			// Key exists with value - deserialize and return cached response
			logger.Infof("Found cached batch response for idempotency key: %s", key)

			var cachedResponse mmodel.BatchResponse
			if err := json.Unmarshal([]byte(value), &cachedResponse); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Error deserializing cached batch response", err)

				logger.Errorf("Error deserializing cached batch response: %v", err)

				return nil, err
			}

			return &cachedResponse, nil
		}

		// Key exists but is empty - request in progress, return conflict error
		logger.Warnf("Idempotency key already in use (request in progress): %s", key)

		return nil, pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "ProcessBatch", key)
	}

	// Key was successfully created - proceed with processing
	logger.Infof("Created idempotency lock for batch request: %s", key)

	return nil, nil
}

// setIdempotencyValue stores the batch response in Redis for the given idempotency key.
// This is called asynchronously after successful batch processing.
func (h *BatchHandler) setIdempotencyValue(ctx context.Context, key string, response *mmodel.BatchResponse, ttl time.Duration) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.batch_idempotency_set")
	defer span.End()

	logger.Infof("Storing batch response for idempotency key: %s", key)

	internalKey := utils.BatchIdempotencyKey(key)

	value, err := json.Marshal(response)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error serializing batch response", err)

		logger.Errorf("Error serializing batch response for idempotency: %v", err)

		return
	}

	// Use ttl directly since it's already time.Duration
	// Use SetXX to only set if key exists (we created it with SetNX)
	// This prevents race conditions where key might have expired
	err = h.RedisClient.SetXX(ctx, internalKey, string(value), ttl).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error storing batch response in Redis", err)

		logger.Errorf("Error storing batch response in Redis: %v", err)

		return
	}

	logger.Infof("Successfully stored batch response for idempotency key: %s", key)
}
