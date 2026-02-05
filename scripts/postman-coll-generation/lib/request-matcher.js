#!/usr/bin/env node

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

/**
 * RequestMatcher Class
 * 
 * Finds matching requests in the Postman collection based on method and path.
 * Includes sophisticated matching with alternatives and logging for debugging.
 */

const { PathResolver } = require('./path-resolver');

class RequestMatcher {
  constructor(config) {
    this.config = config;
    this.pathResolver = new PathResolver(config);
    this.matchAttempts = new Map(); // Track matching attempts for debugging
  }

  /**
   * Find a request matching method and path in collection
   * @param {string} method - HTTP method
   * @param {string} path - API path
   * @param {Object} collection - Postman collection
   * @returns {Object|null} Matching request item or null
   */
  async find(method, path, collection) {
    // Initialize match tracking
    const attemptKey = `${method} ${path}`;
    this.matchAttempts.set(attemptKey, {
      method,
      path,
      normalizedTarget: '',
      alternatives: [],
      candidates: [],
      result: null,
      timestamp: new Date().toISOString()
    });

    // Normalize target path
    const normalizedTarget = this.pathResolver.normalize(path);
    this.matchAttempts.get(attemptKey).normalizedTarget = normalizedTarget;

    console.log(`🔍 Searching for: ${method} ${path} (normalized: ${normalizedTarget})`);

    // Generate alternatives based on config
    const alternatives = this.pathResolver.generateAlternatives(normalizedTarget);
    this.matchAttempts.get(attemptKey).alternatives = alternatives;

    if (alternatives.length > 0) {
      console.log(`   Generated ${alternatives.length} alternative paths:`);
      alternatives.forEach((alt, index) => {
        console.log(`     ${index + 1}. ${alt}`);
      });
    }

    // Search collection
    const candidates = this.searchCollection(collection, method, [
      normalizedTarget,
      ...alternatives,
    ]);

    this.matchAttempts.get(attemptKey).candidates = candidates.map(c => ({
      name: c.name,
      method: c.request?.method,
      path: this.pathResolver.extractPath(c.request?.url),
      normalizedPath: this.pathResolver.normalize(this.pathResolver.extractPath(c.request?.url))
    }));

    if (candidates.length === 0) {
      this.logMissing(method, path, normalizedTarget, alternatives);
      this.matchAttempts.get(attemptKey).result = null;
      return null;
    }

    if (candidates.length > 1) {
      this.logAmbiguous(method, path, candidates);
    }

    const selectedCandidate = this.selectBestCandidate(candidates, normalizedTarget);
    this.matchAttempts.get(attemptKey).result = {
      name: selectedCandidate.name,
      selected: true,
      reason: 'best_match'
    };

    console.log(`✅ Selected: ${selectedCandidate.name}`);
    return selectedCandidate;
  }

  /**
   * Search collection recursively for matching requests
   * @param {Object} collection - Postman collection
   * @param {string} method - HTTP method
   * @param {Array} targetPaths - Array of target paths to match
   * @returns {Array} Array of matching request items
   */
  searchCollection(collection, method, targetPaths) {
    const results = [];
    const targetPathsSet = new Set(targetPaths);

    const search = (items, folderPath = '') => {
      for (const item of items) {
        // If it's a request, check if it matches
        if (item.request) {
          const requestMethod = item.request.method;
          const requestPath = this.pathResolver.extractPath(item.request.url);
          const normalizedRequestPath = this.pathResolver.normalize(requestPath);

          // Log the comparison for debugging
          console.log(`  Comparing: Collection[${requestMethod} ${normalizedRequestPath}] (Name: ${item.name})`);

          // Check if method and any target path matches
          if (requestMethod === method && targetPathsSet.has(normalizedRequestPath)) {
            console.log(`    ✅ Match found: ${item.name}`);
            results.push({
              ...item,
              _matchInfo: {
                originalPath: requestPath,
                normalizedPath: normalizedRequestPath,
                folderPath,
                matchedTargetPath: targetPaths.find(tp => tp === normalizedRequestPath)
              }
            });
          }
        }

        // If it's a folder, search recursively within its items
        if (item.item && Array.isArray(item.item)) {
          const currentFolderPath = folderPath ? `${folderPath}/${item.name}` : item.name;
          search(item.item, currentFolderPath);
        }
      }
    };

    search(collection.item || []);
    return results;
  }

