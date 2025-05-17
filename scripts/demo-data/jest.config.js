/** @type {import('jest').Config} */
module.exports = {
  // Use the default Jest environment for Node.js
  testEnvironment: 'node',
  
  // Include both .js and .ts tests
  testMatch: [
    '**/tests/**/*.test.js',
    '**/tests/**/*.test.ts'
  ],
  
  // Exclude sdk-source directory from testing
  testPathIgnorePatterns: [
    '/node_modules/',
    '/sdk-source/'
  ],
  
  // Transform TypeScript files
  preset: 'ts-jest',
  transform: {
    '^.+\\.tsx?$': 'ts-jest'
  },
  
  // Setup files to run before tests
  setupFiles: ['./tests/setup.js'],
  
  // Verbose output for diagnostics
  verbose: true,
  
  // Use CommonJS modules
  testEnvironmentOptions: {
    NODE_ENV: 'test'
  },
  
  // Ensure the mock SDK module is available
  moduleNameMapper: {
    '^midaz-sdk$': '<rootDir>/sdk-source/src',
    '^midaz-sdk/(.*)$': '<rootDir>/sdk-source/src/$1'
  }
};
