#!/usr/bin/env node

/**
 * Script to convert OpenAPI specification to Postman Collection
 * Usage: node convert-openapi.js <input-file> <output-file>
 */

const fs = require('fs');
const path = require('path');
const { exec } = require('child_process');
const { promisify } = require('util');

const execAsync = promisify(exec);

// Check arguments
if (process.argv.length < 4) {
  console.error('Usage: node convert-openapi.js <input-file> <output-file>');
  process.exit(1);
}

const inputFile = process.argv[2];
const outputFile = process.argv[3];

// Check if input file exists
if (!fs.existsSync(inputFile)) {
  console.error(`Input file not found: ${inputFile}`);
  process.exit(1);
}

// Create a temporary directory for the conversion
const tempDir = path.dirname(outputFile);
if (!fs.existsSync(tempDir)) {
  fs.mkdirSync(tempDir, { recursive: true });
}

// Read and parse the input file
const openApiSpec = JSON.parse(fs.readFileSync(inputFile, 'utf8'));

// Fix path parameter formats (replace :param with {param})
function fixPathParameters(spec) {
  const paths = spec.paths || {};
  const fixedPaths = {};
  
  for (const path in paths) {
    // Replace :param with {param} in path
    const fixedPath = path
      .replace(/:organization_id/g, '{organization_id}')
      .replace(/:ledger_id/g, '{ledger_id}')
      .replace(/:account_id/g, '{account_id}')
      .replace(/:transaction_id/g, '{transaction_id}')
      .replace(/:operation_id/g, '{operation_id}')
      .replace(/:balance_id/g, '{balance_id}')
      .replace(/:external_id/g, '{external_id}')
      .replace(/:asset_code/g, '{asset_code}')
      .replace(/:id/g, '{id}')
      .replace(/:alias/g, '{alias}');
    
    fixedPaths[fixedPath] = paths[path];
  }
  
  spec.paths = fixedPaths;
  return spec;
}

// Fix the OpenAPI spec
const fixedSpec = fixPathParameters(openApiSpec);

// Write the fixed spec to a temporary file
const tempFile = `${tempDir}/fixed-spec.json`;
fs.writeFileSync(tempFile, JSON.stringify(fixedSpec, null, 2));

// Use postman-collection-transformer to convert the spec
async function convertToPostman() {
  try {
    // Install required packages if not already installed
    await execAsync('npm list -g postman-collection-transformer || npm install -g postman-collection-transformer');
    
    // Convert OpenAPI to Postman Collection
    const { stdout, stderr } = await execAsync(`postman-collection-transformer convert -i ${tempFile} -o ${outputFile} -t 2.0.0 -f openapi -j`);
    
    if (stderr) {
      console.error(`Error during conversion: ${stderr}`);
      return false;
    }
    
    console.log(`Successfully converted ${inputFile} to ${outputFile}`);
    return true;
  } catch (error) {
    console.error(`Failed to convert: ${error.message}`);
    return false;
  } finally {
    // Clean up temporary file
    if (fs.existsSync(tempFile)) {
      fs.unlinkSync(tempFile);
    }
  }
}

// Execute the conversion
convertToPostman().then(success => {
  process.exit(success ? 0 : 1);
});
