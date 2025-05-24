import { z } from 'zod';
import { ValidationError } from '../errors';

export interface ValidationResult<T> {
  success: boolean;
  data?: T;
  errors?: string[];
}

export class Validator {
  /**
   * Validates a single entity against a schema
   */
  static validate<T>(schema: z.ZodSchema<T>, data: unknown, entityType: string): ValidationResult<T> {
    try {
      const validatedData = schema.parse(data);
      return {
        success: true,
        data: validatedData,
      };
    } catch (error) {
      if (error instanceof z.ZodError) {
        const errors = error.errors.map(err => {
          const path = err.path.join('.');
          return `${path ? `${path}: ` : ''}${err.message}`;
        });
        
        return {
          success: false,
          errors,
        };
      }
      
      return {
        success: false,
        errors: [`Unexpected validation error for ${entityType}: ${error}`],
      };
    }
  }

  /**
   * Validates a single entity and throws on failure
   */
  static validateOrThrow<T>(schema: z.ZodSchema<T>, data: unknown, entityType: string): T {
    const result = this.validate(schema, data, entityType);
    
    if (!result.success) {
      throw new ValidationError(
        `Validation failed for ${entityType}: ${result.errors?.join(', ')}`,
        entityType,
        result.errors
      );
    }
    
    return result.data!;
  }

  /**
   * Validates an array of entities against a schema
   */
  static validateBatch<T>(schema: z.ZodSchema<T>, dataArray: unknown[], entityType: string): ValidationResult<T[]> {
    const validatedItems: T[] = [];
    const allErrors: string[] = [];
    
    for (let i = 0; i < dataArray.length; i++) {
      const result = this.validate(schema, dataArray[i], `${entityType}[${i}]`);
      
      if (result.success) {
        validatedItems.push(result.data!);
      } else {
        allErrors.push(...(result.errors || []));
      }
    }
    
    if (allErrors.length > 0) {
      return {
        success: false,
        errors: allErrors,
      };
    }
    
    return {
      success: true,
      data: validatedItems,
    };
  }

  /**
   * Validates an array of entities and throws on any failure
   */
  static validateBatchOrThrow<T>(schema: z.ZodSchema<T>, dataArray: unknown[], entityType: string): T[] {
    const result = this.validateBatch(schema, dataArray, entityType);
    
    if (!result.success) {
      throw new ValidationError(
        `Batch validation failed for ${entityType}: ${result.errors?.join(', ')}`,
        entityType,
        result.errors
      );
    }
    
    return result.data!;
  }

  /**
   * Safely validates data and returns a result without throwing
   */
  static safeValidate<T>(schema: z.ZodSchema<T>, data: unknown): z.SafeParseReturnType<unknown, T> {
    return schema.safeParse(data);
  }

  /**
   * Validates partial data against a schema (useful for updates)
   */
  static validatePartial<T extends z.ZodRawShape>(
    schema: z.ZodObject<T>, 
    data: unknown, 
    entityType: string
  ): ValidationResult<Partial<z.infer<z.ZodObject<T>>>> {
    const partialSchema = schema.partial();
    return this.validate(partialSchema, data, entityType);
  }

  /**
   * Creates a validator function for a specific schema
   */
  static createValidator<T>(schema: z.ZodSchema<T>, entityType: string) {
    return {
      validate: (data: unknown) => this.validate(schema, data, entityType),
      validateOrThrow: (data: unknown) => this.validateOrThrow(schema, data, entityType),
      validateBatch: (dataArray: unknown[]) => this.validateBatch(schema, dataArray, entityType),
      validateBatchOrThrow: (dataArray: unknown[]) => this.validateBatchOrThrow(schema, dataArray, entityType),
      safeValidate: (data: unknown) => this.safeValidate(schema, data),
    };
  }
}

// Utility function to extract error messages from ZodError
export function extractZodErrorMessages(error: z.ZodError): string[] {
  return error.errors.map(err => {
    const path = err.path.join('.');
    return `${path ? `${path}: ` : ''}${err.message}`;
  });
}

// Utility function to check if an error is a validation error
export function isValidationError(error: unknown): error is ValidationError {
  return error instanceof ValidationError;
}

// Utility function to format validation errors for logging
export function formatValidationError(error: ValidationError): string {
  const errorsText = error.validationErrors?.join('; ') || 'Unknown validation error';
  return `[${error.entityType}] ${error.message} - Details: ${errorsText}`;
}