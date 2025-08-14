#!/usr/bin/env node

/**
 * VariableMapper Class
 * 
 * Handles parameter substitution and variable mapping for workflow steps.
 * Centralizes all the complex variable mapping logic from the original
 * implementation with improved maintainability and configurability.
 */

class VariableMapper {
  constructor(config) {
    this.config = config;
  }

  /**
   * Map variables in a path array (Postman format)
   * @param {string} stepPath - Original step path from markdown
   * @param {Array} urlPath - URL path array from Postman request
   * @returns {Array} Mapped path array
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
   * Map a single path segment
   * @param {string} segment - Path segment to map
   * @param {string} fullPath - Full path for context-aware mapping
   * @returns {string} Mapped segment
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
   * Map contextual {id} parameter based on URL context
   * @param {string} path - Full path for context determination
   * @returns {string} Appropriate variable replacement
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
   * Map variables in request body
   * @param {string} bodyString - Request body as string
   * @param {Object} context - Context for variable mapping
   * @returns {string} Body with mapped variables
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
   * Map query parameters
   * @param {Array} queryParams - Array of query parameter objects
   * @returns {Array} Array with mapped query parameters
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
   * Map headers
   * @param {Array} headers - Array of header objects
   * @returns {Array} Array with mapped headers
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
   * Map variables in a generic string
   * @param {string} str - String to process
   * @returns {string} String with mapped variables
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
   * Get required variables for a step
   * @param {Object} step - Step object
   * @returns {Array} Array of required variable names
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
   * Extract variable names from a path
   * @param {string} path - Path to analyze
   * @returns {Array} Array of variable names found in path
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
   * Validate variable mapping completeness
   * @param {Object} step - Step object
   * @returns {Object} Validation result
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
   * Escape special regex characters
   * @param {string} str - String to escape
   * @returns {string} Escaped string
   */
  escapeRegex(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  /**
   * Get mapping statistics
   * @returns {Object} Statistics about configured mappings
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