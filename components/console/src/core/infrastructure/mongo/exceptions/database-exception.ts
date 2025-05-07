/**
 * @file Database Exceptions
 * @description Defines a hierarchy of database-related exceptions for the application.
 * These exceptions provide a structured way to handle different types of database errors
 * while maintaining a clean separation between infrastructure and domain layers.
 */

/**
 * Base database exception class
 * @class
 * @extends Error
 * @description Generic database exception used as a base class for more specific exceptions.
 * This can be used directly for general or unexpected database errors.
 */
export class DatabaseException extends Error {
  /**
   * Creates a new DatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(message)
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
export class InvalidObjectDatabaseException extends DatabaseException {
  /**
   * Creates a new InvalidObjectDatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(message)
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
export class ValidationFailedDatabaseException extends DatabaseException {
  /**
   * Creates a new ValidationFailedDatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(message)
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
export class ConstraintDatabaseException extends DatabaseException {
  /**
   * Creates a new ConstraintDatabaseException
   * @param message - The error message
   */
  constructor(message: string) {
    super(message)
    this.name = 'ConstraintDatabaseException'
  }
}
