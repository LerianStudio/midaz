#!/usr/bin/env node

/**
 * Script to convert OpenAPI specification to Postman Collection
 * Usage: node convert-openapi.js <input-file> <output-file>
 */

const fs = require('fs');
const path = require('path');

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

// Create a simple Postman collection structure
function createPostmanCollection(spec) {
  const info = spec.info || {};
  const paths = spec.paths || {};
  
  // Create collection structure
  const collection = {
    info: {
      name: info.title || 'API Collection',
      description: info.description || '',
      schema: 'https://schema.getpostman.com/json/collection/v2.1.0/collection.json'
    },
    item: []
  };
  
  // Group by tags
  const tagGroups = {};
  
  // Process each path
  for (const path in paths) {
    const pathItem = paths[path];
    
    // Process each method (GET, POST, etc.)
    for (const method in pathItem) {
      if (['get', 'post', 'put', 'delete', 'patch', 'options', 'head'].includes(method)) {
        const operation = pathItem[method];
        const tags = operation.tags || ['default'];
        const tag = tags[0]; // Use the first tag for grouping
        
        if (!tagGroups[tag]) {
          tagGroups[tag] = [];
        }
        
        // Create request item
        const requestItem = {
          name: operation.summary || `${method.toUpperCase()} ${path}`,
          request: {
            method: method.toUpperCase(),
            header: [],
            url: {
              raw: `{{baseUrl}}${path}`,
              host: ['{{baseUrl}}'],
              path: path.split('/').filter(p => p)
            },
            description: operation.description || ''
          },
          response: []
        };
        
        // Add parameters
        if (operation.parameters) {
          // Path parameters
          const pathParams = operation.parameters.filter(p => p.in === 'path');
          if (pathParams.length > 0) {
            requestItem.request.url.variable = pathParams.map(p => ({
              key: p.name,
              value: '',
              description: p.description || ''
            }));
          }
          
          // Query parameters
          const queryParams = operation.parameters.filter(p => p.in === 'query');
          if (queryParams.length > 0) {
            requestItem.request.url.query = queryParams.map(p => ({
              key: p.name,
              value: '',
              description: p.description || '',
              disabled: !p.required
            }));
          }
          
          // Header parameters
          const headerParams = operation.parameters.filter(p => p.in === 'header');
          if (headerParams.length > 0) {
            requestItem.request.header = headerParams.map(p => ({
              key: p.name,
              value: '',
              description: p.description || '',
              disabled: !p.required
            }));
          }
        }
        
        // Add request body if present
        if (operation.requestBody) {
          const content = operation.requestBody.content || {};
          const jsonContent = content['application/json'];
          
          if (jsonContent) {
            requestItem.request.body = {
              mode: 'raw',
              raw: JSON.stringify(jsonContent.example || {}, null, 2),
              options: {
                raw: {
                  language: 'json'
                }
              }
            };
          }
        }
        
        tagGroups[tag].push(requestItem);
      }
    }
  }
  
  // Create folders for each tag
  for (const tag in tagGroups) {
    collection.item.push({
      name: tag,
      item: tagGroups[tag]
    });
  }
  
  return collection;
}

// Convert the OpenAPI spec to a Postman collection
const postmanCollection = createPostmanCollection(fixedSpec);

// Write the Postman collection to the output file
fs.writeFileSync(outputFile, JSON.stringify(postmanCollection, null, 2));
console.log(`Successfully converted ${inputFile} to ${outputFile}`);
process.exit(0);
