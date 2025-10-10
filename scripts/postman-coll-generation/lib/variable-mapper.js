#!/usr/bin/env node

/**
 * @file VariableMapper Class
 * @description
 * This class handles parameter substitution and variable mapping for workflow steps.
 * It centralizes all complex variable mapping logic from the original implementation,
 * providing improved maintainability and configurability.
 */

class VariableMapper {
  /**
   * @param {Object} config - The workflow configuration object.
   */
  constructor(config) {
    this.config = config;
  }

  /**
   * Maps variables in a path array (Postman format).
   * @param {string} stepPath - The original step path from the Markdown file.
   * @param {Array} urlPath - The URL path array from the Postman request.
   * @returns {Array} The mapped path array.
   */
  mapPath(stepPath, urlPath) {
    if (!Array.isArray(urlPath)) {
      return [];
    }

    // Rebuild the path array from the workflow step path to ensure correct structure
    const expectedPath = stepPath.replace(/^\//, '').split('/');
    
    return expectedPath.map(segment => {
      return this.mapPathSegment(segment, stepPath);
    });
  }

  /**
   * Maps a single path segment to its corresponding variable.
   * @param {string} segment - The path segment to map.
   * @param {string} fullPath - The full path for context-aware mapping.
   * @returns {string} The mapped segment.
   */
  mapPathSegment(segment, fullPath) {
    // Handle direct parameter mappings first
    const directMapping = this.config.variables.mapping.direct[segment];
    if (directMapping) {
      return directMapping;
    }

    // Handle contextual {id} parameter mapping
    if (segment === '{id}') {
      return this.mapContextualId(fullPath);
    }

    // Handle other contextual parameters
    const contextualMappings = this.config.variables.mapping.contextual[segment];
    if (contextualMappings) {
      for (const mapping of contextualMappings) {
        if (mapping.pattern.test(fullPath)) {
          return mapping.replacement;
        }
      }
    }

    // Return segment unchanged if no mapping found
    return segment;
  }

  /**
   * Maps a contextual `{id}` parameter based on the URL.
   * @param {string} path - The full path for context determination.
   * @returns {string} The appropriate variable replacement (e.g., "{{organizationId}}").
   */
  mapContextualId(path) {
    const contextualMappings = this.config.variables.mapping.contextual['{id}'];
    
    for (const mapping of contextualMappings) {
      if (mapping.pattern.test(path)) {
        return mapping.replacement;
      }
    }
    
    // Fallback to generic ID
    return '{{id}}';
  }

  /**
   * Maps variables in a request body.
   * @param {string} bodyString - The request body as a string.
   * @param {Object} context - The context for variable mapping.
   * @returns {string} The request body with mapped variables.
   */
  mapBodyVariables(bodyString, context = {}) {
    if (!bodyString) return bodyString;

    let mappedBody = bodyString;
    const directMappings = this.config.variables.mapping.direct;

    // Apply direct mappings
    for (const [placeholder, replacement] of Object.entries(directMappings)) {
      // Create regex to match the placeholder (with optional surrounding quotes)
      const regex = new RegExp(`"?${this.escapeRegex(placeholder)}"?`, 'g');
      mappedBody = mappedBody.replace(regex, `"${replacement}"`);
    }

    return mappedBody;
  }

  /**
   * Maps variables in query parameters.
   * @param {Array} queryParams - An array of query parameter objects.
   * @returns {Array} An array with mapped query parameters.
   */
  mapQueryParameters(queryParams) {
    if (!Array.isArray(queryParams)) {
      return queryParams;
    }

    return queryParams.map(param => {
      const mappedParam = { ...param };
      
      if (mappedParam.value) {
        mappedParam.value = this.mapString(mappedParam.value);
      }
      
      if (mappedParam.key) {
        mappedParam.key = this.mapString(mappedParam.key);
      }

      return mappedParam;
    });
  }

  /**
   * Maps variables in HTTP headers.
   * @param {Array} headers - An array of header objects.
   * @returns {Array} An array with mapped headers.
   */
  mapHeaders(headers) {
    if (!Array.isArray(headers)) {
      return headers;
    }

    return headers.map(header => {
      const mappedHeader = { ...header };
      
      if (mappedHeader.value) {
        mappedHeader.value = this.mapString(mappedHeader.value);
      }
      
      if (mappedHeader.key) {
        mappedHeader.key = this.mapString(mappedHeader.key);
      }

      return mappedHeader;
    });
  }

  /**
   * Maps variables in a generic string.
   * @param {string} str - The string to process.
   * @returns {string} The string with mapped variables.
   */
  mapString(str) {
    if (typeof str !== 'string') return str;

    let mappedString = str;
    const directMappings = this.config.variables.mapping.direct;

    for (const [placeholder, replacement] of Object.entries(directMappings)) {
      const regex = new RegExp(this.escapeRegex(placeholder), 'g');
      mappedString = mappedString.replace(regex, replacement);
    }

    return mappedString;
  }

  /**
   * Gets the required variables for a workflow step.
   * @param {Object} step - The step object from the parsed workflow.
   * @returns {Array} An array of required variable names.
   */
  getRequiredVariables(step) {
    const required = new Set();

    // Add base required variables
    required.add('organizationId');
    required.add('ledgerId');

    // Extract from step uses
    if (step.uses && Array.isArray(step.uses)) {
      step.uses.forEach(use => {
        const varName = typeof use === 'string' ? use : use.variable;
        if (varName) {
          required.add(varName);
        }
      });
    }

    // Add variables based on path analysis
    const pathVars = this.extractPathVariables(step.path);
    pathVars.forEach(varName => required.add(varName));

    // Add method-specific requirements
    if (step.method && ['PATCH', 'DELETE', 'GET'].includes(step.method)) {
      if (step.path.includes('/transactions/')) {
        required.add('transactionId');
      }
      if (step.path.includes('/operations/')) {
        required.add('operationId');
      }
      if (step.path.includes('/balances/')) {
        required.add('balanceId');
      }
      if (step.path.includes('/accounts/')) {
        required.add('accountId');
      }
      if (step.path.includes('/assets/')) {
        required.add('assetId');
      }
      if (step.path.includes('/portfolios/')) {
        required.add('portfolioId');
      }
      if (step.path.includes('/segments/')) {
        required.add('segmentId');
      }
    }

    return Array.from(required);
  }

  /**
   * Extracts variable names from a path.
   * @param {string} path - The path to analyze.
   * @returns {Array} An array of variable names found in the path.
   */
  extractPathVariables(path) {
    if (!path) return [];

    const variables = [];
    const directMappings = this.config.variables.mapping.direct;

    // Find all placeholders in the path
    const placeholderMatches = path.matchAll(/\{([^}]+)\}/g);
    
    for (const match of placeholderMatches) {
      const placeholder = `{${match[1]}}`;
      const mapping = directMappings[placeholder];
      
      if (mapping) {
        // Extract variable name from Postman template
        const varMatch = mapping.match(/\{\{([^}]+)\}\}/);
        if (varMatch) {
          variables.push(varMatch[1]);
        }
      }
    }

