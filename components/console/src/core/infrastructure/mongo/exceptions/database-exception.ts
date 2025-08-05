/**
 * @file Database Exceptions
 * @description Defines a hierarchy of database-related exceptions for the application.
 * These exceptions provide a structured way to handle different types of database errors
 * while maintaining a clean separation between infrastructure and domain layers.
 */

import { ApiException, HttpStatus } from '@/lib/http'

/**
 * Base database exception class
 * @class
 * @extends Error
 * @description Generic database exception used as a base class for more specific exceptions.
 * This can be used directly for general or unexpected database errors.
 */
export class DatabaseException extends ApiException {
  /**
   * Creates a new DatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(
      '0000',
      'Database Exception',
      message,
      HttpStatus.INTERNAL_SERVER_ERROR
    )
    this.name = 'DatabaseException'
  }
}

/**
 * Invalid object database exception
 * @class
 * @extends DatabaseException
 * @description Thrown when an operation fails due to an invalid object ID or reference.
 * Typically occurs with MongoDB when a cast error happens (e.g., invalid ObjectId format).
 */
export class InvalidObjectDatabaseException extends ApiException {
  /**
   * Creates a new InvalidObjectDatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(
      '0001',
      'Invalid Object Database Exception',
      message,
      HttpStatus.BAD_REQUEST
    )
    this.name = 'InvalidObjectDatabaseException'
  }
}

/**
 * Validation failed database exception
 * @class
 * @extends DatabaseException
 * @description Thrown when database validation rules fail during create or update operations.
 * Occurs when the data being saved doesn't meet the schema validation requirements.
 */
export class ValidationFailedDatabaseException extends ApiException {
  /**
   * Creates a new ValidationFailedDatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(
      '0002',
      'Validation Failed Database Exception',
      message,
      HttpStatus.BAD_REQUEST
    )
    this.name = 'ValidationFailedDatabaseException'
  }
}

/**
 * Constraint database exception
 * @class
 * @extends DatabaseException
 * @description Thrown when a database constraint is violated (e.g., unique index, foreign key).
 * Typically occurs when trying to create duplicate entries where uniqueness is required.
 */
export class ConstraintDatabaseException extends ApiException {
  /**
   * Creates a new ConstraintDatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(
      '0003',
      'Constraint Database Exception',
      message,
      HttpStatus.BAD_REQUEST
    )
    this.name = 'ConstraintDatabaseException'
  }
}

export class NotFoundDatabaseException extends ApiException {
  entity: string
  constructor(message: string, entity: string) {
    super('0004', 'Not Found Database Exception', message, HttpStatus.NOT_FOUND)

    this.entity = entity
    this.name = 'NotFoundDatabaseException'
  }
}

export class DuplicatedKeyError extends ApiException {
  constructor(message: string) {
    super(
      '0005',
      'Duplicated Key Database Exception',
      message,
      HttpStatus.BAD_REQUEST
    )
    this.name = 'DuplicatedKeyError'
  }
}
