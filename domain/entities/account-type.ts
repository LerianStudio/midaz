/**
 * Account Type Domain Entity
 * 
 * This file defines the complete AccountType entity structure for the Accounting plugin,
 * including domain validation, usage tracking, audit fields, and all related interfaces.
 * 
 * @module AccountType
 * @version 1.0.0
 */

/**
 * Account domain types that determine validation and control mechanisms
 */
export enum AccountDomain {
  /** Internal ledger management with built-in validation */
  LEDGER = 'ledger',
  /** External system management with custom validation */
  EXTERNAL = 'external',
}

/**
 * Account type status for lifecycle management
 */
export enum AccountTypeStatus {
  /** Account type is active and available for use */
  ACTIVE = 'active',
  /** Account type is temporarily disabled */
  INACTIVE = 'inactive',
  /** Account type is in draft state during creation */
  DRAFT = 'draft',
  /** Account type has validation errors */
  INVALID = 'invalid',
}

/**
 * Core Account Type entity representing accounting classification
 * 
 * Account types define the chart of accounts structure and determine
 * how accounts are validated and controlled within the ledger system.
 */
export interface AccountType {
  /** Unique identifier for the account type */
  readonly id: string;
  
  /** Human-readable name of the account type */
  name: string;
  
  /** Detailed description of the account type's purpose */
  description: string;
  
  /** 
   * User-defined identifier for ledger integration
   * Must be unique across all account types
   * Used as reference when creating accounts in the Ledger
   */
  keyValue: string;
  
  /** 
   * Domain that controls validation behavior
   * - ledger: Internal ledger validation and control
   * - external: External system validation and control
   */
  domain: AccountDomain;
  
  /** Current status of the account type */
  status: AccountTypeStatus;
  
  /** Usage tracking and analytics */
  usage: AccountTypeUsage;
  
  /** Validation rules and compliance settings */
  validation: AccountTypeValidation;
  
  /** Audit and timestamp information */
  audit: AccountTypeAudit;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Usage tracking and analytics for account types
 */
export interface AccountTypeUsage {
  /** Total number of times this account type has been used */
  usageCount: number;
  
  /** Number of accounts currently linked to this type */
  linkedAccounts: number;
  
  /** Timestamp of last usage */
  lastUsed?: Date;
  
  /** Monthly usage trend data */
  monthlyUsage: MonthlyUsageData[];
  
  /** Performance metrics */
  performance: AccountTypePerformance;
}

/**
 * Monthly usage statistics
 */
export interface MonthlyUsageData {
  /** Month in YYYY-MM format */
  month: string;
  
  /** Usage count for the month */
  count: number;
  
  /** Percentage change from previous month */
  changePercent: number;
}

/**
 * Performance metrics for account types
 */
export interface AccountTypePerformance {
  /** Average processing time for operations */
  avgProcessingTime: number;
  
  /** Success rate percentage */
  successRate: number;
  
  /** Error rate percentage */
  errorRate: number;
  
  /** Compliance score (0-100) */
  complianceScore: number;
}

/**
 * Validation rules and compliance settings
 */
export interface AccountTypeValidation {
  /** Whether the account type is currently valid */
  isValid: boolean;
  
  /** List of validation errors if any */
  errors: ValidationError[];
  
  /** List of validation warnings */
  warnings: ValidationWarning[];
  
  /** Compliance requirements */
  compliance: ComplianceRequirements;
  
  /** Last validation timestamp */
  lastValidated: Date;
}

/**
 * Validation error details
 */
export interface ValidationError {
  /** Error code for programmatic handling */
  code: string;
  
  /** Human-readable error message */
  message: string;
  
  /** Field that caused the error */
  field?: string;
  
  /** Severity level of the error */
  severity: 'critical' | 'high' | 'medium' | 'low';
  
  /** Timestamp when error occurred */
  timestamp: Date;
}

/**
 * Validation warning details
 */
export interface ValidationWarning {
  /** Warning code for programmatic handling */
  code: string;
  
  /** Human-readable warning message */
  message: string;
  
  /** Field that triggered the warning */
  field?: string;
  