  /**
   * Select the best candidate from multiple matches
   * @param {Array} candidates - Array of candidate matches
   * @param {string} targetPath - Original target path
   * @returns {Object} Best candidate
   */
  selectBestCandidate(candidates, targetPath) {
    if (candidates.length === 1) {
      return candidates[0];
    }

    // Prefer exact matches over alternative matches
    const exactMatches = candidates.filter(c => 
      c._matchInfo?.normalizedPath === targetPath
    );

    if (exactMatches.length === 1) {
      console.log(`   Selecting exact match: ${exactMatches[0].name}`);
      return exactMatches[0];
    }

    if (exactMatches.length > 1) {
      console.log(`   Multiple exact matches, selecting first: ${exactMatches[0].name}`);
      return exactMatches[0];
    }

    // If no exact matches, prefer matches from main folders over nested ones
    const sortedByFolderDepth = candidates.sort((a, b) => {
      const depthA = (a._matchInfo?.folderPath || '').split('/').length;
      const depthB = (b._matchInfo?.folderPath || '').split('/').length;
      return depthA - depthB;
    });

    console.log(`   Selecting by folder depth: ${sortedByFolderDepth[0].name}`);
    return sortedByFolderDepth[0];
  }

  /**
   * Log missing request details
   * @param {string} method - HTTP method
   * @param {string} path - Original path
   * @param {string} normalizedTarget - Normalized target path
   * @param {Array} alternatives - Alternative paths tried
   */
  logMissing(method, path, normalizedTarget, alternatives) {
    console.warn(`❌ No matching request found for: ${method} ${path}`);
    console.warn(`   Normalized target: ${normalizedTarget}`);
    if (alternatives.length > 0) {
      console.warn(`   Tried ${alternatives.length} alternatives:`);
      alternatives.forEach((alt, index) => {
        console.warn(`     ${index + 1}. ${alt}`);
      });
    }
    console.warn(`   💡 Check if the request exists in the collection with correct method and path`);
  }

  /**
   * Log ambiguous matches
   * @param {string} method - HTTP method
   * @param {string} path - Original path
   * @param {Array} candidates - Matching candidates
   */
  logAmbiguous(method, path, candidates) {
    console.warn(`⚠️ Multiple matches found for: ${method} ${path}`);
    candidates.forEach((candidate, index) => {
      const requestPath = this.pathResolver.extractPath(candidate.request?.url);
      console.warn(`   ${index + 1}. ${candidate.name} (${candidate.request?.method} ${requestPath})`);
      console.warn(`      Folder: ${candidate._matchInfo?.folderPath || 'root'}`);
    });
    console.warn(`   💡 Using first match: ${candidates[0].name}`);
  }

  /**
   * Get matching statistics for debugging
   * @returns {Object} Statistics about matching attempts
   */
  getStatistics() {
    const attempts = Array.from(this.matchAttempts.values());
    const successful = attempts.filter(a => a.result !== null);
    const failed = attempts.filter(a => a.result === null);
    
    const methodBreakdown = {};
    attempts.forEach(attempt => {
      if (!methodBreakdown[attempt.method]) {
        methodBreakdown[attempt.method] = { total: 0, successful: 0, failed: 0 };
      }
      methodBreakdown[attempt.method].total++;
      if (attempt.result) {
        methodBreakdown[attempt.method].successful++;
      } else {
        methodBreakdown[attempt.method].failed++;
      }
    });

    return {
      totalAttempts: attempts.length,
      successful: successful.length,
      failed: failed.length,
      successRate: attempts.length > 0 ? (successful.length / attempts.length * 100).toFixed(1) + '%' : '0%',
      methodBreakdown,
      failedRequests: failed.map(f => ({
        method: f.method,
        path: f.path,
        normalizedTarget: f.normalizedTarget,
        alternativesCount: f.alternatives.length
      }))
    };
  }

