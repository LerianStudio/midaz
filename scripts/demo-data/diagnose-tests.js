#!/usr/bin/env node
/**
 * Test diagnostic script for the Midaz demo data generator
 * This script identifies common issues with Jest setup and outputs diagnostic information
 */

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

console.log('Running Midaz demo data test diagnostics...\n');

// Check for basic dependencies
try {
  console.log('Checking for Jest...');
  const jestVersion = execSync('npx jest --version', { encoding: 'utf8' }).trim();
  console.log(`✅ Jest version ${jestVersion} found`);
} catch (error) {
  console.error('❌ Jest not found or not working properly.');
  console.error('   Try running: npm install --save-dev jest');
  process.exit(1);
}

// Check configurations
console.log('\nChecking Jest configuration...');
const jestConfigPath = path.join(__dirname, 'jest.config.js');
if (fs.existsSync(jestConfigPath)) {
  console.log('✅ Jest config file found');
} else {
  console.error('❌ Jest config file not found at:', jestConfigPath);
}

// Check for test files
console.log('\nChecking for test files...');
const testDir = path.join(__dirname, 'tests');
if (fs.existsSync(testDir)) {
  const testFiles = execSync(`find ${testDir} -name "*.test.js" -o -name "*.test.ts"`, 
                           { encoding: 'utf8' }).trim().split('\n');
  
  if (testFiles.length > 0 && testFiles[0] !== '') {
    console.log(`✅ Found ${testFiles.length} test files:`);
    testFiles.forEach(file => console.log(`   - ${path.relative(__dirname, file)}`));
  } else {
    console.error('❌ No test files found');
  }
} else {
  console.error('❌ Test directory not found at:', testDir);
}

// Try running the basic test
console.log('\nTrying to run basic test...');
try {
  const basicTest = path.join(testDir, 'basic.test.js');
  if (fs.existsSync(basicTest)) {
    console.log('Running basic test with --no-cache --detectOpenHandles...');
    execSync('npx jest tests/basic.test.js --no-cache --detectOpenHandles --forceExit', 
             { stdio: 'inherit' });
    console.log('✅ Basic test executed successfully');
  } else {
    console.log('Basic test file not found, creating one...');
    fs.writeFileSync(
      basicTest,
      `// Basic test file to verify Jest is working
test('Basic test to verify Jest is working', () => {
  expect(1 + 1).toBe(2);
});`
    );
    console.log('Basic test file created, trying to run it...');
    execSync('npx jest tests/basic.test.js --no-cache --detectOpenHandles --forceExit', 
             { stdio: 'inherit' });
  }
} catch (error) {
  console.error('❌ Error running basic test:', error.message);
  console.log('\nDiagnostic information:');
  console.log('Node version:', process.version);
  console.log('Platform:', process.platform);
  
  // Check for common issues
  if (error.message.includes('Cannot find module')) {
    console.log('\nPossible solution: There may be missing dependencies.');
    console.log('Try running: npm install jest ts-jest @types/jest');
  } else if (error.message.includes('EADDRINUSE')) {
    console.log('\nPossible solution: A port used by tests is already in use.');
    console.log('Check for running processes and terminate them, or modify the test to use a different port.');
  }
}

console.log('\nDiagnostic complete.');
