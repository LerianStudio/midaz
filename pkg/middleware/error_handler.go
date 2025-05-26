package middleware

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/mongo"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error      ErrorDetails `json:"error"`
	RequestID  string       `json:"request_id,omitempty"`
	Timestamp  time.Time    `json:"timestamp"`
	StatusCode int          `json:"status_code"`
}

// ErrorDetails contains detailed error information
type ErrorDetails struct {
	Code        string                 `json:"code"`
	Message     string                 `json:"message"`
	Details     map[string]interface{} `json:"details,omitempty"`
	StackTrace  string                 `json:"stack_trace,omitempty"`
	UserMessage string                 `json:"user_message,omitempty"`
}

// ErrorType represents the type of error
type ErrorType string

const (
	ErrorTypeValidation   ErrorType = "VALIDATION_ERROR"
	ErrorTypeNotFound     ErrorType = "NOT_FOUND"
	ErrorTypeConflict     ErrorType = "CONFLICT"
	ErrorTypeInternal     ErrorType = "INTERNAL_ERROR"
	ErrorTypeUnauthorized ErrorType = "UNAUTHORIZED"
	ErrorTypeForbidden    ErrorType = "FORBIDDEN"
	ErrorTypeBadRequest   ErrorType = "BAD_REQUEST"
	ErrorTypeTimeout      ErrorType = "TIMEOUT"
	ErrorTypeRateLimit    ErrorType = "RATE_LIMIT"
)

// AppError represents an application-specific error
type AppError struct {
	Type       ErrorType
	Message    string
	UserMessage string
	Details    map[string]interface{}
	Err        error
	StatusCode int
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError creates a new application error
func NewAppError(errType ErrorType, message string, err error) *AppError {
	appErr := &AppError{
		Type:    errType,
		Message: message,
		Err:     err,
		Details: make(map[string]interface{}),
	}

	// Set default status code based on error type
	switch errType {
	case ErrorTypeValidation, ErrorTypeBadRequest:
		appErr.StatusCode = http.StatusBadRequest
	case ErrorTypeNotFound:
		appErr.StatusCode = http.StatusNotFound
	case ErrorTypeConflict:
		appErr.StatusCode = http.StatusConflict
	case ErrorTypeUnauthorized:
		appErr.StatusCode = http.StatusUnauthorized
	case ErrorTypeForbidden:
		appErr.StatusCode = http.StatusForbidden
	case ErrorTypeTimeout:
		appErr.StatusRequestTimeout
	case ErrorTypeRateLimit:
		appErr.StatusCode = http.StatusTooManyRequests
	default:
		appErr.StatusCode = http.StatusInternalServerError
	}

	return appErr
}

// WithUserMessage adds a user-friendly message
func (e *AppError) WithUserMessage(message string) *AppError {
	e.UserMessage = message
	return e
}

// WithDetails adds additional error details
func (e *AppError) WithDetails(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WithStatusCode overrides the default status code
func (e *AppError) WithStatusCode(code int) *AppError {
	e.StatusCode = code
	return e
}

// ErrorHandler creates an error handling middleware
func ErrorHandler(includeStackTrace bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a custom response writer to capture errors
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Recover from panics
			defer func() {
				if err := recover(); err != nil {
					handlePanic(rw, r, err, includeStackTrace)
				}
			}()

			// Process request
			next.ServeHTTP(rw, r)
		})
	}
}

