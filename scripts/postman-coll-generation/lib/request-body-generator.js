#!/usr/bin/env node

/**
 * @file RequestBodyGenerator Class
 * @description
 * This class generates request bodies for different transaction types based on
 * predefined templates. It centralizes the logic for creating complex
 * transaction bodies.
 */

class RequestBodyGenerator {
  /**
   * @param {Object} config - The workflow configuration object.
   */
  constructor(config) {
    this.config = config;
  }

  /**
   * Generates a request body for a given workflow step.
   * @param {Object} step - The step object from the parsed workflow.
   * @returns {Object|null} A request body object, or null if no body is needed.
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
   * Checks if a workflow step requires a request body to be generated.
   * @param {Object} step - The step object from the parsed workflow.
   * @returns {boolean} True if a body is needed, false otherwise.
   */
  needsBody(step) {
    return step.method === 'POST' && (
      step.path.includes('/transactions/') ||
      step.title === "Zero Out Balance"
    );
  }

  /**
   * Generates the request body for a transaction based on its path.
   * @param {Object} step - The step object from the parsed workflow.
   * @returns {Object} The generated transaction body.
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
   * Generates the request body for a "zero out" transaction.
   * @returns {Object} The "zero out" transaction body.
   */
  generateZeroOutBody() {
    return this.config.transactions.templates.zeroOut;
  }
}

module.exports = { RequestBodyGenerator };