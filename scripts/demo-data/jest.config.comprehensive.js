/**
 * Comprehensive Jest configuration for all tests
 */

module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  
  // Test patterns
  testMatch: [
    '**/tests/**/*.test.ts',
    '**/tests/**/*.spec.ts',
  ],
  
  // Setup files
  setupFilesAfterEnv: ['<rootDir>/tests/setup.js'],
  
  // Coverage configuration
  collectCoverageFrom: [
    'src/**/*.ts',
    '!src/**/*.d.ts',
    '!src/index.ts', // Entry point
    '!src/types.ts', // Type definitions
    '!src/config.ts', // Static configuration
  ],
  
  coverageReporters: [
    'text',
    'text-summary',
    'html',
    'lcov',
  ],
  
  coverageDirectory: 'coverage',
  
  // Coverage thresholds
  coverageThreshold: {
    global: {
      branches: 80,
      functions: 80,
      lines: 80,
      statements: 80,
    },
  },
  
  // Module resolution
  moduleNameMapping: {
    '^@/(.*)$': '<rootDir>/src/$1',
  },
  
  // Timeout configuration
  testTimeout: 30000,
  
  // Reporter configuration
  reporters: [
    'default',
    [
      'jest-junit',
      {
        outputDirectory: 'test-results',
        outputName: 'junit.xml',
        suiteNameTemplate: '{filepath}',
        classNameTemplate: '{classname}',
        titleTemplate: '{title}',
      },
    ],
  ],
  
  // Verbose output for CI
  verbose: process.env.CI === 'true',
  
  // Detect open handles
  detectOpenHandles: true,
  forceExit: true,
  
  // Transform configuration
  transform: {
    '^.+\\.ts$': 'ts-jest',
  },
  
  // TypeScript configuration
  globals: {
    'ts-jest': {
      tsconfig: {
        module: 'commonjs',
        target: 'es2020',
        lib: ['es2020'],
        moduleResolution: 'node',
        allowSyntheticDefaultImports: true,
        esModuleInterop: true,
        skipLibCheck: true,
        strict: true,
        resolveJsonModule: true,
      },
    },
  },
  
  // Mock configuration
  clearMocks: true,
  restoreMocks: true,
  
  // Test organization
  testPathIgnorePatterns: [
    '/node_modules/',
    '/dist/',
    '/coverage/',
    'sdk-source',
  ],
  
  // Environment variables for testing
  setupFiles: ['<rootDir>/tests/env.setup.js'],
};