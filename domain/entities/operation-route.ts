/**
 * Operation Route Domain Entity
 * 
 * This file defines the complete OperationRoute entity structure for the Accounting plugin,
 * including account mappings, operation types, validation, and all related interfaces
 * for operation-level transaction routing and validation.
 * 
 * @module OperationRoute
 * @version 1.0.0
 */

import { ValidationError, ValidationWarning } from './account-type';

/**
 * Operation route types that define the role in a transaction
 */
export enum OperationRouteType {
  /** Source account (debit side of transaction) */
  SOURCE = 'source',
  /** Destination account (credit side of transaction) */
  DESTINATION = 'destination',
}

/**
 * Operation route status for lifecycle management
 */
export enum OperationRouteStatus {
  /** Operation route is active and ready for use */
  ACTIVE = 'active',
  /** Operation route is temporarily disabled */
  INACTIVE = 'inactive',
  /** Operation route is in draft state during creation */
  DRAFT = 'draft',
  /** Operation route has validation errors */
  INVALID = 'invalid',
}

/**
 * Account selection modes for operation routes
 */
export enum AccountSelectionMode {
  /** Specific account selected by ID */
  SPECIFIC = 'specific',
  /** Account selected by alias pattern */
  ALIAS_PATTERN = 'alias_pattern',
  /** Account selected by type matching */
  TYPE_MATCHING = 'type_matching',
  /** Dynamic account selection at runtime */
  DYNAMIC = 'dynamic',
}

/**
 * Core Operation Route entity representing individual operation mappings
 * 
 * Operation routes define how accounts are mapped within transaction routes,
 * including source/destination roles, account selection criteria, and
 * validation rules for individual operations.
 */
export interface OperationRoute {
  /** Unique identifier for the operation route */
  readonly id: string;
  
  /** Reference to the parent transaction route */
  transactionId: string;
  
  /** Short, descriptive title for the operation */
  title: string;
  
  /** Type of operation (source or destination) */
  type: OperationRouteType;
  
  /** Current status of the operation route */
  status: OperationRouteStatus;
  
  /** Account mapping configuration */
  account: AccountMapping;
  
  /** Operation-specific rules and constraints */
  rules: OperationRouteRules;
  
  /** Validation results and status */
  validation: OperationRouteValidation;
  
  /** Usage tracking and analytics */
  usage: OperationRouteUsage;
  
  /** Audit and timestamp information */
  audit: OperationRouteAudit;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Account mapping configuration for operation routes
 */
export interface AccountMapping {
  /** Account selection mode */
  selectionMode: AccountSelectionMode;
  
  /** Specific account ID (if using SPECIFIC mode) */
  accountId?: string;
  
  /** Account alias or alias pattern */
  alias?: string;
  
  /** Account type constraints */
  type: string[];
  
  /** Account selection criteria */
  criteria: AccountSelectionCriteria;
  
  /** Runtime account resolution */
  resolution: AccountResolution;
}

/**
 * Account selection criteria for flexible account matching
 */
export interface AccountSelectionCriteria {
  /** Required account properties */
  required: AccountProperty[];
  
  /** Optional account properties that are preferred */
  preferred: AccountProperty[];
  
  /** Account properties to exclude */
  excluded: AccountProperty[];
  
  /** Custom selection logic */
  customLogic?: string;
}

/**
 * Account property for selection criteria
 */
export interface AccountProperty {
  /** Property name */
  name: string;
  
  /** Property value or pattern */
  value: any;
  
  /** Comparison operator */
  operator: 'equals' | 'contains' | 'starts_with' | 'ends_with' | 'regex' | 'in' | 'not_in';
  
  /** Whether this property is mandatory */
  mandatory: boolean;
}

/**
 * Account resolution results and status
 */
export interface AccountResolution {
  /** Whether account resolution was successful */
  resolved: boolean;
  
  /** Resolved account ID */
  resolvedAccountId?: string;
  
  /** Resolved account alias */
  resolvedAlias?: string;
  
  /** Resolution timestamp */
  resolvedAt?: Date;
  
