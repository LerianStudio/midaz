package http

import (
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
)

// WithError returns an error with the given status code and message.
//
// DESIGN NOTE: No assertions on c or err parameters.
// - c (fiber.Ctx): Fiber guarantees non-nil context when calling handlers.
//   If we receive nil, Fiber itself is broken - panic is appropriate.
// - err: May be nil in edge cases (defensive callers). We handle this gracefully
//   in handleUnknownError rather than asserting.
//
// The current behavior (implicit nil dereference for c, graceful handling for err)
// is intentional and appropriate for this HTTP boundary.
func WithError(c *fiber.Ctx, err error) error {
	// Handle standard error types
	if handled, result := handleStandardErrors(c, err); handled {
		return result
	}

	// Handle special error types
	if handled, result := handleSpecialErrors(c, err); handled {
		return result
	}

	// Handle unknown errors
	return handleUnknownError(c, err)
}

// handleStandardErrors handles common business errors
func handleStandardErrors(c *fiber.Ctx, err error) (bool, error) {
	var entityNotFoundErr pkg.EntityNotFoundError
	if errors.As(err, &entityNotFoundErr) {
		return true, NotFound(c, entityNotFoundErr.Code, entityNotFoundErr.Title, entityNotFoundErr.Message)
	}

	var entityConflictErr pkg.EntityConflictError
	if errors.As(err, &entityConflictErr) {
		return true, Conflict(c, entityConflictErr.Code, entityConflictErr.Title, entityConflictErr.Message)
	}

	var validationErr pkg.ValidationError
	if errors.As(err, &validationErr) {
		return true, BadRequest(c, pkg.ValidationKnownFieldsError{
			Code:    validationErr.Code,
			Title:   validationErr.Title,
			Message: validationErr.Message,
			Fields:  nil,
		})
	}

	var unprocessableOpErr pkg.UnprocessableOperationError
	if errors.As(err, &unprocessableOpErr) {
		return true, UnprocessableEntity(c, unprocessableOpErr.Code, unprocessableOpErr.Title, unprocessableOpErr.Message)
	}

	// Handle FailedPreconditionError - maps to 422 (Unprocessable Entity) for precondition failures
	var failedPrecondErr pkg.FailedPreconditionError
	if errors.As(err, &failedPrecondErr) {
		return true, UnprocessableEntity(c, failedPrecondErr.Code, failedPrecondErr.Title, failedPrecondErr.Message)
	}

	var unauthorizedErr pkg.UnauthorizedError
	if errors.As(err, &unauthorizedErr) {
		return true, Unauthorized(c, unauthorizedErr.Code, unauthorizedErr.Title, unauthorizedErr.Message)
	}

	var forbiddenErr pkg.ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return true, Forbidden(c, forbiddenErr.Code, forbiddenErr.Title, forbiddenErr.Message)
	}

	return false, nil
}

// handleSpecialErrors handles validation field errors and response errors
func handleSpecialErrors(c *fiber.Ctx, err error) (bool, error) {
	if handled, result := handleValidationFieldsError(c, err); handled {
		return true, result
	}

	var responseErr pkg.ResponseError
	if errors.As(err, &responseErr) {
		return true, JSONResponseError(c, responseErr)
	}

	var libCommonsResp libCommons.Response
	if errors.As(err, &libCommonsResp) {
		return true, handleLibCommonsResponse(c, libCommonsResp)
	}

	var internalServerErr pkg.InternalServerError
	if errors.As(err, &internalServerErr) {
		return true, InternalServerError(c, internalServerErr.Code, internalServerErr.Title, internalServerErr.Message)
	}

	return false, nil
}

// handleUnknownError handles unknown errors by converting them to InternalServerError
func handleUnknownError(c *fiber.Ctx, err error) error {
	internalErr := pkg.ValidateInternalError(err, "")

	var internalServerErr pkg.InternalServerError
	if errors.As(internalErr, &internalServerErr) {
		return InternalServerError(c, internalServerErr.Code, internalServerErr.Title, internalServerErr.Message)
	}

	// Fallback if ValidateInternalError doesn't return the expected type
	return InternalServerError(c, "INTERNAL_ERROR", "Internal Server Error", "An unexpected error occurred")
}

// handleValidationFieldsError handles ValidationKnownFieldsError and ValidationUnknownFieldsError
// It checks for both value and pointer types since ValidateStruct returns pointers.
func handleValidationFieldsError(c *fiber.Ctx, err error) (bool, error) {
	// Check for value type
	var knownFieldsErr pkg.ValidationKnownFieldsError
	if errors.As(err, &knownFieldsErr) {
		return true, BadRequest(c, knownFieldsErr)
	}

	// Check for pointer type (ValidateStruct returns *ValidationKnownFieldsError)
	var knownFieldsErrPtr *pkg.ValidationKnownFieldsError
	if errors.As(err, &knownFieldsErrPtr) && knownFieldsErrPtr != nil {
		return true, BadRequest(c, *knownFieldsErrPtr)
	}

	// Check for value type
	var unknownFieldsErr pkg.ValidationUnknownFieldsError
	if errors.As(err, &unknownFieldsErr) {
		return true, BadRequest(c, unknownFieldsErr)
	}

	// Check for pointer type
	var unknownFieldsErrPtr *pkg.ValidationUnknownFieldsError
	if errors.As(err, &unknownFieldsErrPtr) && unknownFieldsErrPtr != nil {
		return true, BadRequest(c, *unknownFieldsErrPtr)
	}

	return false, nil
}

// handleLibCommonsResponse handles libCommons.Response error type
func handleLibCommonsResponse(c *fiber.Ctx, resp libCommons.Response) error {
	switch resp.Code {
	case libConstants.ErrInsufficientFunds.Error(), libConstants.ErrAccountIneligibility.Error():
		return UnprocessableEntity(c, resp.Code, resp.Title, resp.Message)
	case libConstants.ErrAssetCodeNotFound.Error():
		return NotFound(c, resp.Code, resp.Title, resp.Message)
	case libConstants.ErrOverFlowInt64.Error():
		return InternalServerError(c, resp.Code, resp.Title, resp.Message)
	default:
		return BadRequest(c, pkg.ValidationKnownFieldsError{
			Code:    resp.Code,
			Title:   resp.Title,
			Message: resp.Message,
		})
	}
}