  /**
   * Get detailed matching history for debugging
   * @returns {Array} Array of matching attempts with details
   */
  getMatchingHistory() {
    return Array.from(this.matchAttempts.values());
  }

  /**
   * Clear matching history
   */
  clearHistory() {
    this.matchAttempts.clear();
  }

  /**
   * Find requests that might be close matches for failed searches
   * @param {Object} collection - Postman collection
   * @param {string} method - HTTP method
   * @param {string} path - Original path
   * @returns {Array} Array of potential close matches
   */
  findSimilarRequests(collection, method, path) {
    const results = [];
    const pathSegments = this.pathResolver.getSegments(path);
    const pathLength = pathSegments.length;

    const search = (items) => {
      for (const item of items) {
        if (item.request) {
          const requestMethod = item.request.method;
          const requestPath = this.pathResolver.extractPath(item.request.url);
          const requestSegments = this.pathResolver.getSegments(requestPath);

          // Calculate similarity score
          let score = 0;
          
          // Method match bonus
          if (requestMethod === method) {
            score += 50;
          }
          
          // Path length similarity
          const lengthDiff = Math.abs(pathLength - requestSegments.length);
          score += Math.max(0, 20 - lengthDiff * 5);
          
          // Segment overlap
          const commonSegments = pathSegments.filter(seg => 
            requestSegments.some(reqSeg => 
              seg === reqSeg || 
              (seg.includes('{}') && reqSeg.includes('{}')) ||
              this.segmentsSimilar(seg, reqSeg)
            )
          );
          score += (commonSegments.length / Math.max(pathLength, requestSegments.length)) * 30;

          if (score >= 30) { // Minimum similarity threshold
            results.push({
              item: item,
              score,
              method: requestMethod,
              path: requestPath,
              name: item.name
            });
          }
        }

        if (item.item && Array.isArray(item.item)) {
          search(item.item);
        }
      }
    };

    search(collection.item || []);
    
    // Sort by score descending
    return results
      .sort((a, b) => b.score - a.score)
      .slice(0, 5); // Return top 5 similar requests
  }

  /**
   * Check if two path segments are similar
   * @param {string} seg1 - First segment
   * @param {string} seg2 - Second segment
   * @returns {boolean} True if segments are similar
   */
  segmentsSimilar(seg1, seg2) {
    if (seg1 === seg2) return true;
    
    // Both are parameters
    if (seg1.includes('{}') && seg2.includes('{}')) return true;
    
    // String similarity (simple check)
    const longer = seg1.length > seg2.length ? seg1 : seg2;
    const shorter = seg1.length > seg2.length ? seg2 : seg1;
    
    if (longer.length === 0) return true;
    
    const similarity = (longer.length - this.levenshteinDistance(longer, shorter)) / longer.length;
    return similarity >= 0.6; // 60% similarity threshold
  }

  /**
   * Calculate Levenshtein distance between two strings
   * @param {string} str1 - First string
   * @param {string} str2 - Second string
   * @returns {number} Edit distance
   */
  levenshteinDistance(str1, str2) {
    const matrix = Array(str2.length + 1).fill(null).map(() => Array(str1.length + 1).fill(null));

    for (let i = 0; i <= str1.length; i++) {
      matrix[0][i] = i;
    }

    for (let j = 0; j <= str2.length; j++) {
      matrix[j][0] = j;
    }

    for (let j = 1; j <= str2.length; j++) {
      for (let i = 1; i <= str1.length; i++) {
        const indicator = str1[i - 1] === str2[j - 1] ? 0 : 1;
        matrix[j][i] = Math.min(
          matrix[j][i - 1] + 1, // deletion
          matrix[j - 1][i] + 1, // insertion  
          matrix[j - 1][i - 1] + indicator // substitution
        );
      }
    }

    return matrix[str2.length][str1.length];
  }
}

module.exports = { RequestMatcher };
