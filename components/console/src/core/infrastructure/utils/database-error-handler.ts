/**
 * @file Database Error Handler
 * @description Provides utilities for handling and transforming database errors into domain-specific exceptions
 * with proper internationalization support. Centralizes error handling logic for MongoDB operations.
 */

import { getIntl } from '@/lib/intl'
import mongoose, { MongooseError } from 'mongoose'
import { IntlShape } from 'react-intl'
import {
  DatabaseException,
  InvalidObjectDatabaseException,
  ValidationFailedDatabaseException
} from '../mongo/exceptions/database-exception'

/**
 * Main entry point for handling database errors
 * @param error - The error thrown by a database operation
 * @throws {DatabaseException} A domain-specific exception with localized error message
 * @description Transforms raw database errors into domain-specific exceptions with proper internationalization.
 * This function should be used in catch blocks of database operations.
 */
export async function handleDatabaseError(error: unknown): Promise<void> {
  const intl = await getIntl()

  console.log('[DATABASE ERROR]', error)

  if (error instanceof mongoose.Error) {
    const mapped = mapMongooseError(error, intl)
    throw mapped
  }

  throw unexpectedDatabaseError(intl)
}

/**
 * Maps Mongoose-specific errors to domain-specific exceptions
 * @param error - The Mongoose error to map
 * @param intl - Internationalization service for localized error messages
 * @returns A domain-specific database exception
 * @private
 * @description Routes different types of Mongoose errors to their specific handlers
 */
function mapMongooseError(
  error: MongooseError,
  intl: IntlShape
): DatabaseException {
  switch (error.name) {
    case 'ValidationError':
      return validationFailedError(
        error as mongoose.Error.ValidationError,
        intl
      )
    case 'CastError':
      return invalidObjectError(error as mongoose.Error.CastError, intl)
    default:
      return unexpectedDatabaseError(intl)
  }
}

/**
 * Handles cast errors from Mongoose
 * @param mongooseError - The Mongoose CastError to handle
 * @param intl - Internationalization service for localized error messages
 * @returns An InvalidObjectDatabaseException with localized message
 * @private
 * @description Transforms Mongoose CastError (typically for invalid ObjectIds or type mismatches)
 * into a domain-specific exception with proper error message
 */
function invalidObjectError(
  mongooseError: mongoose.Error.CastError,
  intl: IntlShape
): InvalidObjectDatabaseException {
  const message = intl.formatMessage(
    {
      id: 'error.database.invalidObject',
      defaultMessage: 'Invalid object id'
    },
    {
      entity: mongooseError.path
    }
  )

  return new InvalidObjectDatabaseException(message)
}

/**
 * Creates a generic database exception for unexpected errors
 * @param intl - Internationalization service for localized error messages
 * @returns A generic DatabaseException with localized message
 * @private
 * @description Used as a fallback for unhandled or unexpected database errors
 */
function unexpectedDatabaseError(intl: IntlShape): DatabaseException {
  const message = intl.formatMessage({
    id: 'error.database.unexpected',
    defaultMessage: 'Unexpected error'
  })

  return new DatabaseException(message)
}

/**
 * Handles validation errors from Mongoose
 * @param mongooseError - The Mongoose ValidationError to handle
 * @param intl - Internationalization service for localized error messages
 * @returns A ValidationFailedDatabaseException with localized message
 * @private
 * @description Transforms Mongoose ValidationError (schema validation failures)
 * into a domain-specific exception with proper error message including the failed fields
 */
function validationFailedError(
  mongooseError: mongoose.Error.ValidationError,
  intl: IntlShape
): ValidationFailedDatabaseException {
  const validationErrors = extractValidationErrors(mongooseError)
  const message = intl.formatMessage(
    {
      id: 'error.database.validationFailed',
      defaultMessage: 'Validation failed'
    },
    {
      fields: validationErrors.map((error) => error.field).join(', ')
    }
  )

  return new ValidationFailedDatabaseException(message)
}

/**
 * Extracts structured validation errors from a Mongoose ValidationError
 * @param mongooseError - The Mongoose ValidationError to extract information from
 * @returns An array of objects containing field names and error messages
 * @private
 * @description Transforms the complex Mongoose validation error structure into a simpler
 * array of field/message pairs that can be used for error reporting
 */
function extractValidationErrors(
  mongooseError: mongoose.Error.ValidationError
): { field: string; message: string }[] {
  return Object.values(mongooseError.errors).map((fieldError) => ({
    field: fieldError.path,
    message: fieldError.message
  }))
}
