/**
 * Logging service for Midaz demo data generator
 */

import { GeneratorOptions } from '../types';

/**
 * Log levels
 */
export enum LogLevel {
  DEBUG = 'DEBUG',
  INFO = 'INFO',
  WARN = 'WARN',
  ERROR = 'ERROR',
}

/**
 * Logger service
 */
export class Logger {
  private readonly debugEnabled: boolean;

  constructor(options: GeneratorOptions) {
    this.debugEnabled = options.debug;
  }

  /**
   * Log a debug message (only when debug is enabled)
   */
  debug(message: string, ...args: any[]): void {
    if (this.debugEnabled) {
      this.log(LogLevel.DEBUG, message, ...args);
    }
  }

  /**
   * Log an informational message
   */
  info(message: string, ...args: any[]): void {
    this.log(LogLevel.INFO, message, ...args);
  }

  /**
   * Log a warning message
   */
  warn(message: string, ...args: any[]): void {
    this.log(LogLevel.WARN, message, ...args);
  }

  /**
   * Log an error message
   */
  error(message: string, error?: Error, ...args: any[]): void {
    const errorMsg = error ? `${message} - ${error.message}` : message;
    this.log(LogLevel.ERROR, errorMsg, ...args);
    
    if (error?.stack && this.debugEnabled) {
      console.error(error.stack);
    }
  }

  /**
   * Log a progress message with completion percentage
   */
  progress(message: string, completed: number, total: number): void {
    const percentage = Math.round((completed / total) * 100);
    this.info(`${message} [${completed}/${total}] ${percentage}%`);
  }

  /**
   * Log generation metrics at the end of a run
   */
  metrics(metrics: {
    totalOrganizations: number;
    totalLedgers: number;
    totalAssets: number;
    totalPortfolios: number;
    totalSegments: number;
    totalAccounts: number;
    totalTransactions: number;
    // Add error counts per entity type
    organizationErrors?: number;
    ledgerErrors?: number;
    assetErrors?: number;
    portfolioErrors?: number;
    segmentErrors?: number;
    accountErrors?: number;
    transactionErrors?: number;
    errors: number;
    retries: number;
    duration: number;
  }): void {
    const durationSec = metrics.duration / 1000;
    
    console.log('\n------------------------------------------');
    console.log(' GENERATION METRICS');
    console.log('------------------------------------------');
    // Format each line to show: Total / Errors
    const orgErrors = metrics.organizationErrors || 0;
    const ledgerErrors = metrics.ledgerErrors || 0;
    const assetErrors = metrics.assetErrors || 0;
    const portfolioErrors = metrics.portfolioErrors || 0;
    const segmentErrors = metrics.segmentErrors || 0;
    const accountErrors = metrics.accountErrors || 0;
    const txErrors = metrics.transactionErrors || 0;
    
    console.log(` Organizations:  ${metrics.totalOrganizations}/${orgErrors}`);
    console.log(` Ledgers:        ${metrics.totalLedgers}/${ledgerErrors}`);
    console.log(` Assets:         ${metrics.totalAssets}/${assetErrors}`);
    console.log(` Portfolios:     ${metrics.totalPortfolios}/${portfolioErrors}`);
    console.log(` Segments:       ${metrics.totalSegments}/${segmentErrors}`);
    console.log(` Accounts:       ${metrics.totalAccounts}/${accountErrors}`);
    console.log(` Transactions:   ${metrics.totalTransactions}/${txErrors}`);
    console.log('------------------------------------------');
    console.log(` Total Errors:   ${metrics.errors}`);
    console.log(` Retries:        ${metrics.retries}`);
    console.log(` Duration:       ${durationSec.toFixed(2)}s`);
    console.log('------------------------------------------');
  }

  /**
   * Internal log method with timestamp and level
   */
  private log(level: LogLevel, message: string, ...args: any[]): void {
    const timestamp = new Date().toISOString();
    const prefix = `[${timestamp}] ${level}:`;
    
    if (args.length === 0) {
      console.log(`${prefix} ${message}`);
    } else {
      console.log(`${prefix} ${message}`, ...args);
    }
  }
}