    return variables;
  }

  /**
   * Validates the completeness of variable mappings for a step.
   * @param {Object} step - The step object from the parsed workflow.
   * @returns {Object} A validation result object.
   */
  validateMappings(step) {
    const result = {
      isValid: true,
      unmappedParameters: [],
      missingVariables: []
    };

    if (!step.path) return result;

    // Find all parameters in the path
    const parameterMatches = step.path.matchAll(/\{([^}]+)\}/g);
    
    for (const match of parameterMatches) {
      const parameter = `{${match[1]}}`;
      const directMapping = this.config.variables.mapping.direct[parameter];
      
      if (!directMapping) {
        // Check for contextual mapping
        const contextualMappings = this.config.variables.mapping.contextual[parameter];
        let hasContextualMapping = false;
        
        if (contextualMappings) {
          for (const mapping of contextualMappings) {
            if (mapping.pattern.test(step.path)) {
              hasContextualMapping = true;
              break;
            }
          }
        }
        
        if (!hasContextualMapping) {
          result.unmappedParameters.push(parameter);
          result.isValid = false;
        }
      }
    }

    return result;
  }

  /**
   * Escapes special characters in a string for use in a regular expression.
   * @param {string} str - The string to escape.
   * @returns {string} The escaped string.
   */
  escapeRegex(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  /**
   * Gets statistics about the configured variable mappings.
   * @returns {Object} An object containing statistics about the mappings.
   */
  getStatistics() {
    const directMappings = this.config.variables.mapping.direct;
    const contextualMappings = this.config.variables.mapping.contextual;

    return {
      directMappings: Object.keys(directMappings).length,
      contextualMappings: Object.keys(contextualMappings).length,
      totalMappingRules: Object.keys(directMappings).length + 
                        Object.values(contextualMappings).flat().length,
      supportedParameters: Object.keys(directMappings).concat(Object.keys(contextualMappings))
    };
  }
}

module.exports = { VariableMapper };