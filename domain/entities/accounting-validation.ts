/**
 * Accounting Validation Domain Entity
 * 
 * This file defines comprehensive validation rules, compliance entities, and
 * business logic validation for the Accounting plugin. It includes validation
 * engines, rule definitions, compliance frameworks, and audit capabilities.
 * 
 * @module AccountingValidation
 * @version 1.0.0
 */

import { AccountType, AccountDomain } from './account-type';
import { TransactionRoute, TransactionRouteCategory } from './transaction-route';
import { OperationRoute, OperationRouteType } from './operation-route';

/**
 * Validation rule types for different accounting scenarios
 */
export enum ValidationRuleType {
  /** Account type validation rules */
  ACCOUNT_TYPE = 'account_type',
  /** Transaction route validation rules */
  TRANSACTION_ROUTE = 'transaction_route',
  /** Operation route validation rules */
  OPERATION_ROUTE = 'operation_route',
  /** Cross-entity relationship validation */
  RELATIONSHIP = 'relationship',
  /** Business logic validation */
  BUSINESS_LOGIC = 'business_logic',
  /** Compliance and regulatory validation */
  COMPLIANCE = 'compliance',
  /** Data integrity validation */
  DATA_INTEGRITY = 'data_integrity',
  /** Performance validation */
  PERFORMANCE = 'performance',
}

/**
 * Validation severity levels
 */
export enum ValidationSeverity {
  /** Information only, no action required */
  INFO = 'info',
  /** Warning that should be addressed */
  WARNING = 'warning',
  /** Error that blocks operation */
  ERROR = 'error',
  /** Critical error that requires immediate attention */
  CRITICAL = 'critical',
}

/**
 * Validation rule status
 */
export enum ValidationRuleStatus {
  /** Rule is active and being enforced */
  ACTIVE = 'active',
  /** Rule is temporarily disabled */
  DISABLED = 'disabled',
  /** Rule is in draft state */
  DRAFT = 'draft',
  /** Rule is deprecated but still active */
  DEPRECATED = 'deprecated',
  /** Rule is archived and no longer enforced */
  ARCHIVED = 'archived',
}

/**
 * Compliance framework types
 */
export enum ComplianceFramework {
  /** Generally Accepted Accounting Principles */
  GAAP = 'gaap',
  /** International Financial Reporting Standards */
  IFRS = 'ifrs',
  /** Sarbanes-Oxley Act compliance */
  SOX = 'sox',
  /** Payment Card Industry Data Security Standard */
  PCI_DSS = 'pci_dss',
  /** Know Your Customer regulations */
  KYC = 'kyc',
  /** Anti-Money Laundering regulations */
  AML = 'aml',
  /** Basel III banking regulations */
  BASEL_III = 'basel_iii',
  /** Custom organizational compliance */
  CUSTOM = 'custom',
}

/**
 * Core validation rule entity
 */
export interface ValidationRule {
  /** Unique identifier for the validation rule */
  readonly id: string;
  
  /** Human-readable name of the rule */
  name: string;
  
  /** Detailed description of the rule's purpose */
  description: string;
  
  /** Type of validation rule */
  type: ValidationRuleType;
  
  /** Current status of the rule */
  status: ValidationRuleStatus;
  
  /** Severity level of violations */
  severity: ValidationSeverity;
  
  /** Rule definition and logic */
  definition: ValidationRuleDefinition;
  
  /** Compliance frameworks this rule supports */
  frameworks: ComplianceFramework[];
  
  /** Rule execution context */
  context: ValidationRuleContext;
  
  /** Rule performance metrics */
  performance: ValidationRulePerformance;
  
  /** Audit and timestamp information */
  audit: ValidationRuleAudit;
  
  /** Additional metadata */
  metadata?: Record<string, any>;
}

/**
 * Validation rule definition and logic
 */
export interface ValidationRuleDefinition {
  /** Rule expression or script */
  expression: string;
  
