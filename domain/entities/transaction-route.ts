/**
 * Transaction Route Domain Entity
 * 
 * This file defines the complete TransactionRoute entity structure for the Accounting plugin,
 * including operations, metadata, validation, and all related interfaces for accounting
 * transaction template management.
 * 
 * @module TransactionRoute
 * @version 1.0.0
 */

import { OperationRoute } from './operation-route';
import { ValidationError, ValidationWarning } from './account-type';

/**
 * Transaction route status for lifecycle management
 */
export enum TransactionRouteStatus {
  /** Transaction route is active and ready for use */
  ACTIVE = 'active',
  /** Transaction route is temporarily disabled */
  INACTIVE = 'inactive',
  /** Transaction route is in draft state during creation */
  DRAFT = 'draft',
  /** Transaction route has validation errors */
  INVALID = 'invalid',
  /** Transaction route is archived but preserved for history */
  ARCHIVED = 'archived',
}

/**
 * Transaction route categories for organization and filtering
 */
export enum TransactionRouteCategory {
  /** Standard account-to-account transfers */
  TRANSFERS = 'transfers',
  /** Payment processing routes */
  PAYMENTS = 'payments',
  /** External wire transfers */
  EXTERNAL_TRANSFERS = 'external_transfers',
  /** Deposit operations */
  DEPOSITS = 'deposits',
  /** Withdrawal operations */
  WITHDRAWALS = 'withdrawals',
  /** Fee collection routes */
  FEES = 'fees',
  /** Interest calculations */
  INTEREST = 'interest',
  /** Adjustment entries */
  ADJUSTMENTS = 'adjustments',
  /** Regulatory compliance operations */
  COMPLIANCE = 'compliance',
  /** Custom business-specific routes */
  CUSTOM = 'custom',
}

/**
 * Priority levels for transaction route processing
 */
export enum TransactionRoutePriority {
  /** Low priority for batch processing */
  LOW = 'low',
  /** Normal priority for standard operations */
  NORMAL = 'normal',
  /** High priority for urgent operations */
  HIGH = 'high',
  /** Critical priority for immediate processing */
  CRITICAL = 'critical',
}

/**
 * Core Transaction Route entity representing accounting transaction templates
 * 
 * Transaction routes define reusable templates for accounting operations,
 * including source and destination account mappings, validation rules,
 * and business logic for financial transactions.
 */
export interface TransactionRoute {
  /** Unique identifier for the transaction route */
  readonly id: string;
  
  /** Short, descriptive title for the transaction route */
  title: string;
  
  /** Detailed description of the route's purpose and usage */
  description: string;
  
  /** Category classification for organization */
  category: TransactionRouteCategory;
  
  /** Current status of the transaction route */
  status: TransactionRouteStatus;
  
  /** Processing priority level */
  priority: TransactionRoutePriority;
  
  /** Collection of operation routes that define the transaction flow */
  operationRoutes: OperationRoute[];
  
  /** Business logic and validation rules */
  rules: TransactionRouteRules;
  
  /** Usage tracking and analytics */
  usage: TransactionRouteUsage;
  
  /** Validation results and compliance status */
  validation: TransactionRouteValidation;
  
  /** Audit and timestamp information */
  audit: TransactionRouteAudit;
  
  /** Template and versioning information */
  template: TransactionRouteTemplate;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Business rules and validation logic for transaction routes
 */
export interface TransactionRouteRules {
  /** Whether the route requires manual approval */
  requiresApproval: boolean;
  
  /** Minimum transaction amount */
  minimumAmount?: number;
  
  /** Maximum transaction amount */
  maximumAmount?: number;
  
  /** Automatic validation on transaction creation */
  autoValidate: boolean;
  
  /** Currency restrictions */
  allowedCurrencies: string[];
  
  /** Time-based restrictions */
  timeRestrictions: TimeRestrictions;
  
  /** Amount-based fee calculations */
  feeRules: FeeRule[];
  
  /** Compliance requirements */
  complianceLevel: 'low' | 'medium' | 'high' | 'critical';
  
  /** Custom validation rules */
  customRules: CustomValidationRule[];
}

/**
 * Time-based restrictions for transaction processing
 */
export interface TimeRestrictions {
  /** Whether the route is available 24/7 */
  always: boolean;
  
  /** Business hours when route is available */
  businessHours?: BusinessHours;
  
