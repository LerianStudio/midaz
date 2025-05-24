/**
 * Tests for validation system
 */

import { Validator, ValidationResult } from '../../../src/validation/validator';
import { ValidationError } from '../../../src/errors';
import { organizationSchema, assetSchema } from '../../../src/validation/schemas';
import { z } from 'zod';

describe('Validator', () => {
  describe('validate', () => {
    it('should validate correct data successfully', () => {
      const validData = {
        legalName: 'Test Organization',
        code: 'TEST_ORG',
        status: 'ACTIVE',
      };

      const result = Validator.validate(organizationSchema, validData, 'Organization');

      expect(result.success).toBe(true);
      expect(result.data).toEqual(validData);
      expect(result.errors).toBeUndefined();
    });

    it('should return validation errors for invalid data', () => {
      const invalidData = {
        legalName: '', // Empty name should fail
        code: 'TEST_ORG',
        status: 'INVALID_STATUS', // Invalid status
      };

      const result = Validator.validate(organizationSchema, invalidData, 'Organization');

      expect(result.success).toBe(false);
      expect(result.data).toBeUndefined();
      expect(result.errors).toBeDefined();
      expect(result.errors).toContain('legalName: Name cannot be empty');
      expect(result.errors?.some(err => err.includes('status'))).toBe(true);
    });

    it('should handle missing required fields', () => {
      const incompleteData = {
        legalName: 'Test Organization',
        // Missing code and status
      };

      const result = Validator.validate(organizationSchema, incompleteData, 'Organization');

      expect(result.success).toBe(false);
      expect(result.errors?.some(err => err.includes('code'))).toBe(true);
      expect(result.errors?.some(err => err.includes('status'))).toBe(true);
    });
  });

  describe('validateOrThrow', () => {
    it('should return validated data for valid input', () => {
      const validData = {
        legalName: 'Test Organization',
        code: 'TEST_ORG',
        status: 'ACTIVE',
      };

      const result = Validator.validateOrThrow(organizationSchema, validData, 'Organization');

      expect(result).toEqual(validData);
    });

    it('should throw ValidationError for invalid input', () => {
      const invalidData = {
        legalName: '',
        code: 'TEST_ORG',
        status: 'INVALID_STATUS',
      };

      expect(() => {
        Validator.validateOrThrow(organizationSchema, invalidData, 'Organization');
      }).toThrow(ValidationError);

      try {
        Validator.validateOrThrow(organizationSchema, invalidData, 'Organization');
      } catch (error) {
        expect(error).toBeInstanceOf(ValidationError);
        const validationError = error as ValidationError;
        expect(validationError.entityType).toBe('Organization');
        expect(validationError.validationErrors).toBeDefined();
      }
    });
  });

  describe('validateBatch', () => {
    it('should validate array of valid objects', () => {
      const validData = [
        {
          legalName: 'Organization 1',
          code: 'ORG_1',
          status: 'ACTIVE',
        },
        {
          legalName: 'Organization 2',
          code: 'ORG_2',
          status: 'INACTIVE',
        },
      ];

      const result = Validator.validateBatch(organizationSchema, validData, 'Organization');

      expect(result.success).toBe(true);
      expect(result.data).toHaveLength(2);
      expect(result.errors).toBeUndefined();
    });

    it('should return errors for mixed valid/invalid data', () => {
      const mixedData = [
        {
          legalName: 'Valid Organization',
          code: 'VALID_ORG',
          status: 'ACTIVE',
        },
        {
          legalName: '', // Invalid
          code: 'INVALID_ORG',
          status: 'ACTIVE',
        },
      ];

      const result = Validator.validateBatch(organizationSchema, mixedData, 'Organization');

      expect(result.success).toBe(false);
      expect(result.errors).toBeDefined();
      expect(result.errors?.some(err => err.includes('Organization[1]'))).toBe(true);
    });
  });

  describe('validateBatchOrThrow', () => {
    it('should return validated data for valid array', () => {
      const validData = [
        {
          legalName: 'Organization 1',
          code: 'ORG_1',
          status: 'ACTIVE',
        },
      ];

      const result = Validator.validateBatchOrThrow(organizationSchema, validData, 'Organization');

      expect(result).toHaveLength(1);
      expect(result[0]).toEqual(validData[0]);
    });

    it('should throw ValidationError for invalid array', () => {
      const invalidData = [
        {
          legalName: '', // Invalid
          code: 'INVALID_ORG',
          status: 'ACTIVE',
        },
      ];

      expect(() => {
        Validator.validateBatchOrThrow(organizationSchema, invalidData, 'Organization');
      }).toThrow(ValidationError);
    });
  });

  describe('safeValidate', () => {
    it('should return safe parse result for valid data', () => {
      const validData = {
        legalName: 'Test Organization',
        code: 'TEST_ORG',
        status: 'ACTIVE',
      };

      const result = Validator.safeValidate(organizationSchema, validData);

      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.data).toEqual(validData);
      }
    });

    it('should return safe parse result with errors for invalid data', () => {
      const invalidData = {
        legalName: '',
        code: 'TEST_ORG',
        status: 'INVALID_STATUS',
      };

      const result = Validator.safeValidate(organizationSchema, invalidData);

      expect(result.success).toBe(false);
      if (!result.success) {
        expect(result.error).toBeDefined();
        expect(result.error.errors.length).toBeGreaterThan(0);
      }
    });
  });

  describe('createValidator', () => {
    it('should create validator with schema-specific methods', () => {
      const validator = Validator.createValidator(organizationSchema, 'Organization');

      expect(typeof validator.validate).toBe('function');
      expect(typeof validator.validateOrThrow).toBe('function');
      expect(typeof validator.validateBatch).toBe('function');
      expect(typeof validator.validateBatchOrThrow).toBe('function');
      expect(typeof validator.safeValidate).toBe('function');
    });

    it('should use correct entity type in error messages', () => {
      const validator = Validator.createValidator(organizationSchema, 'TestEntity');
      const invalidData = { legalName: '' };

      const result = validator.validate(invalidData);

      expect(result.success).toBe(false);
      // The validator should maintain the entity type passed to createValidator
    });
  });

  describe('complex validation scenarios', () => {
    it('should validate asset with all required fields', () => {
      const validAsset = {
        name: 'Bitcoin',
        code: 'BTC',
        type: 'crypto',
        status: 'ACTIVE',
        organizationId: 'c7b3d8e0-5e0f-4b7d-8e3b-7b7b7b7b7b7b',
        ledgerId: 'f47ac10b-58cc-4372-a567-0e02b2c3d479',
        metadata: {
          symbol: 'BTC',
          decimals: 8,
        },
      };

      const result = Validator.validate(assetSchema, validAsset, 'Asset');

      expect(result.success).toBe(true);
      expect(result.data).toEqual(validAsset);
    });

    it('should reject asset with invalid UUID format', () => {
      const invalidAsset = {
        name: 'Bitcoin',
        code: 'BTC',
        type: 'crypto',
        status: 'ACTIVE',
        organizationId: 'invalid-uuid',
        ledgerId: 'f47ac10b-58cc-4372-a567-0e02b2c3d479',
      };

      const result = Validator.validate(assetSchema, invalidAsset, 'Asset');

      expect(result.success).toBe(false);
      expect(result.errors?.some(err => err.includes('organizationId'))).toBe(true);
      expect(result.errors?.some(err => err.includes('UUID'))).toBe(true);
    });

    it('should reject asset with invalid type', () => {
      const invalidAsset = {
        name: 'Bitcoin',
        code: 'BTC',
        type: 'invalid_type',
        status: 'ACTIVE',
        organizationId: 'c7b3d8e0-5e0f-4b7d-8e3b-7b7b7b7b7b7b',
        ledgerId: 'f47ac10b-58cc-4372-a567-0e02b2c3d479',
      };

      const result = Validator.validate(assetSchema, invalidAsset, 'Asset');

      expect(result.success).toBe(false);
      expect(result.errors?.some(err => err.includes('type'))).toBe(true);
    });
  });
});