  /** Expression language or format */
  language: 'javascript' | 'json_logic' | 'regex' | 'sql' | 'custom';
  
  /** Input parameters for the rule */
  parameters: ValidationParameter[];
  
  /** Expected output format */
  output: ValidationOutput;
  
  /** Rule dependencies */
  dependencies: string[];
  
  /** Custom functions used by the rule */
  customFunctions: CustomFunction[];
}

/**
 * Validation parameter definition
 */
export interface ValidationParameter {
  /** Parameter name */
  name: string;
  
  /** Parameter type */
  type: 'string' | 'number' | 'boolean' | 'object' | 'array' | 'date';
  
  /** Whether parameter is required */
  required: boolean;
  
  /** Default value if not provided */
  defaultValue?: any;
  
  /** Parameter description */
  description: string;
  
  /** Validation constraints for the parameter */
  constraints: ParameterConstraints;
}

/**
 * Parameter validation constraints
 */
export interface ParameterConstraints {
  /** Minimum value (for numbers) */
  min?: number;
  
  /** Maximum value (for numbers) */
  max?: number;
  
  /** Minimum length (for strings/arrays) */
  minLength?: number;
  
  /** Maximum length (for strings/arrays) */
  maxLength?: number;
  
  /** Regular expression pattern (for strings) */
  pattern?: string;
  
  /** Enumerated values */
  enum?: any[];
  
  /** Custom validation function */
  customValidator?: string;
}

/**
 * Validation output definition
 */
export interface ValidationOutput {
  /** Output type */
  type: 'boolean' | 'object' | 'array';
  
  /** Output schema for structured results */
  schema?: ValidationOutputSchema;
  
  /** Error message template */
  errorTemplate: string;
  
  /** Warning message template */
  warningTemplate?: string;
  
  /** Success message template */
  successTemplate?: string;
}

/**
 * Validation output schema
 */
export interface ValidationOutputSchema {
  /** Required fields in output */
  required: string[];
  
  /** Field definitions */
  fields: Record<string, ValidationFieldDefinition>;
}

/**
 * Validation field definition
 */
export interface ValidationFieldDefinition {
  /** Field type */
  type: string;
  
  /** Field description */
  description: string;
  
  /** Whether field is required */
  required: boolean;
}

/**
 * Custom function definition for validation rules
 */
export interface CustomFunction {
  /** Function name */
  name: string;
  
  /** Function implementation */
  implementation: string;
  
  /** Function description */
  description: string;
  
  /** Function parameters */
  parameters: string[];
  
  /** Return type */
  returnType: string;
}

/**
 * Validation rule execution context
 */
export interface ValidationRuleContext {
  /** When the rule should be executed */
  triggers: ValidationTrigger[];
  
  /** Execution order priority */
  priority: number;
  
  /** Whether rule can be executed in parallel */
  parallel: boolean;
  
  /** Timeout for rule execution in milliseconds */
  timeout: number;
  
  /** Retry policy for failed executions */
  retryPolicy: RetryPolicy;
  
  /** Caching configuration */
  caching: CachingConfig;
}

/**
 * Validation trigger definition
 */
export interface ValidationTrigger {
  /** Event that triggers validation */
  event: 'create' | 'update' | 'delete' | 'status_change' | 'scheduled' | 'manual';
  
  /** Conditions for trigger activation */
  conditions: TriggerCondition[];
  
  /** Whether trigger is enabled */
  enabled: boolean;
}

/**
 * Trigger condition definition
 */
export interface TriggerCondition {
  /** Field to check */
  field: string;
  
  /** Comparison operator */
  operator: 'equals' | 'not_equals' | 'contains' | 'changed' | 'exists';
  
  /** Value to compare against */
  value: any;
}

/**
 * Retry policy for failed validations
 */
export interface RetryPolicy {
  /** Maximum number of retries */
  maxRetries: number;
  