// HandleError processes and responds with appropriate error
func HandleError(w http.ResponseWriter, r *http.Request, err error, includeStackTrace bool) {
	ctx := r.Context()
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "middleware.handle_error")
	defer span.End()

	// Extract request ID
	requestID := getRequestID(r)

	// Determine error type and build response
	var errResp ErrorResponse
	errResp.RequestID = requestID
	errResp.Timestamp = time.Now()

	// Handle different error types
	switch e := err.(type) {
	case *AppError:
		errResp.StatusCode = e.StatusCode
		errResp.Error = ErrorDetails{
			Code:        string(e.Type),
			Message:     e.Message,
			Details:     e.Details,
			UserMessage: e.UserMessage,
		}
		
		// Log based on error type
		if e.StatusCode >= 500 {
			logger.Errorf("Internal error [%s]: %v", requestID, e)
			libOpentelemetry.HandleSpanError(&span, e.Message, e.Err)
		} else {
			logger.Warnf("Client error [%s]: %v", requestID, e)
		}

	default:
		// Map known errors to appropriate responses
		errResp.StatusCode, errResp.Error = mapError(err)
		
		if errResp.StatusCode >= 500 {
			logger.Errorf("Unhandled error [%s]: %v", requestID, err)
			libOpentelemetry.HandleSpanError(&span, "Unhandled error", err)
		} else {
			logger.Warnf("Known error [%s]: %v", requestID, err)
		}
	}

	// Add stack trace in development
	if includeStackTrace && errResp.StatusCode >= 500 {
		errResp.Error.StackTrace = string(debug.Stack())
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(errResp.StatusCode)

	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		logger.Errorf("Failed to encode error response: %v", err)
	}
}

// mapError maps known error types to appropriate responses
func mapError(err error) (int, ErrorDetails) {
	// Check for context errors
	if errors.Is(err, context.Canceled) {
		return http.StatusRequestTimeout, ErrorDetails{
			Code:        string(ErrorTypeTimeout),
			Message:     "Request canceled",
			UserMessage: "The request was canceled",
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusRequestTimeout, ErrorDetails{
			Code:        string(ErrorTypeTimeout),
			Message:     "Request timeout",
			UserMessage: "The request took too long to complete",
		}
	}

	// Check for database errors
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound, ErrorDetails{
			Code:        string(ErrorTypeNotFound),
			Message:     "Resource not found",
			UserMessage: "The requested resource was not found",
		}
	}

	// Check for MongoDB errors
	if errors.Is(err, mongo.ErrNoDocuments) {
		return http.StatusNotFound, ErrorDetails{
			Code:        string(ErrorTypeNotFound),
			Message:     "Document not found",
			UserMessage: "The requested document was not found",
		}
	}

	// Check for duplicate key errors
	if mongo.IsDuplicateKeyError(err) {
		return http.StatusConflict, ErrorDetails{
			Code:        string(ErrorTypeConflict),
			Message:     "Duplicate key error",
			UserMessage: "A resource with the same key already exists",
		}
	}

	// Default to internal server error
	return http.StatusInternalServerError, ErrorDetails{
		Code:        string(ErrorTypeInternal),
		Message:     "Internal server error",
		UserMessage: "An unexpected error occurred. Please try again later.",
	}
}

// handlePanic handles panic recovery
func handlePanic(w http.ResponseWriter, r *http.Request, recovered interface{}, includeStackTrace bool) {
	logger := libCommons.NewLoggerFromContext(r.Context())
	requestID := getRequestID(r)

	// Log the panic
	logger.Errorf("Panic recovered [%s]: %v\n%s", requestID, recovered, debug.Stack())

	// Build error response
	errResp := ErrorResponse{
		RequestID:  requestID,
		Timestamp:  time.Now(),
		StatusCode: http.StatusInternalServerError,
		Error: ErrorDetails{
			Code:        string(ErrorTypeInternal),
			Message:     fmt.Sprintf("Panic: %v", recovered),
			UserMessage: "An unexpected error occurred. Please try again later.",
		},
	}

	if includeStackTrace {
		errResp.Error.StackTrace = string(debug.Stack())
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(errResp)
}

// getRequestID extracts or generates a request ID
func getRequestID(r *http.Request) string {
	// Try to get from header
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	
	// Try to get from context
	if id, ok := r.Context().Value("request_id").(string); ok {
		return id
	}
	
	// Generate new ID
	return uuid.New().String()
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.written = true
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// ErrorLogger creates a middleware that logs errors with context
func ErrorLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add request ID to context
			requestID := getRequestID(r)
			ctx := context.WithValue(r.Context(), "request_id", requestID)
			
			// Create logger with request context
			logger := libCommons.NewLoggerFromContext(ctx)
			logger = logger.WithFields(map[string]interface{}{
				"request_id": requestID,
				"method":     r.Method,
				"path":       r.URL.Path,
				"remote_addr": r.RemoteAddr,
			})
			
			// Add logger to context
			ctx = libCommons.ContextWithLogger(ctx, logger)
			
			// Continue with updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}