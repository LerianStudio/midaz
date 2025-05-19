#!/usr/bin/env node
/**
 * Simple test script for the Midaz demo data generator
 * Verifies basic functionality without requiring Jest
 */

const path = require('path');
const fs = require('fs');
const { execSync } = require('child_process');

// Define our test cases
const tests = [
  {
    name: 'Config module has expected default values',
    run: () => {
      const config = require('../src/config');
      if (!config.DEFAULT_OPTIONS) {
        throw new Error('DEFAULT_OPTIONS not found in config');
      }
      if (!config.VOLUME_METRICS) {
        throw new Error('VOLUME_METRICS not found in config');
      }
      return 'Config module has expected properties';
    }
  },
  {
    name: 'StateManager is a singleton',
    run: () => {
      const { StateManager } = require('../src/utils/state');
      const instance1 = StateManager.getInstance();
      const instance2 = StateManager.getInstance();
      if (instance1 !== instance2) {
        throw new Error('StateManager is not a singleton');
      }
      return 'StateManager is correctly implemented as a singleton';
    }
  },
  {
    name: 'StateManager can track metrics',
    run: () => {
      const { StateManager } = require('../src/utils/state');
      const manager = StateManager.getInstance();
      manager.reset();
      
      // Add test data
      manager.addOrganizationId('test-org');
      manager.addLedgerId('test-org', 'test-ledger');
      manager.incrementErrorCount('organization');
      manager.incrementRetryCount();
      
      // Check metrics
      const metrics = manager.completeGeneration();
      if (metrics.totalOrganizations !== 1) {
        throw new Error(`Expected totalOrganizations to be 1, got ${metrics.totalOrganizations}`);
      }
      if (metrics.totalLedgers !== 1) {
        throw new Error(`Expected totalLedgers to be 1, got ${metrics.totalLedgers}`);
      }
      if (metrics.organizationErrors !== 1) {
        throw new Error(`Expected organizationErrors to be 1, got ${metrics.organizationErrors}`);
      }
      if (metrics.retries !== 1) {
        throw new Error(`Expected retries to be 1, got ${metrics.retries}`);
      }
      
      return 'StateManager correctly tracks metrics';
    }
  },
  {
    name: 'Script can run in test mode',
    run: () => {
      // Run the generator script with test mode flag
      const scriptPath = path.resolve(__dirname, '../run-generator.sh');
      const result = execSync(`${scriptPath} small --test-mode`, { 
        encoding: 'utf8',
        env: { ...process.env, MIDAZ_TEST_MODE: 'true' }
      });
      
      // Check the output
      if (!result.includes('Running in test mode')) {
        throw new Error('Script did not acknowledge test mode');
      }
      if (!result.includes('Demo data generation completed successfully')) {
        throw new Error('Script did not complete successfully in test mode');
      }
      
      return 'Generator script runs correctly in test mode';
    }
  }
];

// Run the tests
console.log('Running simple tests for Midaz demo data generator\n');

let passed = 0;
let failed = 0;

tests.forEach((test, index) => {
  console.log(`Test ${index + 1}: ${test.name}`);
  try {
    const result = test.run();
    console.log(`✅ PASS: ${result}\n`);
    passed++;
  } catch (error) {
    console.error(`❌ FAIL: ${error.message}\n`);
    failed++;
  }
});

// Print summary
console.log('Test Summary:');
console.log(`Passed: ${passed}`);
console.log(`Failed: ${failed}`);
console.log(`Total: ${tests.length}`);

// Return appropriate exit code
if (failed > 0) {
  process.exit(1);
} else {
  console.log('\nAll tests passed!');
  process.exit(0);
}
