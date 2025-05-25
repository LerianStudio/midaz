/**
 * Validation Plugin
 * Provides additional validation and data integrity checks
 */

import { BasePlugin, PluginContext, EntityContext } from '../internal/plugin-interface';
import { z } from 'zod';

interface ValidationRule {
  type: string;
  field: string;
  rule: (value: any) => boolean;
  message: string;
}

interface ValidationResult {
  valid: boolean;
  errors: string[];
  warnings: string[];
}

export class ValidationPlugin extends BasePlugin {
  name = 'ValidationPlugin';
  version = '1.0.0';
  priority = 20; // Run after metrics but before other plugins

  private validationRules = new Map<string, ValidationRule[]>();
  private context?: PluginContext;
  private validationStats = {
    totalValidations: 0,
    passed: 0,
    failed: 0,
    warnings: 0,
  };

  async onInit(context: PluginContext): Promise<void> {
    this.context = context;
    this.setupDefaultRules();
    context.logger.debug('ValidationPlugin initialized');
  }

  /**
   * Setup default validation rules for each entity type
   */
  private setupDefaultRules(): void {
    // Organization rules
    this.addRule('organization', 'legalName', 
      (value) => value && value.length >= 3 && value.length <= 255,
      'Legal name must be between 3 and 255 characters'
    );

    this.addRule('organization', 'doingBusinessAs',
      (value) => !value || (value.length >= 3 && value.length <= 255),
      'Trading name must be between 3 and 255 characters if provided'
    );

    // Ledger rules
    this.addRule('ledger', 'name',
      (value) => value && value.length >= 3 && value.length <= 100,
      'Ledger name must be between 3 and 100 characters'
    );

    this.addRule('ledger', 'status',
      (value) => ['ACTIVE', 'INACTIVE', 'PENDING'].includes(value),
      'Ledger status must be ACTIVE, INACTIVE, or PENDING'
    );

    // Asset rules
    this.addRule('asset', 'code',
      (value) => value && /^[A-Z]{3,10}$/.test(value),
      'Asset code must be 3-10 uppercase letters'
    );

    this.addRule('asset', 'scale',
      (value) => Number.isInteger(value) && value >= 0 && value <= 18,
      'Asset scale must be an integer between 0 and 18'
    );

    // Account rules
    this.addRule('account', 'alias',
      (value) => value && value.length >= 3 && value.length <= 100,
      'Account alias must be between 3 and 100 characters'
    );

    this.addRule('account', 'type',
      (value) => ['ASSET', 'LIABILITY', 'EQUITY', 'INCOME', 'EXPENSE'].includes(value),
      'Invalid account type'
    );

    // Transaction rules
    this.addRule('transaction', 'amount',
      (value) => Number.isInteger(value) && value > 0,
      'Transaction amount must be a positive integer'
    );

    this.addRule('transaction', 'operations',
      (value) => Array.isArray(value) && value.length >= 2,
      'Transaction must have at least 2 operations'
    );
  }

  /**
   * Add a validation rule
   */
  addRule(entityType: string, field: string, rule: (value: any) => boolean, message: string): void {
    if (!this.validationRules.has(entityType)) {
      this.validationRules.set(entityType, []);
    }

    this.validationRules.get(entityType)!.push({
      type: entityType,
      field,
      rule,
      message,
    });
  }

  /**
   * Validate an entity
   */
  private validateEntity(type: string, entity: any): ValidationResult {
    const result: ValidationResult = {
      valid: true,
      errors: [],
      warnings: [],
    };

    const rules = this.validationRules.get(type) || [];

    for (const rule of rules) {
      try {
        const value = this.getFieldValue(entity, rule.field);
        
        if (!rule.rule(value)) {
          result.valid = false;
          result.errors.push(`${rule.field}: ${rule.message}`);
        }
      } catch (error) {
        result.warnings.push(`Failed to validate ${rule.field}: ${error}`);
      }
    }

    // Additional business logic validations
    this.performBusinessLogicValidation(type, entity, result);

    return result;
  }

  /**
   * Perform business logic validations
   */
  private performBusinessLogicValidation(type: string, entity: any, result: ValidationResult): void {
    switch (type) {
      case 'account':
        // Check if account has valid asset code
        if (entity.assetCode && !this.isValidAssetCode(entity.assetCode)) {
          result.warnings.push('Account references unknown asset code');
        }
        break;

      case 'transaction':
        // Validate double-entry bookkeeping
        if (entity.operations) {
          const debits = entity.operations
            .filter((op: any) => op.type === 'DEBIT')
            .reduce((sum: number, op: any) => sum + (op.amount?.value || 0), 0);
          
          const credits = entity.operations
            .filter((op: any) => op.type === 'CREDIT')
            .reduce((sum: number, op: any) => sum + (op.amount?.value || 0), 0);

          if (debits !== credits) {
            result.errors.push('Transaction is not balanced (debits != credits)');
            result.valid = false;
          }
        }
        break;

      case 'portfolio':
        // Validate portfolio has valid segments
        if (entity.segments && Array.isArray(entity.segments)) {
          if (entity.segments.length === 0) {
            result.warnings.push('Portfolio has no segments');
          }
        }
        break;
    }
  }

  /**
   * Check if asset code exists in state
   */
  private isValidAssetCode(assetCode: string): boolean {
    if (!this.context) return true; // Can't validate without context

    const state = this.context.stateManager.getState();
    
    // Check all ledgers for this asset code
    for (const [_, assetCodes] of state.assetCodes) {
      if (assetCodes.includes(assetCode)) {
        return true;
      }
    }

    return false;
  }

  /**
   * Get nested field value
   */
  private getFieldValue(obj: any, field: string): any {
    const parts = field.split('.');
    let value = obj;

    for (const part of parts) {
      value = value?.[part];
    }

    return value;
  }

  async afterEntityGeneration(context: EntityContext): Promise<void> {
    const result = this.validateEntity(context.type, context.entity);
    
    this.validationStats.totalValidations++;
    
    if (result.valid) {
      this.validationStats.passed++;
    } else {
      this.validationStats.failed++;
      
      if (this.context) {
        this.context.logger.warn(
          `Validation failed for ${context.type}: ${result.errors.join(', ')}`
        );
      }
    }

    if (result.warnings.length > 0) {
      this.validationStats.warnings += result.warnings.length;
      
      if (this.context) {
        result.warnings.forEach(warning => {
          this.context!.logger.debug(`Validation warning for ${context.type}: ${warning}`);
        });
      }
    }
  }

  async afterGeneration(): Promise<void> {
    if (this.context) {
      this.context.logger.info(
        `Validation Summary: ${this.validationStats.passed}/${this.validationStats.totalValidations} passed, ` +
        `${this.validationStats.failed} failed, ${this.validationStats.warnings} warnings`
      );
    }

    // Reset stats
    this.validationStats = {
      totalValidations: 0,
      passed: 0,
      failed: 0,
      warnings: 0,
    };
  }

  /**
   * Get validation statistics
   */
  getStats(): typeof this.validationStats {
    return { ...this.validationStats };
  }

  /**
   * Clear all custom validation rules
   */
  clearCustomRules(): void {
    this.validationRules.clear();
    this.setupDefaultRules();
  }
}