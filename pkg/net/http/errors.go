package http

import (
	"errors"
	"log"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
)

// maxErrorChainDepth is the maximum depth for unwrapping error chains in diagnostic logging.
// TODO(diagnostic): REMOVE THIS AFTER INFRASTRUCTURE ERROR TYPING IS COMPLETE
const maxErrorChainDepth = 10

// WithError returns an error with the given status code and message.
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

// logUnknownErrorDetails logs detailed information about errors for debugging.
// TODO(diagnostic): REMOVE THIS AFTER INFRASTRUCTURE ERROR TYPING IS COMPLETE
// This diagnostic function was added to identify which errors need proper typing.
// It helps trace what error types are falling through to 500 responses.
func logUnknownErrorDetails(err error) {
	// Log error type and value
	log.Printf("[DIAGNOSTIC] Unknown error falling through to 500:")
	log.Printf("  Type: %s", reflect.TypeOf(err))
	log.Printf("  Value: %+v", err)
	log.Printf("  Message: %s", err.Error())

	// Unwrap and log error chain
	log.Printf("  Error Chain:")

	current := err
	depth := 1

	for current != nil {
		log.Printf("    %d. %s: %s", depth, reflect.TypeOf(current), current.Error())

		// Try to unwrap
		unwrapped := errors.Unwrap(current)
		if unwrapped == nil || errors.Is(unwrapped, current) {
			break
		}

		current = unwrapped
		depth++

		// Prevent infinite loops
		if depth > maxErrorChainDepth {
			log.Printf("    ... (max depth reached)")
			break
		}
	}
}

// handleUnknownError handles unknown errors by converting them to InternalServerError
func handleUnknownError(c *fiber.Ctx, err error) error {
	// DIAGNOSTIC: Log details about unknown errors to identify which need typing
	// TODO(diagnostic): REMOVE after infrastructure error typing is complete
	logUnknownErrorDetails(err)

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