  /** Delay between retries in milliseconds */
  delay: number;
  
  /** Backoff strategy */
  backoffStrategy: 'linear' | 'exponential' | 'fixed';
  
  /** Maximum delay between retries */
  maxDelay: number;
}

/**
 * Caching configuration for validation results
 */
export interface CachingConfig {
  /** Whether results should be cached */
  enabled: boolean;
  
  /** Cache TTL in seconds */
  ttl: number;
  
  /** Cache key strategy */
  keyStrategy: 'simple' | 'hash' | 'custom';
  
  /** Custom cache key function */
  customKeyFunction?: string;
}

/**
 * Validation rule performance metrics
 */
export interface ValidationRulePerformance {
  /** Total number of executions */
  totalExecutions: number;
  
  /** Number of successful executions */
  successfulExecutions: number;
  
  /** Number of failed executions */
  failedExecutions: number;
  
  /** Average execution time in milliseconds */
  avgExecutionTime: number;
  
  /** 95th percentile execution time */
  p95ExecutionTime: number;
  
  /** Cache hit rate percentage */
  cacheHitRate: number;
  
  /** Last execution timestamp */
  lastExecuted?: Date;
}

/**
 * Validation rule audit information
 */
export interface ValidationRuleAudit {
  /** Creation timestamp */
  createdAt: Date;
  
  /** Last update timestamp */
  updatedAt: Date;
  
  /** User who created the rule */
  createdBy: string;
  
  /** User who last updated the rule */
  updatedBy: string;
  
  /** Version number */
  version: number;
  
  /** Change history */
  changeHistory: ValidationRuleChange[];
}

/**
 * Validation rule change history
 */
export interface ValidationRuleChange {
  /** Change identifier */
  id: string;
  
  /** Type of change */
  changeType: 'created' | 'updated' | 'status_changed' | 'expression_updated';
  
  /** Fields that changed */
  changedFields: string[];
  
  /** Previous values */
  previousValues: Record<string, any>;
  
  /** New values */
  newValues: Record<string, any>;
  
  /** User who made the change */
  changedBy: string;
  
  /** Change timestamp */
  changedAt: Date;
  
  /** Reason for change */
  reason?: string;
}

/**
 * Validation execution result
 */
export interface ValidationResult {
  /** Validation rule identifier */
  ruleId: string;
  
  /** Rule name for reference */
  ruleName: string;
  
  /** Whether validation passed */
  passed: boolean;
  
  /** Validation severity */
  severity: ValidationSeverity;
  
  /** Result message */
  message: string;
  
  /** Detailed result data */
  details: ValidationResultDetails;
  
  /** Execution metadata */
  execution: ValidationExecution;
}

/**
 * Detailed validation result information
 */
export interface ValidationResultDetails {
  /** Input values used for validation */
  inputs: Record<string, any>;
  
  /** Output values from validation */
  outputs: Record<string, any>;
  
  /** Validation context */
  context: Record<string, any>;
  
  /** Affected entities */
  affectedEntities: AffectedEntity[];
  
  /** Recommendations for fixing issues */
  recommendations: ValidationRecommendation[];
}

/**
 * Entity affected by validation
 */
export interface AffectedEntity {
  /** Entity type */
  type: 'account_type' | 'transaction_route' | 'operation_route';
  
  /** Entity identifier */
  id: string;
  
  /** Entity name or description */
  name: string;
  
  /** Fields affected */
  affectedFields: string[];
}

/**
 * Validation recommendation
 */
export interface ValidationRecommendation {
  /** Recommendation type */
  type: 'fix' | 'improvement' | 'optimization';
  
  /** Recommendation description */
  description: string;
  
  /** Priority level */
  priority: 'low' | 'medium' | 'high' | 'critical';
  
  /** Estimated effort to implement */
  effort: 'low' | 'medium' | 'high';
  
  /** Step-by-step instructions */
  instructions: string[];
}

