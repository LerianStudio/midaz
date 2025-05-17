/** @type {import('jest').Config} */
module.exports = {
  // Use the default Jest environment for Node.js
  testEnvironment: 'node',
  
  // Include both .js and .ts tests
  testMatch: [
    '**/tests/**/*.test.js',
    '**/tests/**/*.test.ts'
  ],
  
  // Transform TypeScript files
  transform: {
    '^.+\\.tsx?$': ['ts-jest', {
      // Use a simpler tsconfig for tests
      tsconfig: {
        "compilerOptions": {
          "target": "es6",
          "module": "commonjs",
          "esModuleInterop": true,
          "strict": false,
          "noImplicitAny": false
        }
      }
    }]
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
    '^midaz-sdk-typescript/(.*)$': '<rootDir>/node_modules/midaz-sdk-typescript/$1',
    '^../../midaz-sdk-typescript/(.*)$': '<rootDir>/node_modules/midaz-sdk-typescript/$1',
    '^../../../midaz-sdk-typescript/(.*)$': '<rootDir>/node_modules/midaz-sdk-typescript/$1'
  }
};
