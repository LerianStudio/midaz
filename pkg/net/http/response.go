// Package http provides HTTP utilities and helpers for the Midaz ledger system.
// This file contains HTTP response helper functions for consistent API responses.
package http

import (
	"net/http"
	"strconv"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
)

// HTTP Response Helper Functions
//
// This file provides a set of helper functions for sending standardized HTTP responses
// using the Fiber web framework. These functions ensure consistent response formatting
// across all API endpoints.

// Unauthorized sends an HTTP 401 Unauthorized response.
//
// Use this function when authentication is required but not provided, or when the
// provided authentication token is invalid or expired.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - code: Business error code (e.g., "0041" for token missing)
//   - title: Human-readable error title
//   - message: Detailed error message for the client
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.Unauthorized(c, "0041", "Token Missing", "A valid token must be provided.")
func Unauthorized(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// Forbidden sends an HTTP 403 Forbidden response.
//
// Use this function when the authenticated user lacks sufficient privileges to perform
// the requested action.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - code: Business error code (e.g., "0043" for insufficient privileges)
//   - title: Human-readable error title
//   - message: Detailed error message for the client
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.Forbidden(c, "0043", "Insufficient Privileges", "You do not have permission.")
func Forbidden(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusForbidden).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// BadRequest sends an HTTP 400 Bad Request response.
//
// Use this function when the request is malformed, contains invalid data, or fails
// validation. The body can be any serializable structure, typically a validation error.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - s: Response body (typically pkg.ValidationKnownFieldsError or pkg.ValidationUnknownFieldsError)
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.BadRequest(c, pkg.ValidationKnownFieldsError{
//	    Code: "0009",
//	    Title: "Missing Fields",
//	    Message: "Required fields are missing.",
//	    Fields: map[string]string{"name": "Name is required"},
//	})
func BadRequest(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusBadRequest).JSON(s)
}

// Created sends an HTTP 201 Created response.
//
// Use this function when a new resource has been successfully created. The body typically
// contains the newly created resource.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - s: Response body (typically the created entity)
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	account, err := service.CreateAccount(input)
//	if err != nil {
//	    return http.WithError(c, err)
//	}
//	return http.Created(c, account)
func Created(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusCreated).JSON(s)
}

// OK sends an HTTP 200 OK response.
//
// Use this function for successful requests that return data. This is the most common
// success response for GET, PUT, and PATCH operations.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - s: Response body (typically an entity or collection)
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	accounts, err := service.ListAccounts(query)
//	if err != nil {
//	    return http.WithError(c, err)
//	}
//	return http.OK(c, accounts)
func OK(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusOK).JSON(s)
}

// NoContent sends an HTTP 204 No Content response.
//
// Use this function for successful requests that don't return data, typically for
// DELETE operations or updates that don't return the updated resource.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	if err := service.DeleteAccount(id); err != nil {
//	    return http.WithError(c, err)
//	}
//	return http.NoContent(c)
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(http.StatusNoContent)
}

// Accepted sends an HTTP 202 Accepted response.
//
// Use this function when a request has been accepted for processing but the processing
// has not been completed. This is typically used for asynchronous operations.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - s: Response body (typically contains a job ID or status information)
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	jobID, err := service.StartAsyncOperation(input)
//	if err != nil {
//	    return http.WithError(c, err)
//	}
//	return http.Accepted(c, fiber.Map{"jobId": jobID})
func Accepted(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusAccepted).JSON(s)
}

// PartialContent sends an HTTP 206 Partial Content response.
//
// Use this function when returning a partial result, typically for range requests
// or when a large dataset is being returned in chunks.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - s: Response body (partial data)
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.PartialContent(c, partialData)
func PartialContent(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusPartialContent).JSON(s)
}

// RangeNotSatisfiable sends an HTTP 416 Requested Range Not Satisfiable response.
//
// Use this function when a range request cannot be satisfied (e.g., requesting bytes
// beyond the end of a resource).
//
// Parameters:
//   - c: Fiber context for the HTTP request
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	if requestedRange > totalSize {
//	    return http.RangeNotSatisfiable(c)
//	}
func RangeNotSatisfiable(c *fiber.Ctx) error {
	return c.SendStatus(http.StatusRequestedRangeNotSatisfiable)
}

// NotFound sends an HTTP 404 Not Found response.
//
// Use this function when a requested resource does not exist.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - code: Business error code (e.g., "0007" for entity not found)
//   - title: Human-readable error title
//   - message: Detailed error message for the client
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.NotFound(c, "0052", "Account Not Found", "The account ID does not exist.")
func NotFound(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusNotFound).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// Conflict sends an HTTP 409 Conflict response.
//
// Use this function when a request conflicts with the current state of the resource,
// typically for duplicate entries or unique constraint violations.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - code: Business error code (e.g., "0001" for duplicate ledger)
//   - title: Human-readable error title
//   - message: Detailed error message for the client
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.Conflict(c, "0020", "Alias Unavailable", "The alias is already in use.")
func Conflict(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusConflict).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// NotImplemented sends an HTTP 501 Not Implemented response.
//
// Use this function when a feature or endpoint is not yet implemented.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - message: Detailed error message for the client
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.NotImplemented(c, "This feature is not yet available.")
func NotImplemented(c *fiber.Ctx, message string) error {
	return c.Status(http.StatusNotImplemented).JSON(fiber.Map{
		"code":    http.StatusNotImplemented,
		"title":   "Not Implemented",
		"message": message,
	})
}

// UnprocessableEntity sends an HTTP 422 Unprocessable Entity response.
//
// Use this function when the request is well-formed but semantically invalid,
// typically for business logic errors like insufficient funds.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - code: Business error code (e.g., "0018" for insufficient funds)
//   - title: Human-readable error title
//   - message: Detailed error message for the client
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.UnprocessableEntity(c, "0018", "Insufficient Funds", "The account balance is too low.")
func UnprocessableEntity(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// InternalServerError sends an HTTP 500 Internal Server Error response.
//
// Use this function when an unexpected server-side error occurs.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - code: Business error code (e.g., "0046" for internal server error)
//   - title: Human-readable error title
//   - message: Detailed error message for the client
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.InternalServerError(c, "0046", "Internal Server Error", "An unexpected error occurred.")
func InternalServerError(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// JSONResponseError sends a JSON formatted error response with a custom error struct.
//
// This function converts a pkg.ResponseError to an HTTP response, using the error's
// Code field as the HTTP status code. The entire error struct is serialized to JSON.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - err: ResponseError containing code, title, message, and other error details
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	respErr := pkg.ResponseError{
//	    Code: "400",
//	    Title: "Bad Request",
//	    Message: "Invalid input",
//	}
//	return http.JSONResponseError(c, respErr)
func JSONResponseError(c *fiber.Ctx, err pkg.ResponseError) error {
	code, _ := strconv.Atoi(err.Code)

	return c.Status(code).JSON(err)
}

// JSONResponse sends a custom status code and body as a JSON response.
//
// This is a generic response function that allows full control over the HTTP status code
// and response body. Use this when the standard response functions don't fit your needs.
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - status: HTTP status code (e.g., http.StatusOK, http.StatusCreated)
//   - s: Response body (any JSON-serializable structure)
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example:
//
//	return http.JSONResponse(c, http.StatusTeapot, fiber.Map{"message": "I'm a teapot"})
func JSONResponse(c *fiber.Ctx, status int, s any) error {
	return c.Status(status).JSON(s)
}