/**
 * Validation execution metadata
 */
export interface ValidationExecution {
  /** Execution timestamp */
  executedAt: Date;
  
  /** Execution duration in milliseconds */
  duration: number;
  
  /** Executor (user or system) */
  executor: string;
  
  /** Execution trigger */
  trigger: string;
  
  /** Whether result was cached */
  cached: boolean;
  
  /** Error information if execution failed */
  error?: ExecutionError;
}

/**
 * Execution error details
 */
export interface ExecutionError {
  /** Error code */
  code: string;
  
  /** Error message */
  message: string;
  
  /** Stack trace */
  stackTrace?: string;
  
  /** Error context */
  context: Record<string, any>;
}

/**
 * Compliance validation framework
 */
export interface ComplianceFrameworkDefinition {
  /** Framework identifier */
  id: string;
  
  /** Framework name */
  name: string;
  
  /** Framework description */
  description: string;
  
  /** Framework type */
  type: ComplianceFramework;
  
  /** Framework version */
  version: string;
  
  /** Compliance requirements */
  requirements: ComplianceRequirement[];
  
  /** Validation rules for this framework */
  validationRules: string[];
  
  /** Framework configuration */
  configuration: FrameworkConfiguration;
  
  /** Framework audit information */
  audit: ComplianceFrameworkAudit;
}

/**
 * Compliance requirement definition
 */
export interface ComplianceRequirement {
  /** Requirement identifier */
  id: string;
  
  /** Requirement name */
  name: string;
  
  /** Requirement description */
  description: string;
  
  /** Requirement category */
  category: string;
  
  /** Whether requirement is mandatory */
  mandatory: boolean;
  
  /** Validation criteria */
  criteria: RequirementCriteria[];
  
  /** Evidence requirements */
  evidence: EvidenceRequirement[];
}

/**
 * Requirement validation criteria
 */
export interface RequirementCriteria {
  /** Criteria identifier */
  id: string;
  
  /** Criteria description */
  description: string;
  
  /** Validation method */
  method: 'automated' | 'manual' | 'hybrid';
  
  /** Validation frequency */
  frequency: 'continuous' | 'daily' | 'weekly' | 'monthly' | 'quarterly' | 'annually';
  
  /** Acceptance threshold */
  threshold: number;
}

/**
 * Evidence requirement for compliance
 */
export interface EvidenceRequirement {
  /** Evidence type */
  type: 'document' | 'audit_log' | 'report' | 'certification' | 'screenshot';
  
  /** Evidence description */
  description: string;
  
  /** Whether evidence is required */
  required: boolean;
  
  /** Retention period in days */
  retentionDays: number;
  
  /** Access controls for evidence */
  accessControls: string[];
}

/**
 * Framework configuration
 */
export interface FrameworkConfiguration {
  /** Configuration parameters */
  parameters: Record<string, any>;
  
  /** Custom settings */
  customSettings: Record<string, any>;
  
  /** Integration settings */
  integrations: IntegrationSetting[];
  
  /** Notification settings */
  notifications: NotificationSetting[];
}

/**
 * Integration setting for external systems
 */
export interface IntegrationSetting {
  /** Integration name */
  name: string;
  
  /** Integration type */
  type: 'api' | 'webhook' | 'file' | 'database';
  
  /** Connection configuration */
  configuration: Record<string, any>;
  
  /** Whether integration is enabled */
  enabled: boolean;
}

/**
 * Notification setting for compliance alerts
 */
export interface NotificationSetting {
  /** Notification type */
  type: 'email' | 'sms' | 'webhook' | 'dashboard';
  
  /** Recipients */
  recipients: string[];
  
  /** Trigger conditions */
  triggers: NotificationTrigger[];
  
  /** Message template */
  template: string;
}

/**
 * Notification trigger condition
 */
export interface NotificationTrigger {
  /** Event that triggers notification */
  event: 'violation' | 'warning' | 'threshold_exceeded' | 'status_change';
  
