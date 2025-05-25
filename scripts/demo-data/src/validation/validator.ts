/**
 * Validation utilities for the demo data generator
 */

import { z } from 'zod';

export interface ValidationResult {
  isValid: boolean;
  errors?: string[];
}

export class Validator {
  static validate<T>(schema: z.ZodSchema<T>, data: unknown): ValidationResult {
    try {
      schema.parse(data);
      return { isValid: true };
    } catch (error) {
      if (error instanceof z.ZodError) {
        return {
          isValid: false,
          errors: error.errors.map(e => `${e.path.join('.')}: ${e.message}`)
        };
      }
      return {
        isValid: false,
        errors: ['Unknown validation error']
      };
    }
  }

  static validateAsync<T>(schema: z.ZodSchema<T>, data: unknown): Promise<ValidationResult> {
    return Promise.resolve(this.validate(schema, data));
  }

  static isValid<T>(schema: z.ZodSchema<T>, data: unknown): boolean {
    return this.validate(schema, data).isValid;
  }

  static validateOrThrow<T>(schema: z.ZodSchema<T>, data: unknown): T {
    return schema.parse(data);
  }

  static validateBatchOrThrow<T>(schema: z.ZodSchema<T>, items: unknown[]): T[] {
    return items.map(item => schema.parse(item));
  }
}

// Common validation schemas
export const schemas = {
  positiveNumber: z.number().positive(),
  nonEmptyString: z.string().min(1),
  uuid: z.string().uuid(),
  email: z.string().email(),
  url: z.string().url(),
  dateString: z.string().datetime(),
  
  // Business-specific schemas
  organizationName: z.string().min(2).max(255),
  ledgerName: z.string().min(2).max(100),
  assetCode: z.string().length(3).toUpperCase(),
  accountAlias: z.string().min(3).max(50),
  amount: z.number().positive().finite(),
  
  // Configuration schemas
  volumeSize: z.enum(['test', 'small', 'medium', 'large']),
  
  pagination: z.object({
    page: z.number().int().positive().optional(),
    pageSize: z.number().int().positive().max(1000).optional(),
    cursor: z.string().optional()
  })
};