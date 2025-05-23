#!/usr/bin/env node

/**
 * Simple script to run the demo data generator using ts-node
 * This bypasses TypeScript compilation issues with the SDK
 */

const { spawnSync } = require('child_process');
const path = require('path');

// Default volume is small unless specified
const volume = process.argv[2] || 'small';
const validVolumes = ['small', 'medium', 'large'];

if (!validVolumes.includes(volume)) {
  console.error('Invalid volume size. Please use one of: small, medium, large');
  process.exit(1);
}

console.log(`Starting demo data generator with volume: ${volume}`);

// Run the generator using ts-node
const result = spawnSync('npx', ['ts-node', 'src/index.ts', '--volume', volume], {
  cwd: __dirname,
  stdio: 'inherit',
  env: {
    ...process.env,
    TS_NODE_TRANSPILE_ONLY: 'true', // Skip type checking for faster execution
    TS_NODE_SKIP_PROJECT: 'true'    // Skip reading tsconfig.json
  }
});

if (result.error) {
  console.error('Failed to run generator:', result.error);
  process.exit(1);
}

process.exit(result.status);