  /** Timestamp when warning occurred */
  timestamp: Date;
}

/**
 * Compliance requirements and settings
 */
export interface ComplianceRequirements {
  /** Whether compliance validation is required */
  required: boolean;
  
  /** Regulatory standards to comply with */
  standards: string[];
  
  /** Audit trail requirements */
  auditTrail: boolean;
  
  /** Data retention requirements in days */
  retentionDays: number;
  
  /** Approval workflow requirements */
  requiresApproval: boolean;
}

/**
 * Audit trail and timestamp information
 */
export interface AccountTypeAudit {
  /** Creation timestamp */
  createdAt: Date;
  
  /** Last update timestamp */
  updatedAt: Date;
  
  /** Soft deletion timestamp */
  deletedAt?: Date;
  
  /** User who created the account type */
  createdBy: string;
  
  /** User who last updated the account type */
  updatedBy: string;
  
  /** User who deleted the account type */
  deletedBy?: string;
  
  /** Version number for optimistic locking */
  version: number;
  
  /** Change history for audit trail */
  changeHistory: AccountTypeChange[];
}

/**
 * Change history entry for audit trail
 */
export interface AccountTypeChange {
  /** Unique identifier for the change */
  id: string;
  
  /** Type of change made */
  changeType: 'created' | 'updated' | 'deleted' | 'status_changed';
  
  /** Fields that were changed */
  changedFields: string[];
  
  /** Previous values before change */
  previousValues: Record<string, any>;
  
  /** New values after change */
  newValues: Record<string, any>;
  
  /** User who made the change */
  changedBy: string;
  
  /** Timestamp of the change */
  changedAt: Date;
  
  /** Reason for the change */
  reason?: string;
}

/**
 * Data Transfer Object for creating account types
 */
export interface CreateAccountTypeInput {
  /** Human-readable name of the account type */
  name: string;
  
  /** Detailed description of the account type's purpose */
  description: string;
  
  /** 
   * User-defined identifier for ledger integration
   * Must be unique across all account types
   */
  keyValue: string;
  
  /** Domain that controls validation behavior */
  domain: AccountDomain;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Data Transfer Object for updating account types
 */
export interface UpdateAccountTypeInput {
  /** Human-readable name of the account type */
  name?: string;
  
  /** Detailed description of the account type's purpose */
  description?: string;
  
  /** Current status of the account type */
  status?: AccountTypeStatus;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Validation result for account type operations
 */
export interface AccountTypeValidationResult {
  /** Whether the validation passed */
  isValid: boolean;
  
  /** List of validation errors */
  errors: ValidationError[];
  
  /** List of validation warnings */
  warnings: ValidationWarning[];
  
  /** Validation context information */
  context: ValidationContext;
}

/**
 * Validation context for detailed error reporting
 */
export interface ValidationContext {
  /** Operation being validated */
  operation: 'create' | 'update' | 'delete';
  
  /** Account type being validated */
  accountType: AccountType | CreateAccountTypeInput | UpdateAccountTypeInput;
  
  /** Additional validation parameters */
  parameters: Record<string, any>;
  
  /** Timestamp of validation */
  timestamp: Date;
}

/**
 * Analytics data for account types
 */
export interface AccountTypeAnalytics {
  /** Overview statistics */
  overview: AccountTypeOverview;
  
  /** Usage analytics */
  usage: AccountTypeUsageAnalytics;
  
  /** Performance metrics */
  performance: AccountTypePerformanceAnalytics;
  
  /** Compliance analytics */
  compliance: AccountTypeComplianceAnalytics;
  
  /** Trend analysis */
  trends: AccountTypeTrends;
}

/**
 * Overview statistics for account types
 */
export interface AccountTypeOverview {
  /** Total number of account types */
  total: number;
  
  /** Number of active account types */
  active: number;
  
  /** Number of inactive account types */
  inactive: number;
  
  /** Number of draft account types */
  draft: number;
  
  /** Number of invalid account types */
  invalid: number;
  
  /** Breakdown by domain */
  byDomain: DomainBreakdown;
}

/**
 * Domain breakdown statistics
 */
export interface DomainBreakdown {
  /** Number of ledger domain account types */
  ledger: number;
  
