/**
 * Custom error classes for the demo data generator
 */

/**
 * Error thrown when entity generation fails
 */
export class GenerationError extends Error {
  constructor(
    message: string,
    public readonly entityType: string,
    public readonly parentId?: string,
    public readonly context?: Record<string, any>
  ) {
    super(message);
    this.name = 'GenerationError';
    Object.setPrototypeOf(this, GenerationError.prototype);
  }
}

/**
 * Error thrown when validation fails
 */
export class ValidationError extends Error {
  constructor(
    message: string,
    public readonly errors: Array<{ path: string; message: string }>
  ) {
    super(message);
    this.name = 'ValidationError';
    Object.setPrototypeOf(this, ValidationError.prototype);
  }
}

/**
 * Error thrown when dependencies are not met
 */
export class DependencyError extends Error {
  constructor(
    message: string,
    public readonly missingDependency: string,
    public readonly parentId: string
  ) {
    super(message);
    this.name = 'DependencyError';
    Object.setPrototypeOf(this, DependencyError.prototype);
  }
}