  /** Severity levels that trigger notification */
  severityLevels: ValidationSeverity[];
  
  /** Additional conditions */
  conditions: Record<string, any>;
}

/**
 * Compliance framework audit information
 */
export interface ComplianceFrameworkAudit {
  /** Creation timestamp */
  createdAt: Date;
  
  /** Last update timestamp */
  updatedAt: Date;
  
  /** User who created the framework */
  createdBy: string;
  
  /** User who last updated the framework */
  updatedBy: string;
  
  /** Version number */
  version: number;
  
  /** Certification information */
  certifications: FrameworkCertification[];
}

/**
 * Framework certification information
 */
export interface FrameworkCertification {
  /** Certification body */
  certifyingBody: string;
  
  /** Certification date */
  certifiedAt: Date;
  
  /** Expiration date */
  expiresAt: Date;
  
  /** Certification status */
  status: 'valid' | 'expired' | 'suspended' | 'revoked';
  
  /** Certification identifier */
  certificationId: string;
}

/**
 * Validation engine configuration
 */
export interface ValidationEngine {
  /** Engine identifier */
  id: string;
  
  /** Engine name */
  name: string;
  
  /** Engine version */
  version: string;
  
  /** Engine configuration */
  configuration: ValidationEngineConfig;
  
  /** Supported rule types */
  supportedTypes: ValidationRuleType[];
  
  /** Engine performance settings */
  performance: EnginePerformanceConfig;
  
  /** Engine status */
  status: 'active' | 'inactive' | 'maintenance';
}

/**
 * Validation engine configuration
 */
export interface ValidationEngineConfig {
  /** Maximum concurrent validations */
  maxConcurrency: number;
  
  /** Default timeout in milliseconds */
  defaultTimeout: number;
  
  /** Memory limits */
  memoryLimits: MemoryLimits;
  
  /** Logging configuration */
  logging: LoggingConfig;
  
  /** Cache configuration */
  cache: EngineCacheConfig;
}

/**
 * Memory limits for validation engine
 */
export interface MemoryLimits {
  /** Maximum heap size in MB */
  maxHeapSize: number;
  
  /** Maximum cache size in MB */
  maxCacheSize: number;
  
  /** Garbage collection threshold */
  gcThreshold: number;
}

/**
 * Logging configuration for validation engine
 */
export interface LoggingConfig {
  /** Log level */
  level: 'debug' | 'info' | 'warn' | 'error';
  
  /** Whether to log rule executions */
  logExecutions: boolean;
  
  /** Whether to log performance metrics */
  logPerformance: boolean;
  
  /** Log retention period in days */
  retentionDays: number;
}

/**
 * Cache configuration for validation engine
 */
export interface EngineCacheConfig {
  /** Whether caching is enabled */
  enabled: boolean;
  
  /** Cache provider */
  provider: 'memory' | 'redis' | 'database';
  
  /** Cache configuration parameters */
  parameters: Record<string, any>;
}

/**
 * Engine performance configuration
 */
export interface EnginePerformanceConfig {
  /** Performance monitoring enabled */
  monitoringEnabled: boolean;
  
  /** Metrics collection interval in seconds */
  metricsInterval: number;
  
  /** Performance thresholds */
  thresholds: PerformanceThresholds;
  
  /** Circuit breaker configuration */
  circuitBreaker: CircuitBreakerConfig;
}

/**
 * Performance thresholds for monitoring
 */
export interface PerformanceThresholds {
  /** Maximum acceptable execution time in milliseconds */
  maxExecutionTime: number;
  
  /** Maximum acceptable error rate percentage */
  maxErrorRate: number;
  
  /** Minimum acceptable throughput per second */
  minThroughput: number;
}

/**
 * Circuit breaker configuration
 */
export interface CircuitBreakerConfig {
  /** Whether circuit breaker is enabled */
  enabled: boolean;
  