  /** Days of week when route is available */
  allowedDays: number[]; // 0-6, Sunday-Saturday
  
  /** Holidays when route is not available */
  excludedHolidays: string[];
  
  /** Timezone for time calculations */
  timezone: string;
}

/**
 * Business hours definition
 */
export interface BusinessHours {
  /** Start time in HH:mm format */
  startTime: string;
  
  /** End time in HH:mm format */
  endTime: string;
  
  /** Whether to span multiple days */
  spanMultipleDays: boolean;
}

/**
 * Fee calculation rules
 */
export interface FeeRule {
  /** Unique identifier for the fee rule */
  id: string;
  
  /** Human-readable name */
  name: string;
  
  /** Fee calculation type */
  type: 'fixed' | 'percentage' | 'tiered' | 'custom';
  
  /** Fee amount or percentage */
  value: number;
  
  /** Minimum fee amount */
  minimum?: number;
  
  /** Maximum fee amount */
  maximum?: number;
  
  /** Conditions for applying the fee */
  conditions: FeeCondition[];
}

/**
 * Conditions for fee application
 */
export interface FeeCondition {
  /** Field to evaluate */
  field: string;
  
  /** Comparison operator */
  operator: 'equals' | 'greater_than' | 'less_than' | 'between' | 'in';
  
  /** Value to compare against */
  value: any;
}

/**
 * Custom validation rules for business logic
 */
export interface CustomValidationRule {
  /** Unique identifier for the rule */
  id: string;
  
  /** Human-readable name */
  name: string;
  
  /** Rule description */
  description: string;
  
  /** Rule expression or script */
  expression: string;
  
  /** Error message if rule fails */
  errorMessage: string;
  
  /** Warning message if rule has issues */
  warningMessage?: string;
  
  /** Whether rule is mandatory or advisory */
  mandatory: boolean;
}

/**
 * Usage tracking and analytics for transaction routes
 */
export interface TransactionRouteUsage {
  /** Total number of times this route has been used */
  usageCount: number;
  
  /** Timestamp of last usage */
  lastUsed?: Date;
  
  /** Daily usage statistics */
  dailyUsage: DailyUsageData[];
  
  /** Performance metrics */
  performance: TransactionRoutePerformance;
  
  /** Success and failure statistics */
  statistics: TransactionRouteStatistics;
}

/**
 * Daily usage statistics
 */
export interface DailyUsageData {
  /** Date in YYYY-MM-DD format */
  date: string;
  
  /** Usage count for the day */
  count: number;
  
  /** Success count */
  successCount: number;
  
  /** Failure count */
  failureCount: number;
  
  /** Total transaction amount */
  totalAmount: number;
}

/**
 * Performance metrics for transaction routes
 */
export interface TransactionRoutePerformance {
  /** Average processing time in milliseconds */
  avgProcessingTime: number;
  
  /** 95th percentile processing time */
  p95ProcessingTime: number;
  
  /** Success rate percentage */
  successRate: number;
  
  /** Error rate percentage */
  errorRate: number;
  
  /** Throughput (transactions per minute) */
  throughput: number;
}

/**
 * Success and failure statistics
 */
export interface TransactionRouteStatistics {
  /** Total successful transactions */
  totalSuccess: number;
  
  /** Total failed transactions */
  totalFailure: number;
  
  /** Common failure reasons */
  failureReasons: FailureReason[];
  
  /** Peak usage times */
  peakUsageTimes: PeakUsageTime[];
}

/**
 * Failure reason analysis
 */
export interface FailureReason {
  /** Error code or type */
  code: string;
  
  /** Human-readable description */
  description: string;
  
  /** Number of occurrences */
  count: number;
  
  /** Percentage of total failures */
  percentage: number;
}

/**
 * Peak usage time analysis
 */
export interface PeakUsageTime {
  /** Hour of day (0-23) */
  hour: number;
  
  /** Day of week (0-6, Sunday-Saturday) */
  dayOfWeek: number;
  
  /** Usage count during this time */
  count: number;
}

/**
 * Validation results and compliance status
 */
export interface TransactionRouteValidation {
  /** Whether the route is currently valid */
  isValid: boolean;
  
  /** List of validation errors */
  errors: ValidationError[];
  
  /** List of validation warnings */
  warnings: ValidationWarning[];
  
  /** Compliance validation results */
  compliance: ComplianceValidation;
  
  /** Last validation timestamp */
  lastValidated: Date;
  
