/**
 * Type guards for better type safety
 */

import { GenerationError, ValidationError, DependencyError } from '../errors';

/**
 * Check if error is a GenerationError
 */
export function isGenerationError(error: unknown): error is GenerationError {
  return error instanceof Error && 'entityType' in error;
}

/**
 * Check if error is a ValidationError
 */
export function isValidationError(error: unknown): error is ValidationError {
  return error instanceof Error && 'errors' in error && Array.isArray((error as any).errors);
}

/**
 * Check if error is a DependencyError
 */
export function isDependencyError(error: unknown): error is DependencyError {
  return error instanceof Error && 'missingDependency' in error;
}

/**
 * Check if value is a string
 */
export function isString(value: unknown): value is string {
  return typeof value === 'string';
}

/**
 * Check if value is a number
 */
export function isNumber(value: unknown): value is number {
  return typeof value === 'number' && !isNaN(value);
}

/**
 * Check if value is an object
 */
export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * Check if value is an array
 */
export function isArray(value: unknown): value is unknown[] {
  return Array.isArray(value);
}

/**
 * Assert that a value is defined (not null or undefined)
 */
export function assertDefined<T>(value: T | null | undefined, message?: string): asserts value is T {
  if (value === null || value === undefined) {
    throw new Error(message || 'Expected value to be defined');
  }
}

/**
 * Check if value is a valid organization ID
 */
export function isValidOrganizationId(value: unknown): value is string {
  return isString(value) && value.length > 0;
}

/**
 * Check if value is a valid entity ID
 */
export function isValidEntityId(value: unknown): value is string {
  return isString(value) && value.length > 0;
}

/**
 * Safe JSON parse with type checking
 */
export function safeJsonParse<T>(
  value: string, 
  validator: (parsed: unknown) => parsed is T
): T | null {
  try {
    const parsed = JSON.parse(value);
    return validator(parsed) ? parsed : null;
  } catch {
    return null;
  }
}