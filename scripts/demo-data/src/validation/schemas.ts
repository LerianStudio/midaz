import { z } from 'zod';

// Common validation schemas
export const uuidSchema = z.string().uuid('Must be a valid UUID');
export const codeSchema = z.string().min(1, 'Code cannot be empty').max(100, 'Code too long');
export const nameSchema = z.string().min(1, 'Name cannot be empty').max(255, 'Name too long');
export const statusSchema = z.enum(['ACTIVE', 'INACTIVE']);

// Organization validation schema
export const organizationSchema = z.object({
  legalName: nameSchema,
  doingBusinessAs: nameSchema.optional(),
  code: codeSchema,
  status: statusSchema,
  metadata: z.record(z.any()).optional(),
});

// Ledger validation schema
export const ledgerSchema = z.object({
  name: nameSchema,
  code: codeSchema,
  status: statusSchema,
  organizationId: uuidSchema,
  metadata: z.record(z.any()).optional(),
});

// Asset validation schema
export const assetSchema = z.object({
  name: nameSchema,
  code: codeSchema,
  type: z.enum(['currency', 'crypto', 'commodity']),
  status: statusSchema,
  organizationId: uuidSchema,
  ledgerId: uuidSchema,
  metadata: z.record(z.any()).optional(),
});

// Portfolio validation schema
export const portfolioSchema = z.object({
  name: nameSchema,
  code: codeSchema,
  status: statusSchema,
  organizationId: uuidSchema,
  ledgerId: uuidSchema,
  metadata: z.record(z.any()).optional(),
});

// Segment validation schema
export const segmentSchema = z.object({
  name: nameSchema,
  code: codeSchema,
  status: statusSchema,
  organizationId: uuidSchema,
  ledgerId: uuidSchema,
  metadata: z.record(z.any()).optional(),
});

// Account validation schema
export const accountSchema = z.object({
  name: nameSchema,
  alias: z.string().min(1, 'Alias cannot be empty').max(100, 'Alias too long'),
  type: z.enum(['deposit', 'savings', 'loans', 'external']),
  status: statusSchema,
  organizationId: uuidSchema,
  ledgerId: uuidSchema,
  portfolioId: uuidSchema.optional(),
  productId: uuidSchema.optional(),
  metadata: z.record(z.any()).optional(),
});

// Transaction operation validation schema
export const transactionOperationSchema = z.object({
  type: z.enum(['debit', 'credit']),
  value: z.number().positive('Value must be positive'),
  assetCode: codeSchema,
  account: z.object({
    id: uuidSchema,
    alias: z.string().min(1, 'Account alias cannot be empty'),
  }),
});

// Transaction validation schema
export const transactionSchema = z.object({
  parentTransactionId: uuidSchema.optional(),
  description: z.string().min(1, 'Description cannot be empty').max(500, 'Description too long'),
  template: z.string().min(1, 'Template cannot be empty'),
  status: statusSchema,
  organizationId: uuidSchema,
  ledgerId: uuidSchema,
  operations: z.array(transactionOperationSchema).min(2, 'Transaction must have at least 2 operations'),
  metadata: z.record(z.any()).optional(),
});

// Batch validation schemas
export const batchOrganizationSchema = z.array(organizationSchema);
export const batchLedgerSchema = z.array(ledgerSchema);
export const batchAssetSchema = z.array(assetSchema);
export const batchPortfolioSchema = z.array(portfolioSchema);
export const batchSegmentSchema = z.array(segmentSchema);
export const batchAccountSchema = z.array(accountSchema);
export const batchTransactionSchema = z.array(transactionSchema);

// Generation options validation
export const generationOptionsSchema = z.object({
  organizations: z.number().min(1, 'Must generate at least 1 organization').max(100, 'Too many organizations'),
  ledgersPerOrg: z.number().min(1, 'Must generate at least 1 ledger per organization').max(50, 'Too many ledgers'),
  assetsPerLedger: z.number().min(1, 'Must generate at least 1 asset per ledger').max(20, 'Too many assets'),
  portfoliosPerLedger: z.number().min(1, 'Must generate at least 1 portfolio per ledger').max(10, 'Too many portfolios'),
  segmentsPerLedger: z.number().min(0, 'Segments cannot be negative').max(10, 'Too many segments'),
  accountsPerLedger: z.number().min(2, 'Must generate at least 2 accounts per ledger').max(100, 'Too many accounts'),
  transactionsPerLedger: z.number().min(0, 'Transactions cannot be negative').max(1000, 'Too many transactions'),
  batchSize: z.number().min(1, 'Batch size must be at least 1').max(100, 'Batch size too large'),
  retryAttempts: z.number().min(0, 'Retry attempts cannot be negative').max(10, 'Too many retry attempts'),
  retryDelayMs: z.number().min(0, 'Retry delay cannot be negative').max(30000, 'Retry delay too long'),
});

// Export type definitions
export type OrganizationData = z.infer<typeof organizationSchema>;
export type LedgerData = z.infer<typeof ledgerSchema>;
export type AssetData = z.infer<typeof assetSchema>;
export type PortfolioData = z.infer<typeof portfolioSchema>;
export type SegmentData = z.infer<typeof segmentSchema>;
export type AccountData = z.infer<typeof accountSchema>;
export type TransactionData = z.infer<typeof transactionSchema>;
export type TransactionOperationData = z.infer<typeof transactionOperationSchema>;
export type GenerationOptionsData = z.infer<typeof generationOptionsSchema>;