  /** Validation context */
  context: ValidationContext;
}

/**
 * Compliance validation results
 */
export interface ComplianceValidation {
  /** Overall compliance status */
  status: 'compliant' | 'non_compliant' | 'pending' | 'unknown';
  
  /** Compliance score (0-100) */
  score: number;
  
  /** Regulatory standards checked */
  standardsChecked: string[];
  
  /** Failed compliance checks */
  failedChecks: ComplianceCheck[];
  
  /** Warnings from compliance checks */
  warnings: ComplianceWarning[];
}

/**
 * Individual compliance check result
 */
export interface ComplianceCheck {
  /** Standard or regulation name */
  standard: string;
  
  /** Specific rule or requirement */
  rule: string;
  
  /** Check result status */
  status: 'pass' | 'fail' | 'warning';
  
  /** Detailed message */
  message: string;
  
  /** Severity level */
  severity: 'critical' | 'high' | 'medium' | 'low';
}

/**
 * Compliance warning details
 */
export interface ComplianceWarning {
  /** Warning code */
  code: string;
  
  /** Warning message */
  message: string;
  
  /** Recommended action */
  recommendation: string;
}

/**
 * Validation context for detailed reporting
 */
export interface ValidationContext {
  /** Operation being validated */
  operation: 'create' | 'update' | 'delete' | 'activate' | 'deactivate';
  
  /** User or system performing validation */
  validator: string;
  
  /** Additional context parameters */
  parameters: Record<string, any>;
}

/**
 * Audit trail and timestamp information
 */
export interface TransactionRouteAudit {
  /** Creation timestamp */
  createdAt: Date;
  
  /** Last update timestamp */
  updatedAt: Date;
  
  /** Soft deletion timestamp */
  deletedAt?: Date;
  
  /** User who created the route */
  createdBy: string;
  
  /** User who last updated the route */
  updatedBy: string;
  
  /** User who deleted the route */
  deletedBy?: string;
  
  /** Version number for optimistic locking */
  version: number;
  
  /** Change history for audit trail */
  changeHistory: TransactionRouteChange[];
}

/**
 * Change history entry for audit trail
 */
export interface TransactionRouteChange {
  /** Unique identifier for the change */
  id: string;
  
  /** Type of change made */
  changeType: 'created' | 'updated' | 'deleted' | 'status_changed' | 'rules_updated';
  
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
  
  /** Impact assessment */
  impact: ChangeImpact;
}

/**
 * Impact assessment for changes
 */
export interface ChangeImpact {
  /** Severity of the impact */
  severity: 'low' | 'medium' | 'high' | 'critical';
  
  /** Areas affected by the change */
  affectedAreas: string[];
  
  /** Number of dependent operations */
  dependentOperations: number;
  
  /** Risk assessment */
  riskLevel: 'low' | 'medium' | 'high';
}

/**
 * Template and versioning information
 */
export interface TransactionRouteTemplate {
  /** Whether this route is based on a template */
  isTemplate: boolean;
  
  /** Template ID if derived from template */
  templateId?: string;
  
  /** Template version used */
  templateVersion?: string;
  
  /** Template category */
  templateCategory?: string;
  
  /** Custom template properties */
  customProperties: Record<string, any>;
  
  /** Template compatibility information */
  compatibility: TemplateCompatibility;
}

/**
 * Template compatibility information
 */
export interface TemplateCompatibility {
  /** Minimum system version required */
  minVersion: string;
  
  /** Maximum system version supported */
  maxVersion?: string;
  
  /** Required features */
  requiredFeatures: string[];
  
  /** Optional features that enhance functionality */
  optionalFeatures: string[];
}

/**
 * Data Transfer Object for creating transaction routes
 */
export interface CreateTransactionRouteInput {
  /** Short, descriptive title for the transaction route */
  title: string;
  
  /** Detailed description of the route's purpose and usage */
  description: string;
  
  /** Category classification for organization */
  category?: TransactionRouteCategory;
  
  /** Processing priority level */
  priority?: TransactionRoutePriority;
  
  /** Business logic and validation rules */
  rules?: Partial<TransactionRouteRules>;
  
  /** Template information if creating from template */
  template?: Partial<TransactionRouteTemplate>;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Data Transfer Object for updating transaction routes
 */
export interface UpdateTransactionRouteInput {
  /** Short, descriptive title for the transaction route */
  title?: string;
  