  /** Resolution errors if any */
  errors: ResolutionError[];
  
  /** Resolution warnings */
  warnings: ResolutionWarning[];
  
  /** Fallback account options */
  fallbackOptions: FallbackAccount[];
}

/**
 * Account resolution error details
 */
export interface ResolutionError {
  /** Error code */
  code: string;
  
  /** Error message */
  message: string;
  
  /** Severity level */
  severity: 'critical' | 'high' | 'medium' | 'low';
  
  /** Suggested resolution */
  suggestion?: string;
}

/**
 * Account resolution warning details
 */
export interface ResolutionWarning {
  /** Warning code */
  code: string;
  
  /** Warning message */
  message: string;
  
  /** Recommended action */
  recommendation?: string;
}

/**
 * Fallback account option for resolution failures
 */
export interface FallbackAccount {
  /** Fallback account ID */
  accountId: string;
  
  /** Fallback account alias */
  alias: string;
  
  /** Account type */
  type: string[];
  
  /** Reason for fallback */
  reason: string;
  
  /** Confidence score (0-1) */
  confidence: number;
}

/**
 * Operation-specific rules and constraints
 */
export interface OperationRouteRules {
  /** Whether this operation is mandatory for the transaction */
  mandatory: boolean;
  
  /** Amount constraints for this operation */
  amountConstraints: AmountConstraints;
  
  /** Currency constraints */
  currencyConstraints: CurrencyConstraints;
  
  /** Balance requirements */
  balanceRequirements: BalanceRequirements;
  
  /** Time-based constraints */
  timeConstraints: TimeConstraints;
  
  /** Conditional logic for operation execution */
  conditionalLogic: ConditionalLogic[];
  
  /** Custom validation rules */
  customValidation: CustomOperationRule[];
}

/**
 * Amount constraints for operations
 */
export interface AmountConstraints {
  /** Minimum amount allowed */
  minimum?: number;
  
  /** Maximum amount allowed */
  maximum?: number;
  
  /** Fixed amount (if operation requires specific amount) */
  fixed?: number;
  
  /** Amount calculation formula */
  formula?: string;
  
  /** Percentage of transaction amount */
  percentage?: number;
  
  /** Rounding rules */
  rounding: RoundingRules;
}

/**
 * Rounding rules for amount calculations
 */
export interface RoundingRules {
  /** Rounding mode */
  mode: 'up' | 'down' | 'nearest' | 'none';
  
  /** Decimal places for rounding */
  decimals: number;
  
  /** Minimum unit for rounding */
  minimumUnit?: number;
}

/**
 * Currency constraints for operations
 */
export interface CurrencyConstraints {
  /** Allowed currencies for this operation */
  allowed: string[];
  
  /** Preferred currency */
  preferred?: string;
  
  /** Whether currency conversion is allowed */
  allowConversion: boolean;
  
  /** Exchange rate requirements */
  exchangeRateRequirements?: ExchangeRateRequirements;
}

/**
 * Exchange rate requirements for currency conversion
 */
export interface ExchangeRateRequirements {
  /** Maximum age of exchange rate in minutes */
  maxAge: number;
  
  /** Required rate source */
  source?: string;
  
  /** Maximum spread allowed */
  maxSpread?: number;
}

/**
 * Balance requirements for account operations
 */
export interface BalanceRequirements {
  /** Minimum balance required before operation */
  minimumBalance?: number;
  
  /** Minimum balance required after operation */
  minimumBalanceAfter?: number;
  
  /** Maximum balance allowed after operation */
  maximumBalanceAfter?: number;
  
  /** Whether overdraft is allowed */
  allowOverdraft: boolean;
  
  /** Overdraft limit if allowed */
  overdraftLimit?: number;
}

/**
 * Time-based constraints for operations
 */
export interface TimeConstraints {
  /** Earliest time operation can be executed */
  earliestTime?: Date;
  
  /** Latest time operation can be executed */
  latestTime?: Date;
  
  /** Business hours constraints */
  businessHours?: BusinessHoursConstraint;
  
