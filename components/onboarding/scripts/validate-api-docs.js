#!/usr/bin/env node

/**
 * Midaz API Documentation Validator
 * 
 * This script validates the OpenAPI documentation structure by:
 * 1. Parsing the OpenAPI specification
 * 2. Checking that all documented endpoints have required response codes
 * 3. Verifying parameters are properly defined
 * 
 * Usage: node validate-api-docs.js [--verbose]
 * 
 * Options:
 *   --verbose   Display detailed information during validation
 */

const fs = require('fs');
const path = require('path');
let yaml, program;
try {
  yaml = require('js-yaml');
  program = require('commander').program;
} catch (e) {
  console.error('Error loading required modules. Make sure they are installed:');
  console.error('Run: npm install js-yaml commander');
  process.exit(1);
}

// Configure CLI options
program
  .option('--verbose', 'display detailed information during validation')
  .parse(process.argv);

const options = program.opts();
const isVerbose = options.verbose;

// Configuration
const config = {
  openapiPath: path.join(__dirname, '..', 'api', 'openapi.yaml'),
  
  // Paths to skip during validation
  skipPaths: [
    // Add paths that should be skipped
  ]
};

// Load and parse the OpenAPI specification
function loadOpenApiSpec() {
  try {
    const openapiContent = fs.readFileSync(config.openapiPath, 'utf8');
    return yaml.load(openapiContent);
  } catch (error) {
    console.error(`Failed to load OpenAPI specification: ${error.message}`);
    process.exit(1);
  }
}

// Validate that all documented endpoints have the expected status codes
function validateStatusCodes(operation, path, method) {
  const responses = operation.responses || {};
  const hasSuccess = Object.keys(responses).some(code => code.startsWith('2'));
  const hasClientError = Object.keys(responses).some(code => code.startsWith('4'));
  const hasServerError = Object.keys(responses).some(code => code.startsWith('5'));
  
  let valid = true;
  
  if (!hasSuccess) {
    console.error(`✗ ${method.toUpperCase()} ${path} - Missing success response code`);
    valid = false;
  }
  
  if (!hasClientError) {
    console.error(`✗ ${method.toUpperCase()} ${path} - Missing client error response code`);
    valid = false;
  }
  
  if (!hasServerError) {
    console.error(`✗ ${method.toUpperCase()} ${path} - Missing server error response code`);
    valid = false;
  }
  
  return valid;
}

// Validate parameter definitions
function validateParameters(operation, path, method) {
  const parameters = operation.parameters || [];
  let valid = true;
  
  // Check for required parameters
  parameters.forEach(param => {
    if (param.required === true && !param.schema) {
      console.error(`✗ ${method.toUpperCase()} ${path} - Required parameter '${param.name}' missing schema definition`);
      valid = false;
    }
  });
  
  // For path parameters, ensure they're defined
  const pathParams = path.match(/{([^}]+)}/g) || [];
  pathParams.forEach(pathParam => {
    const paramName = pathParam.slice(1, -1); // Remove { and }
    const isDefined = parameters.some(p => p.name === paramName && p.in === 'path');
    
    if (!isDefined) {
      console.error(`✗ ${method.toUpperCase()} ${path} - Path parameter '${paramName}' not defined in parameters`);
      valid = false;
    }
  });
  
  return valid;
}

// Process OpenAPI specification
function processOpenApiSpec() {
  const spec = loadOpenApiSpec();
  console.log(`Validating OpenAPI specification (version ${spec.info.version})`);
  
  let validEndpoints = 0;
  let totalEndpoints = 0;
  
  for (const [path, pathObj] of Object.entries(spec.paths || {})) {
    // Skip paths in the skipPaths list
    if (config.skipPaths.includes(path)) {
      continue;
    }
    
    for (const [method, operation] of Object.entries(pathObj)) {
      if (method === 'parameters' || method === 'servers') {
        continue; // Skip non-operation fields
      }
      
      totalEndpoints++;
      
      // Validate status codes
      const statusValid = validateStatusCodes(operation, path, method);
      
      // Validate parameters
      const paramsValid = validateParameters(operation, path, method);
      
      // For this validation script, we just validate the structure of the documentation
      const structureValid = statusValid && paramsValid;
      
      if (isVerbose) {
        if (structureValid) {
          console.log(`✓ ${method.toUpperCase()} ${path} - Documentation structure is valid`);
        }
      }
      
      if (structureValid) {
        validEndpoints++;
      }
    }
  }
  
  console.log(`\nValidation Summary:`);
  console.log(`✓ ${validEndpoints} of ${totalEndpoints} endpoints have valid documentation structure`);
  
  if (validEndpoints < totalEndpoints) {
    console.log(`✗ ${totalEndpoints - validEndpoints} endpoints have documentation issues that need to be fixed`);
    return false;
  }
  
  return true;
}

// Main function
function main() {
  const start = Date.now();
  const success = processOpenApiSpec();
  const duration = ((Date.now() - start) / 1000).toFixed(2);
  
  console.log(`\nValidation completed in ${duration}s`);
  
  process.exit(success ? 0 : 1);
}

main();