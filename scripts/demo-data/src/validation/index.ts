// Export all validation schemas
export * from './schemas';

// Export validator utilities
export * from './validator';

// Export specific validators for each entity type
import { Validator } from './validator';
import {
  organizationSchema,
  ledgerSchema,
  assetSchema,
  portfolioSchema,
  segmentSchema,
  accountSchema,
  transactionSchema,
  generationOptionsSchema,
} from './schemas';

// Create pre-configured validators for each entity type
export const organizationValidator = Validator.createValidator(organizationSchema, 'Organization');
export const ledgerValidator = Validator.createValidator(ledgerSchema, 'Ledger');
export const assetValidator = Validator.createValidator(assetSchema, 'Asset');
export const portfolioValidator = Validator.createValidator(portfolioSchema, 'Portfolio');
export const segmentValidator = Validator.createValidator(segmentSchema, 'Segment');
export const accountValidator = Validator.createValidator(accountSchema, 'Account');
export const transactionValidator = Validator.createValidator(transactionSchema, 'Transaction');
export const generationOptionsValidator = Validator.createValidator(generationOptionsSchema, 'GenerationOptions');

// Utility functions for common validation patterns
export function validateGenerationData(data: {
  organizations?: unknown[];
  ledgers?: unknown[];
  assets?: unknown[];
  portfolios?: unknown[];
  segments?: unknown[];
  accounts?: unknown[];
  transactions?: unknown[];
}) {
  const results: { [key: string]: any } = {};
  const errors: string[] = [];

  if (data.organizations) {
    const result = organizationValidator.validateBatch(data.organizations);
    if (result.success) {
      results.organizations = result.data;
    } else {
      errors.push(...(result.errors || []));
    }
  }

  if (data.ledgers) {
    const result = ledgerValidator.validateBatch(data.ledgers);
    if (result.success) {
      results.ledgers = result.data;
    } else {
      errors.push(...(result.errors || []));
    }
  }

  if (data.assets) {
    const result = assetValidator.validateBatch(data.assets);
    if (result.success) {
      results.assets = result.data;
    } else {
      errors.push(...(result.errors || []));
    }
  }

  if (data.portfolios) {
    const result = portfolioValidator.validateBatch(data.portfolios);
    if (result.success) {
      results.portfolios = result.data;
    } else {
      errors.push(...(result.errors || []));
    }
  }

  if (data.segments) {
    const result = segmentValidator.validateBatch(data.segments);
    if (result.success) {
      results.segments = result.data;
    } else {
      errors.push(...(result.errors || []));
    }
  }

  if (data.accounts) {
    const result = accountValidator.validateBatch(data.accounts);
    if (result.success) {
      results.accounts = result.data;
    } else {
      errors.push(...(result.errors || []));
    }
  }

  if (data.transactions) {
    const result = transactionValidator.validateBatch(data.transactions);
    if (result.success) {
      results.transactions = result.data;
    } else {
      errors.push(...(result.errors || []));
    }
  }

  return {
    success: errors.length === 0,
    data: results,
    errors: errors.length > 0 ? errors : undefined,
  };
}