  /** Settlement time requirements */
  settlementTime?: SettlementTimeRequirement;
}

/**
 * Business hours constraint
 */
export interface BusinessHoursConstraint {
  /** Whether operation is restricted to business hours */
  restrictToBusinessHours: boolean;
  
  /** Business hours definition */
  businessHours?: BusinessHours;
  
  /** Timezone for business hours */
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
  
  /** Days of week (0-6, Sunday-Saturday) */
  daysOfWeek: number[];
}

/**
 * Settlement time requirement
 */
export interface SettlementTimeRequirement {
  /** Same day settlement required */
  sameDaySettlement: boolean;
  
  /** Maximum settlement time in hours */
  maxSettlementHours?: number;
  
  /** Settlement cutoff time */
  cutoffTime?: string;
}

/**
 * Conditional logic for operation execution
 */
export interface ConditionalLogic {
  /** Unique identifier for the condition */
  id: string;
  
  /** Condition name */
  name: string;
  
  /** Condition expression */
  expression: string;
  
  /** Action to take if condition is true */
  trueAction: 'execute' | 'skip' | 'modify' | 'error';
  
  /** Action to take if condition is false */
  falseAction: 'execute' | 'skip' | 'modify' | 'error';
  
  /** Parameters for actions */
  actionParameters: Record<string, any>;
}

/**
 * Custom operation validation rule
 */
export interface CustomOperationRule {
  /** Unique identifier for the rule */
  id: string;
  
  /** Rule name */
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
  
  /** Rule execution order */
  order: number;
}

/**
 * Validation results and status for operation routes
 */
export interface OperationRouteValidation {
  /** Whether the operation route is currently valid */
  isValid: boolean;
  
  /** List of validation errors */
  errors: ValidationError[];
  
  /** List of validation warnings */
  warnings: ValidationWarning[];
  
  /** Account compatibility validation */
  accountCompatibility: AccountCompatibilityValidation;
  
  /** Rule validation results */
  ruleValidation: RuleValidationResult[];
  
  /** Last validation timestamp */
  lastValidated: Date;
}

/**
 * Account compatibility validation results
 */
export interface AccountCompatibilityValidation {
  /** Whether account is compatible */
  isCompatible: boolean;
  
  /** Compatibility score (0-100) */
  score: number;
  
  /** Compatibility issues */
  issues: CompatibilityIssue[];
  
  /** Suggested improvements */
  suggestions: CompatibilitySuggestion[];
}

/**
 * Compatibility issue details
 */
export interface CompatibilityIssue {
  /** Issue type */
  type: 'account_type' | 'balance' | 'currency' | 'status' | 'permissions';
  
  /** Issue description */
  description: string;
  
  /** Severity level */
  severity: 'critical' | 'high' | 'medium' | 'low';
  
  /** Possible resolution */
  resolution?: string;
}

/**
 * Compatibility improvement suggestion
 */
export interface CompatibilitySuggestion {
  /** Suggestion type */
  type: 'account_selection' | 'rule_modification' | 'constraint_adjustment';
  
  /** Suggestion description */
  description: string;
  
  /** Expected improvement */
  expectedImprovement: string;
  
  /** Implementation difficulty */
  difficulty: 'easy' | 'medium' | 'hard';
}

/**
 * Rule validation result
 */
export interface RuleValidationResult {
  /** Rule identifier */
  ruleId: string;
  
  /** Rule name */
  ruleName: string;
  
  /** Whether rule passed validation */
  passed: boolean;
  
  /** Error message if rule failed */
  errorMessage?: string;
  
  /** Warning message if applicable */
  warningMessage?: string;
  
  /** Execution time in milliseconds */
  executionTime: number;
}

/**
 * Usage tracking and analytics for operation routes
 */
export interface OperationRouteUsage {
  /** Total number of times this operation has been executed */
  executionCount: number;
  
  /** Number of successful executions */
  successCount: number;
  
  /** Number of failed executions */
  failureCount: number;
  
