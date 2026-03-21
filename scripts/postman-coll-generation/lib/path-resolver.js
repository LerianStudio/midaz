#!/usr/bin/env node

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

/**
 * PathResolver Class
 * 
 * Handles URL path normalization, alternative path generation,
 * and path corrections for API evolution. Centralizes all the
 * path manipulation logic from the original implementation.
 */

class PathResolver {
  constructor(config) {
    this.config = config;
  }

  /**
   * Normalize a path string for comparison
   * @param {string} pathStr - Path string to normalize
   * @returns {string} Normalized path
   */
  normalize(pathStr) {
    if (!pathStr) return '';
    
    let normalized = pathStr.trim();
    
    // Remove trailing slash
    if (normalized.endsWith('/')) {
      normalized = normalized.slice(0, -1);
    }
    
    // Ensure starts with slash
    if (!normalized.startsWith('/')) {
      normalized = '/' + normalized;
    }
    
    // Replace Postman-style {{variables}} with {} for comparison
    normalized = normalized.replace(/\{\{[^}]+\}\}/g, '{}');
    
    // Replace standard {parameters} with {} for comparison
    normalized = normalized.replace(/\{[^}]+\}/g, '{}');

    return normalized;
  }

  /**
   * Extract path from Postman URL object or raw string
   * @param {Object|string} urlObject - URL object from Postman or string
   * @returns {string} Extracted path
   */
  extractPath(urlObject) {
    if (!urlObject) return '';
    
    // Handle string URLs
    if (typeof urlObject === 'string') {
      try {
        const url = new URL(urlObject);
        return url.pathname;
      } catch {
        // If not a valid URL, treat as path
        return urlObject;
      }
    }
    
    // Handle Postman URL object
    if (urlObject.path && Array.isArray(urlObject.path)) {
      return '/' + urlObject.path.join('/');
    }
    
    // Handle raw URL string in object
    if (urlObject.raw) {
      return this.extractPath(urlObject.raw);
    }
    
    return '';
  }

  /**
   * Generate alternative paths based on configuration patterns
   * @param {string} normalizedPath - Normalized target path
   * @returns {Array} Array of alternative paths
   */
  generateAlternatives(normalizedPath) {
    const alternatives = [];
    
    // Apply configured path corrections
    for (const correction of this.config.apiPatterns.pathCorrections) {
      if (correction.detect.test(normalizedPath)) {
        try {
          const alternative = correction.correct(normalizedPath);
          if (alternative && alternative !== normalizedPath) {
            alternatives.push(this.normalize(alternative));
          }
        } catch (error) {
          console.warn(`Failed to apply path correction "${correction.name}": ${error.message}`);
        }
      }
    }
    
    // Generate common path variations
    alternatives.push(...this.generateCommonVariations(normalizedPath));
    
    // Remove duplicates and the original path
    return [...new Set(alternatives)].filter(alt => alt !== normalizedPath);
  }

  /**
   * Generate common path variations based on known patterns
   * @param {string} normalizedPath - Normalized path
   * @returns {Array} Array of path variations
   */
  generateCommonVariations(normalizedPath) {
    const variations = [];
    
    // Handle /organizations/{}/accounts/{} vs /organizations/{}/ledgers/{}/accounts/{}
    if (normalizedPath.includes('/organizations/{}/accounts/')) {
      const altPath = normalizedPath.replace(
        '/organizations/{}/accounts/',
        '/organizations/{}/ledgers/{}/accounts/'
      );
      variations.push(altPath);
    }
    
    // Handle /organizations/{}/balances vs /organizations/{}/ledgers/{}/balances  
    if (normalizedPath.includes('/organizations/{}/balances')) {
      const altPath = normalizedPath.replace(
        '/organizations/{}/balances',
        '/organizations/{}/ledgers/{}/balances'
      );
      variations.push(altPath);
    }
    
    // Handle operations path variations
    if (normalizedPath.includes('/operations/')) {
      // Try with ledgers in the path
      if (!normalizedPath.includes('/ledgers/')) {
        const altPath1 = normalizedPath.replace(
          '/organizations/{}/operations/',
          '/organizations/{}/ledgers/{}/operations/'
        );
        variations.push(altPath1);
      }
      
      // Try with accounts in the path
      if (!normalizedPath.includes('/accounts/')) {
        const altPath2 = normalizedPath.replace(
          '/organizations/{}/operations/',
          '/organizations/{}/ledgers/{}/accounts/{}/operations/'
        );
        variations.push(altPath2);
        
        // Also try with ledgers/{}/operations/ -> ledgers/{}/accounts/{}/operations/
        const altPath3 = normalizedPath.replace(
          '/organizations/{}/ledgers/{}/operations/',
          '/organizations/{}/ledgers/{}/accounts/{}/operations/'
        );
        variations.push(altPath3);
      }
    }
    
    // Handle asset-rates path variations
    if (normalizedPath.includes('/asset-rates') && !normalizedPath.includes('/ledgers/')) {
      const altPath = normalizedPath.replace(
        '/organizations/{}/asset-rates',
        '/organizations/{}/ledgers/{}/asset-rates'
      );
      variations.push(altPath);
    }
    
    // Handle missing version prefix
    if (!normalizedPath.startsWith('/v1/')) {
      variations.push('/v1' + normalizedPath);
    }
    
    // Handle with version prefix removed
    if (normalizedPath.startsWith('/v1/')) {
      variations.push(normalizedPath.substring(3));
    }
    
    return variations;
  }

  /**
   * Apply path corrections based on configuration
   * @param {string} path - Original path
   * @returns {string} Corrected path
   */
  correctPath(path) {
    let correctedPath = path;
    
    for (const correction of this.config.apiPatterns.pathCorrections) {
      if (correction.detect.test(correctedPath)) {
        try {
          correctedPath = correction.correct(correctedPath);
          console.log(`Applied path correction "${correction.name}": ${path} -> ${correctedPath}`);
          break; // Apply only the first matching correction
        } catch (error) {
          console.warn(`Failed to apply path correction "${correction.name}": ${error.message}`);
        }
      }
    }
    
    return correctedPath;
  }

  /**
   * Determine the base URL for a given path
   * @param {string} path - API path
   * @returns {string} Base URL variable name
   */
  determineBaseUrl(path) {
    const transactionPaths = this.config.apiPatterns.serviceRouting.transaction;
    const onboardingPaths = this.config.apiPatterns.serviceRouting.onboarding;
    
    // Check if path matches transaction service patterns
    for (const pattern of transactionPaths) {
      if (path.includes(pattern)) {
        return this.config.baseUrls.transaction;
      }
    }
    
    // Check if path matches onboarding service patterns
    for (const pattern of onboardingPaths) {
      if (path.includes(pattern)) {
        return this.config.baseUrls.onboarding;
      }
    }
    
    // Default to onboarding service
    return this.config.baseUrls.onboarding;
  }

  /**
   * Compare two paths for equality after normalization
   * @param {string} path1 - First path
   * @param {string} path2 - Second path
   * @returns {boolean} True if paths are equivalent
   */
  pathsEqual(path1, path2) {
    return this.normalize(path1) === this.normalize(path2);
  }

  /**
   * Get path segments as array
   * @param {string} path - Path string
   * @returns {Array} Array of path segments
   */
  getSegments(path) {
    const normalized = this.normalize(path);
    return normalized.split('/').filter(segment => segment.length > 0);
  }

  /**
   * Build path from segments array
   * @param {Array} segments - Array of path segments
   * @returns {string} Constructed path
   */
  buildPath(segments) {
    if (!Array.isArray(segments)) return '';
    return '/' + segments.join('/');
  }

  /**
   * Extract parameter names from path
   * @param {string} path - Path with parameters
   * @returns {Array} Array of parameter names
   */
  extractParameters(path) {
    const parameters = [];
    const matches = path.matchAll(/\{([^}]+)\}/g);
    
    for (const match of matches) {
      parameters.push(match[1]);
    }
    
    return parameters;
  }

  /**
   * Validate path format
   * @param {string} path - Path to validate
   * @returns {Object} Validation result
   */
  validatePath(path) {
    const result = {
      isValid: true,
      errors: [],
      warnings: []
    };
    
    if (!path) {
      result.isValid = false;
      result.errors.push('Path is empty or null');
      return result;
    }
    
    if (!path.startsWith('/')) {
      result.errors.push('Path should start with "/"');
    }
    
    if (path.endsWith('/') && path.length > 1) {
      result.warnings.push('Path ends with "/" (will be normalized)');
    }
    
    // Check for invalid characters
    const invalidChars = /[^a-zA-Z0-9\-_\/{}]/;
    if (invalidChars.test(path)) {
      result.warnings.push('Path contains unusual characters');
    }
    
    // Check for double slashes
    if (path.includes('//')) {
      result.errors.push('Path contains double slashes');
    }
    
    // Validate parameter syntax
    const paramMatches = path.matchAll(/\{([^}]*)\}/g);
    for (const match of paramMatches) {
      const paramName = match[1];
      if (!paramName) {
        result.errors.push('Empty parameter brackets found');
      } else if (!/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(paramName)) {
        result.warnings.push(`Parameter "${paramName}" has unusual format`);
      }
    }
    
    result.isValid = result.errors.length === 0;
    return result;
  }
}

module.exports = { PathResolver };
