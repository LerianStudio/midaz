#!/usr/bin/env node

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

/**
 * RequestBodyGenerator Class
 * 
 * Generates request bodies for different transaction types based on templates.
 * Handles the complex transaction body generation logic from the original.
 */

class RequestBodyGenerator {
  constructor(config) {
    this.config = config;
  }

  /**
   * Generate request body for a step
   * @param {Object} step - Step object
   * @returns {Object|null} Request body object or null
   */
  generate(step) {
    if (!this.needsBody(step)) {
      return null;
    }

    // Handle special cases first
    if (step.title === "Zero Out Balance") {
      return this.generateZeroOutBody();
    }

    // Handle transaction endpoints
    if (step.path.includes('/transactions/')) {
      return this.generateTransactionBody(step);
    }

    return null;
  }

  /**
   * Check if step needs body generation
   * @param {Object} step - Step object
   * @returns {boolean} True if body is needed
   */
  needsBody(step) {
    return step.method === 'POST' && (
      step.path.includes('/transactions/') ||
      step.title === "Zero Out Balance"
    );
  }

  /**
   * Generate transaction body based on path
   * @param {Object} step - Step object
   * @returns {Object} Transaction body
   */
  generateTransactionBody(step) {
    if (step.path.includes('/transactions/json')) {
      return this.config.transactions.templates.json;
    } else if (step.path.includes('/transactions/inflow')) {
      return this.config.transactions.templates.inflow;
    } else if (step.path.includes('/transactions/outflow')) {
      return this.config.transactions.templates.outflow;
    }
    
    // Default to JSON transaction
    return this.config.transactions.templates.json;
  }

  /**
   * Generate zero out transaction body
   * @returns {Object} Zero out transaction body
   */
  generateZeroOutBody() {
    return this.config.transactions.templates.zeroOut;
  }
}

module.exports = { RequestBodyGenerator };