  /** Timestamp of last execution */
  lastExecuted?: Date;
  
  /** Daily execution statistics */
  dailyStats: DailyExecutionStats[];
  
  /** Performance metrics */
  performance: OperationRoutePerformance;
}

/**
 * Daily execution statistics
 */
export interface DailyExecutionStats {
  /** Date in YYYY-MM-DD format */
  date: string;
  
  /** Total executions */
  executions: number;
  
  /** Successful executions */
  successes: number;
  
  /** Failed executions */
  failures: number;
  
  /** Average execution time */
  avgExecutionTime: number;
}

/**
 * Performance metrics for operation routes
 */
export interface OperationRoutePerformance {
  /** Average execution time in milliseconds */
  avgExecutionTime: number;
  
  /** 95th percentile execution time */
  p95ExecutionTime: number;
  
  /** Success rate percentage */
  successRate: number;
  
  /** Error rate percentage */
  errorRate: number;
  
  /** Account resolution success rate */
  resolutionSuccessRate: number;
}

/**
 * Audit trail and timestamp information
 */
export interface OperationRouteAudit {
  /** Creation timestamp */
  createdAt: Date;
  
  /** Last update timestamp */
  updatedAt: Date;
  
  /** Soft deletion timestamp */
  deletedAt?: Date;
  
  /** User who created the operation route */
  createdBy: string;
  
  /** User who last updated the operation route */
  updatedBy: string;
  
  /** User who deleted the operation route */
  deletedBy?: string;
  
  /** Version number for optimistic locking */
  version: number;
  
  /** Change history for audit trail */
  changeHistory: OperationRouteChange[];
}

/**
 * Change history entry for audit trail
 */
export interface OperationRouteChange {
  /** Unique identifier for the change */
  id: string;
  
  /** Type of change made */
  changeType: 'created' | 'updated' | 'deleted' | 'status_changed' | 'account_changed' | 'rules_updated';
  
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
 * Data Transfer Object for creating operation routes
 */
export interface CreateOperationRouteInput {
  /** Reference to the parent transaction route */
  transactionId: string;
  
  /** Short, descriptive title for the operation */
  title: string;
  
  /** Type of operation (source or destination) */
  type: OperationRouteType;
  
  /** Account mapping configuration */
  account: Partial<AccountMapping>;
  
  /** Operation-specific rules and constraints */
  rules?: Partial<OperationRouteRules>;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Data Transfer Object for updating operation routes
 */
export interface UpdateOperationRouteInput {
  /** Short, descriptive title for the operation */
  title?: string;
  
  /** Current status of the operation route */
  status?: OperationRouteStatus;
  
  /** Account mapping configuration */
  account?: Partial<AccountMapping>;
  
  /** Operation-specific rules and constraints */
  rules?: Partial<OperationRouteRules>;
  
  /** Additional metadata for extensibility */
  metadata?: Record<string, any>;
}

/**
 * Validation result for operation route operations
 */
export interface OperationRouteValidationResult {
  /** Whether the validation passed */
  isValid: boolean;
  
  /** List of validation errors */
  errors: ValidationError[];
  
  /** List of validation warnings */
  warnings: ValidationWarning[];
  
  /** Account compatibility validation */
  accountCompatibility: AccountCompatibilityValidation;
  
  /** Rule validation results */
  ruleValidation: RuleValidationResult[];
  
  /** Validation context information */
  context: ValidationContext;
}

/**
 * Validation context for detailed reporting
 */
export interface ValidationContext {
  /** Operation being validated */
  operation: 'create' | 'update' | 'delete' | 'execute';
  
  /** User or system performing validation */
  validator: string;
  
  /** Transaction context if applicable */
  transactionContext?: Record<string, any>;
  
  /** Additional context parameters */
  parameters: Record<string, any>;
}

/**
 * Operation route analytics data
 */
export interface OperationRouteAnalytics {
  /** Overview statistics */
  overview: OperationRouteOverview;
  
  /** Usage analytics */
  usage: OperationRouteUsageAnalytics;
  