  /** Number of external domain account types */
  external: number;
}

/**
 * Usage analytics for account types
 */
export interface AccountTypeUsageAnalytics {
  /** Total usage across all account types */
  totalUsage: number;
  
  /** Most used account types */
  mostUsed: AccountTypeUsageSummary[];
  
  /** Least used account types */
  leastUsed: AccountTypeUsageSummary[];
  
  /** Usage distribution by domain */
  usageByDomain: Record<AccountDomain, number>;
  
  /** Monthly usage trends */
  monthlyTrends: MonthlyUsageData[];
}

/**
 * Usage summary for individual account types
 */
export interface AccountTypeUsageSummary {
  /** Account type identifier */
  id: string;
  
  /** Account type name */
  name: string;
  
  /** Key value */
  keyValue: string;
  
  /** Usage count */
  usageCount: number;
  
  /** Usage percentage */
  percentage: number;
}

/**
 * Performance analytics for account types
 */
export interface AccountTypePerformanceAnalytics {
  /** Overall performance score */
  overallScore: number;
  
  /** Performance by account type */
  byAccountType: AccountTypePerformanceSummary[];
  
  /** Performance trends over time */
  trends: PerformanceTrend[];
  
  /** Benchmark comparisons */
  benchmarks: PerformanceBenchmark[];
}

/**
 * Performance summary for individual account types
 */
export interface AccountTypePerformanceSummary {
  /** Account type identifier */
  id: string;
  
  /** Account type name */
  name: string;
  
  /** Key value */
  keyValue: string;
  
  /** Performance metrics */
  performance: AccountTypePerformance;
}

/**
 * Performance trend data
 */
export interface PerformanceTrend {
  /** Date of measurement */
  date: Date;
  
  /** Performance score */
  score: number;
  
  /** Success rate */
  successRate: number;
  
  /** Error rate */
  errorRate: number;
}

/**
 * Performance benchmark data
 */
export interface PerformanceBenchmark {
  /** Benchmark name */
  name: string;
  
  /** Benchmark value */
  value: number;
  
  /** Current value for comparison */
  current: number;
  
  /** Percentage difference */
  difference: number;
}

/**
 * Compliance analytics for account types
 */
export interface AccountTypeComplianceAnalytics {
  /** Overall compliance score */
  overallScore: number;
  
  /** Compliance by account type */
  byAccountType: AccountTypeComplianceSummary[];
  
  /** Compliance trends over time */
  trends: ComplianceTrend[];
  
  /** Violation summary */
  violations: ComplianceViolationSummary;
}

/**
 * Compliance summary for individual account types
 */
export interface AccountTypeComplianceSummary {
  /** Account type identifier */
  id: string;
  
  /** Account type name */
  name: string;
  
  /** Key value */
  keyValue: string;
  
  /** Compliance score */
  score: number;
  
  /** Number of violations */
  violations: number;
}

/**
 * Compliance trend data
 */
export interface ComplianceTrend {
  /** Date of measurement */
  date: Date;
  
  /** Compliance score */
  score: number;
  
  /** Number of violations */
  violations: number;
}

/**
 * Compliance violation summary
 */
export interface ComplianceViolationSummary {
  /** Total number of violations */
  total: number;
  
  /** Critical violations */
  critical: number;
  
  /** High severity violations */
  high: number;
  
  /** Medium severity violations */
  medium: number;
  
  /** Low severity violations */
  low: number;
}

/**
 * Trend analysis for account types
 */
export interface AccountTypeTrends {
  /** Growth trends */
  growth: GrowthTrend[];
  
  /** Usage trends */
  usage: UsageTrend[];
  
  /** Performance trends */
  performance: PerformanceTrend[];
  
  /** Compliance trends */
  compliance: ComplianceTrend[];
}

/**
 * Growth trend data
 */
export interface GrowthTrend {
  /** Date of measurement */
  date: Date;
  
  /** Number of account types */
  count: number;
  
  /** Growth rate percentage */
  growthRate: number;
}

/**
 * Usage trend data
 */
export interface UsageTrend {
  /** Date of measurement */
  date: Date;
  
  /** Total usage count */
  usage: number;
  
  /** Usage growth rate percentage */
  growthRate: number;
}