  /** Failure threshold to open circuit */
  failureThreshold: number;
  
  /** Success threshold to close circuit */
  successThreshold: number;
  
  /** Timeout before attempting to close circuit */
  timeout: number;
}

/**
 * Data Transfer Objects for validation operations
 */

/**
 * Input for creating validation rules
 */
export interface CreateValidationRuleInput {
  /** Rule name */
  name: string;
  
  /** Rule description */
  description: string;
  
  /** Rule type */
  type: ValidationRuleType;
  
  /** Rule severity */
  severity: ValidationSeverity;
  
  /** Rule definition */
  definition: Partial<ValidationRuleDefinition>;
  
  /** Compliance frameworks */
  frameworks?: ComplianceFramework[];
  
  /** Rule context */
  context?: Partial<ValidationRuleContext>;
  
  /** Additional metadata */
  metadata?: Record<string, any>;
}

/**
 * Input for updating validation rules
 */
export interface UpdateValidationRuleInput {
  /** Rule name */
  name?: string;
  
  /** Rule description */
  description?: string;
  
  /** Rule status */
  status?: ValidationRuleStatus;
  
  /** Rule severity */
  severity?: ValidationSeverity;
  
  /** Rule definition */
  definition?: Partial<ValidationRuleDefinition>;
  
  /** Rule context */
  context?: Partial<ValidationRuleContext>;
  
  /** Additional metadata */
  metadata?: Record<string, any>;
}

/**
 * Input for validation execution
 */
export interface ValidationExecutionInput {
  /** Rules to execute (empty means all applicable rules) */
  ruleIds?: string[];
  
  /** Entity to validate */
  entity: ValidationEntity;
  
  /** Validation context */
  context: Record<string, any>;
  
  /** Whether to use cached results */
  useCache?: boolean;
  
  /** Execution timeout override */
  timeout?: number;
}

/**
 * Entity being validated
 */
export interface ValidationEntity {
  /** Entity type */
  type: 'account_type' | 'transaction_route' | 'operation_route';
  
  /** Entity identifier */
  id: string;
  
  /** Entity data */
  data: AccountType | TransactionRoute | OperationRoute;
  
  /** Operation being performed */
  operation: 'create' | 'update' | 'delete' | 'status_change';
}

/**
 * Validation execution result
 */
export interface ValidationExecutionResult {
  /** Overall validation status */
  passed: boolean;
  
  /** Individual rule results */
  results: ValidationResult[];
  
  /** Summary statistics */
  summary: ValidationSummary;
  
  /** Execution metadata */
  execution: ValidationExecutionMetadata;
}

/**
 * Validation summary statistics
 */
export interface ValidationSummary {
  /** Total rules executed */
  totalRules: number;
  
  /** Number of rules that passed */
  passedRules: number;
  
  /** Number of rules that failed */
  failedRules: number;
  
  /** Number of warnings */
  warnings: number;
  
  /** Number of errors */
  errors: number;
  
  /** Number of critical issues */
  critical: number;
  
  /** Overall compliance score */
  complianceScore: number;
}

/**
 * Validation execution metadata
 */
export interface ValidationExecutionMetadata {
  /** Execution identifier */
  executionId: string;
  
  /** Start timestamp */
  startedAt: Date;
  
  /** End timestamp */
  completedAt: Date;
  
  /** Total execution time */
  totalDuration: number;
  
  /** Executor information */
  executor: string;
  
  /** Engine used for validation */
  engine: string;
  
  /** Cache statistics */
  cacheStats: CacheStatistics;
}

/**
 * Cache statistics for validation execution
 */
export interface CacheStatistics {
  /** Number of cache hits */
  hits: number;
  
  /** Number of cache misses */
  misses: number;
  
  /** Cache hit rate percentage */
  hitRate: number;
  
  /** Time saved by caching in milliseconds */
  timeSaved: number;
}