  /** Performance metrics */
  performance: OperationRoutePerformanceAnalytics;
  
  /** Account compatibility analytics */
  compatibility: OperationRouteCompatibilityAnalytics;
}

/**
 * Overview statistics for operation routes
 */
export interface OperationRouteOverview {
  /** Total number of operation routes */
  total: number;
  
  /** Number of active operation routes */
  active: number;
  
  /** Number of inactive operation routes */
  inactive: number;
  
  /** Number of draft operation routes */
  draft: number;
  
  /** Number of invalid operation routes */
  invalid: number;
  
  /** Breakdown by type */
  byType: Record<OperationRouteType, number>;
  
  /** Breakdown by selection mode */
  bySelectionMode: Record<AccountSelectionMode, number>;
}

/**
 * Usage analytics for operation routes
 */
export interface OperationRouteUsageAnalytics {
  /** Total executions across all operation routes */
  totalExecutions: number;
  
  /** Most executed operation routes */
  mostExecuted: OperationRouteUsageSummary[];
  
  /** Least executed operation routes */
  leastExecuted: OperationRouteUsageSummary[];
  
  /** Execution distribution by type */
  executionsByType: Record<OperationRouteType, number>;
  
  /** Daily execution trends */
  dailyTrends: DailyExecutionStats[];
}

/**
 * Usage summary for individual operation routes
 */
export interface OperationRouteUsageSummary {
  /** Operation route identifier */
  id: string;
  
  /** Operation route title */
  title: string;
  
  /** Operation route type */
  type: OperationRouteType;
  
  /** Execution count */
  executionCount: number;
  
  /** Success rate */
  successRate: number;
  
  /** Usage percentage */
  percentage: number;
}

/**
 * Performance analytics for operation routes
 */
export interface OperationRoutePerformanceAnalytics {
  /** Overall performance score */
  overallScore: number;
  
  /** Performance by operation route */
  byOperationRoute: OperationRoutePerformanceSummary[];
  
  /** Performance trends over time */
  trends: PerformanceTrend[];
  
  /** Benchmark comparisons */
  benchmarks: PerformanceBenchmark[];
}

/**
 * Performance summary for individual operation routes
 */
export interface OperationRoutePerformanceSummary {
  /** Operation route identifier */
  id: string;
  
  /** Operation route title */
  title: string;
  
  /** Operation route type */
  type: OperationRouteType;
  
  /** Performance metrics */
  performance: OperationRoutePerformance;
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
  
  /** Average execution time */
  avgExecutionTime: number;
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
 * Compatibility analytics for operation routes
 */
export interface OperationRouteCompatibilityAnalytics {
  /** Overall compatibility score */
  overallScore: number;
  
  /** Compatibility by operation route */
  byOperationRoute: OperationRouteCompatibilitySummary[];
  
  /** Common compatibility issues */
  commonIssues: CompatibilityIssueStats[];
  
  /** Resolution success rates */
  resolutionRates: ResolutionRateStats;
}

/**
 * Compatibility summary for individual operation routes
 */
export interface OperationRouteCompatibilitySummary {
  /** Operation route identifier */
  id: string;
  
  /** Operation route title */
  title: string;
  
  /** Operation route type */
  type: OperationRouteType;
  
  /** Compatibility score */
  score: number;
  
  /** Number of issues */
  issues: number;
}

/**
 * Compatibility issue statistics
 */
export interface CompatibilityIssueStats {
  /** Issue type */
  type: string;
  
  /** Number of occurrences */
  count: number;
  
  /** Percentage of total issues */
  percentage: number;
  
  /** Average resolution time */
  avgResolutionTime: number;
}

/**
 * Resolution rate statistics
 */
export interface ResolutionRateStats {
  /** Overall account resolution success rate */
  overall: number;
  
  /** Resolution rate by selection mode */
  bySelectionMode: Record<AccountSelectionMode, number>;
  
  /** Resolution rate by account type */
  byAccountType: Record<string, number>;
  
  /** Average resolution time */
  avgResolutionTime: number;
}