  /** Detailed description of the route's purpose and usage */
  description?: string;
  
  /** Category classification for organization */
  category?: TransactionRouteCategory;
  
  /** Current status of the transaction route */
  status?: TransactionRouteStatus;
  
  /** Processing priority level */
  priority?: TransactionRoutePriority;
  
  /** Business logic and validation rules */
  rules?: Partial<TransactionRouteRules>;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Validation result for transaction route operations
 */
export interface TransactionRouteValidationResult {
  /** Whether the validation passed */
  isValid: boolean;
  
  /** List of validation errors */
  errors: ValidationError[];
  
  /** List of validation warnings */
  warnings: ValidationWarning[];
  
  /** Compliance validation results */
  compliance: ComplianceValidation;
  
  /** Validation context information */
  context: ValidationContext;
}

/**
 * Transaction route analytics data
 */
export interface TransactionRouteAnalytics {
  /** Overview statistics */
  overview: TransactionRouteOverview;
  
  /** Usage analytics */
  usage: TransactionRouteUsageAnalytics;
  
  /** Performance metrics */
  performance: TransactionRoutePerformanceAnalytics;
  
  /** Compliance analytics */
  compliance: TransactionRouteComplianceAnalytics;
  
  /** Trend analysis */
  trends: TransactionRouteTrends;
}

/**
 * Overview statistics for transaction routes
 */
export interface TransactionRouteOverview {
  /** Total number of transaction routes */
  total: number;
  
  /** Number of active routes */
  active: number;
  
  /** Number of inactive routes */
  inactive: number;
  
  /** Number of draft routes */
  draft: number;
  
  /** Number of invalid routes */
  invalid: number;
  
  /** Breakdown by category */
  byCategory: Record<TransactionRouteCategory, number>;
  
  /** Breakdown by priority */
  byPriority: Record<TransactionRoutePriority, number>;
}

/**
 * Usage analytics for transaction routes
 */
export interface TransactionRouteUsageAnalytics {
  /** Total usage across all routes */
  totalUsage: number;
  
  /** Most used routes */
  mostUsed: TransactionRouteUsageSummary[];
  
  /** Least used routes */
  leastUsed: TransactionRouteUsageSummary[];
  
  /** Usage distribution by category */
  usageByCategory: Record<TransactionRouteCategory, number>;
  
  /** Daily usage trends */
  dailyTrends: DailyUsageData[];
}

/**
 * Usage summary for individual transaction routes
 */
export interface TransactionRouteUsageSummary {
  /** Route identifier */
  id: string;
  
  /** Route title */
  title: string;
  
  /** Route category */
  category: TransactionRouteCategory;
  
  /** Usage count */
  usageCount: number;
  
  /** Usage percentage */
  percentage: number;
  
  /** Success rate */
  successRate: number;
}

/**
 * Performance analytics for transaction routes
 */
export interface TransactionRoutePerformanceAnalytics {
  /** Overall performance score */
  overallScore: number;
  
  /** Performance by route */
  byRoute: TransactionRoutePerformanceSummary[];
  
  /** Performance trends over time */
  trends: PerformanceTrend[];
  
  /** Benchmark comparisons */
  benchmarks: PerformanceBenchmark[];
}

/**
 * Performance summary for individual transaction routes
 */
export interface TransactionRoutePerformanceSummary {
  /** Route identifier */
  id: string;
  
  /** Route title */
  title: string;
  
  /** Route category */
  category: TransactionRouteCategory;
  
  /** Performance metrics */
  performance: TransactionRoutePerformance;
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
  
  /** Average processing time */
  avgProcessingTime: number;
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
 * Compliance analytics for transaction routes
 */
export interface TransactionRouteComplianceAnalytics {
  /** Overall compliance score */
  overallScore: number;
  
  /** Compliance by route */
  byRoute: TransactionRouteComplianceSummary[];
  
  /** Compliance trends over time */
  trends: ComplianceTrend[];
  
  /** Violation summary */
  violations: ComplianceViolationSummary;
}

/**
 * Compliance summary for individual transaction routes
 */
export interface TransactionRouteComplianceSummary {
  /** Route identifier */
  id: string;
  
  /** Route title */
  title: string;
  
  /** Route category */
  category: TransactionRouteCategory;
  
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
 * Trend analysis for transaction routes
 */
export interface TransactionRouteTrends {
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
  
  /** Number of routes */
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
