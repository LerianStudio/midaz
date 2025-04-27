#!/usr/bin/env node

/**
 * Enhanced script to convert OpenAPI specification to Postman Collection
 * 
 * Improvements:
 * - Adds pre-request scripts for handling dependencies and authentication
 * - Creates test scripts for validating responses
 * - Extracts variables from responses for use in subsequent requests
 * - Handles request and response examples better
 * - Creates proper environment variable templates
 * 
 * Usage: node convert-openapi.js <input-file> <output-file> [--env <env-output-file>]
 */

const fs = require('fs');
const path = require('path');

// Check arguments
const args = process.argv.slice(2);
if (args.length < 2) {
  console.error('Usage: node convert-openapi.js <input-file> <output-file> [--env <env-output-file>]');
  process.exit(1);
}

const inputFile = args[0];
const outputFile = args[1];

// Check for environment output file
let envOutputFile = null;
if (args.includes('--env') && args.indexOf('--env') + 1 < args.length) {
  envOutputFile = args[args.indexOf('--env') + 1];
}

// Check if input file exists
if (!fs.existsSync(inputFile)) {
  console.error(`Input file not found: ${inputFile}`);
  process.exit(1);
}

// Create necessary directories
const outputDir = path.dirname(outputFile);
if (!fs.existsSync(outputDir)) {
  fs.mkdirSync(outputDir, { recursive: true });
}

if (envOutputFile) {
  const envDir = path.dirname(envOutputFile);
  if (!fs.existsSync(envDir)) {
    fs.mkdirSync(envDir, { recursive: true });
  }
}

// Read and parse the input file
const openApiSpec = JSON.parse(fs.readFileSync(inputFile, 'utf8'));

// Dependency map for API endpoints
const dependencyMap = {
  // Organization endpoints
  "POST /v1/organizations": {
    provides: ["organizationId"],
    requires: []
  },
  "GET /v1/organizations/{id}": {
    provides: [],
    requires: ["organizationId"]
  },
  
  // Ledger endpoints
  "POST /v1/organizations/{organization_id}/ledgers": {
    provides: ["ledgerId"],
    requires: ["organizationId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId"]
  },
  
  // Asset endpoints
  "POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets": {
    provides: ["assetId"],
    requires: ["organizationId", "ledgerId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "assetId"]
  },
  
  // Account endpoints
  "POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts": {
    provides: ["accountId"],
    requires: ["organizationId", "ledgerId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "accountId"]
  },
  
  // Transaction endpoints
  "POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions": {
    provides: ["transactionId"],
    requires: ["organizationId", "ledgerId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "transactionId"]
  },
  
  // Operation endpoints
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/operations/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "operationId"]
  },
  
  // Balance endpoints
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances": {
    provides: [],
    requires: ["organizationId", "ledgerId", "accountId"]
  }
};

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

// Generate a pre-request script based on the endpoint dependencies
function generatePreRequestScript(operation, path, method) {
  let script = '';
  
  // Get the endpoint key for the dependency map
  const endpointKey = `${method.toUpperCase()} ${path}`;
  const dependencies = dependencyMap[endpointKey] || { requires: [], provides: [] };
  
  // Add authentication handling
  script += `
// Check for auth token
if (!pm.environment.get("authToken")) {
  console.log("Warning: authToken is not set in the environment");
}

// Set authorization header if it exists
if (pm.environment.get("authToken")) {
  pm.request.headers.upsert({
    key: "Authorization",
    value: "Bearer " + pm.environment.get("authToken")
  });
}

// Set request ID for tracing
pm.request.headers.upsert({
  key: "X-Request-Id",
  value: pm.variables.replaceIn("{{$guid}}")
});
`;

  // Add validation for required variables
  if (dependencies.requires && dependencies.requires.length > 0) {
    script += `
// Validate required variables
`;
    
    dependencies.requires.forEach(variable => {
      script += `
if (!pm.environment.get("${variable}")) {
  console.log("Warning: ${variable} is not set. This request may fail.");
}`;
    });
  }
  
  return script;
}

// Generate a test script based on the endpoint and its expected responses
function generateTestScript(operation, path, method) {
  let script = '';
  
  // Get the endpoint key for the dependency map
  const endpointKey = `${method.toUpperCase()} ${path}`;
  const dependencies = dependencyMap[endpointKey] || { requires: [], provides: [] };
  
  // Basic status code validation
  script += `
// Test for successful response status
pm.test("Status code is successful", function () {
  pm.response.to.be.oneOf([200, 201, 202, 204]);
});

// Validate response has the expected format
pm.test("Response has the correct structure", function() {
  pm.response.to.be.json;
  
  // Add specific validation based on response schema here
});
`;

  // Extract variables from response if needed
  if (dependencies.provides && dependencies.provides.length > 0) {
    script += `
// Extract variables from response for use in subsequent requests
`;
    
    dependencies.provides.forEach(variable => {
      // Handle different variable extraction patterns based on endpoint type
      if (variable === 'organizationId' && endpointKey === 'POST /v1/organizations') {
        script += `
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("${variable}", jsonData.id);
    console.log("${variable} set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract ${variable}: ", error);
}`;
      } else if (variable === 'ledgerId' && endpointKey.includes('/ledgers')) {
        script += `
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("${variable}", jsonData.id);
    console.log("${variable} set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract ${variable}: ", error);
}`;
      } else if (variable === 'assetId' && endpointKey.includes('/assets')) {
        script += `
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("${variable}", jsonData.id);
    console.log("${variable} set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract ${variable}: ", error);
}`;
      } else if (variable === 'accountId' && endpointKey.includes('/accounts')) {
        script += `
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("${variable}", jsonData.id);
    console.log("${variable} set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract ${variable}: ", error);
}`;
      } else if (variable === 'transactionId' && endpointKey.includes('/transactions')) {
        script += `
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("${variable}", jsonData.id);
    console.log("${variable} set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract ${variable}: ", error);
}`;
      }
    });
  }
  
  return script;
}

// Generate an example based on the schema definition
function generateExampleFromSchema(schema) {
  if (!schema) return {};
  
  // If there's already an example, use it
  if (schema.example) return schema.example;
  
  // If there's a provided example in the specification, use it
  if (schema.examples && schema.examples.length > 0) {
    return schema.examples[0].value;
  }
  
  // Handle different types
  if (schema.type === 'object') {
    const example = {};
    if (schema.properties) {
      for (const prop in schema.properties) {
        example[prop] = generateExampleFromSchema(schema.properties[prop]);
      }
    }
    return example;
  } else if (schema.type === 'array') {
    if (schema.items) {
      return [generateExampleFromSchema(schema.items)];
    }
    return [];
  } else if (schema.type === 'string') {
    if (schema.format === 'uuid') return '00000000-0000-0000-0000-000000000000';
    if (schema.format === 'date-time') return new Date().toISOString();
    if (schema.format === 'date') return new Date().toISOString().split('T')[0];
    if (schema.enum) return schema.enum[0];
    return 'string';
  } else if (schema.type === 'number' || schema.type === 'integer') {
    return 0;
  } else if (schema.type === 'boolean') {
    return false;
  } else {
    return null;
  }
}

// Create a Postman collection from the OpenAPI spec
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
    variable: [
      {
        key: "baseUrl",
        value: spec.servers && spec.servers.length > 0 ? spec.servers[0].url : "http://localhost:3000",
        type: "string"
      }
    ],
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
              path: path.split('/').filter(p => p).map(p => {
                // Handle path parameters to use environment variables
                if (p.startsWith('{') && p.endsWith('}')) {
                  const paramName = p.slice(1, -1);
                  if (paramName === 'organization_id') return '{{organizationId}}';
                  if (paramName === 'ledger_id') return '{{ledgerId}}';
                  if (paramName === 'account_id') return '{{accountId}}';
                  if (paramName === 'asset_id') return '{{assetId}}';
                  if (paramName === 'id') {
                    // Try to determine the entity type from the path
                    if (path.includes('/organizations/') && !path.includes('/ledgers/')) return '{{organizationId}}';
                    if (path.includes('/ledgers/') && !path.includes('/accounts/') && !path.includes('/assets/')) return '{{ledgerId}}';
                    if (path.includes('/accounts/') && !path.includes('/balances/')) return '{{accountId}}';
                    if (path.includes('/assets/')) return '{{assetId}}';
                  }
                  return `{{${paramName}}}`;
                }
                return p;
              })
            },
            description: operation.description || ''
          },
          response: []
        };
        
        // Add pre-request and test scripts
        requestItem.event = [
          {
            listen: "prerequest",
            script: {
              type: "text/javascript",
              exec: generatePreRequestScript(operation, path, method).split('\n')
            }
          },
          {
            listen: "test",
            script: {
              type: "text/javascript",
              exec: generateTestScript(operation, path, method).split('\n')
            }
          }
        ];
        
        // Add parameters
        if (operation.parameters) {
          // Path parameters
          const pathParams = operation.parameters.filter(p => p.in === 'path');
          if (pathParams.length > 0) {
            requestItem.request.url.variable = pathParams.map(p => {
              // Map common parameter names to environment variables
              let value = '';
              if (p.name === 'organization_id') value = '{{organizationId}}';
              else if (p.name === 'ledger_id') value = '{{ledgerId}}';
              else if (p.name === 'account_id') value = '{{accountId}}';
              else if (p.name === 'asset_id') value = '{{assetId}}';
              else if (p.name === 'id') {
                // Try to determine the entity type from the path
                if (path.includes('/organizations/') && !path.includes('/ledgers/')) value = '{{organizationId}}';
                if (path.includes('/ledgers/') && !path.includes('/accounts/') && !path.includes('/assets/')) value = '{{ledgerId}}';
                if (path.includes('/accounts/') && !path.includes('/balances/')) value = '{{accountId}}';
                if (path.includes('/assets/')) value = '{{assetId}}';
              }
              
              return {
                key: p.name,
                value: value,
                description: p.description || ''
              };
            });
          }
          
          // Query parameters
          const queryParams = operation.parameters.filter(p => p.in === 'query');
          if (queryParams.length > 0) {
            requestItem.request.url.query = queryParams.map(p => ({
              key: p.name,
              value: p.schema?.default !== undefined ? String(p.schema.default) : '',
              description: p.description || '',
              disabled: !p.required
            }));
          }
          
          // Header parameters
          const headerParams = operation.parameters.filter(p => p.in === 'header');
          if (headerParams.length > 0) {
            requestItem.request.header = headerParams.map(p => {
              // Handle auth headers specially
              let value = '';
              if (p.name === 'Authorization') {
                value = '{{authToken}}';
                if (value.indexOf('Bearer') === -1) {
                  value = 'Bearer ' + value;
                }
              } else if (p.name === 'X-Request-Id') {
                value = '{{$guid}}';
              }
              
              return {
                key: p.name,
                value: value,
                description: p.description || '',
                disabled: !p.required
              };
            });
          }
        }
        
        // Add request body if present
        if (operation.requestBody) {
          const content = operation.requestBody.content || {};
          const jsonContent = content['application/json'];
          
          if (jsonContent) {
            let example = {};
            
            // Try to get example from the schema
            if (jsonContent.schema) {
              example = generateExampleFromSchema(jsonContent.schema);
            }
            
            // If there are explicit examples, use the first one
            if (jsonContent.examples && Object.keys(jsonContent.examples).length > 0) {
              const firstExampleKey = Object.keys(jsonContent.examples)[0];
              example = jsonContent.examples[firstExampleKey].value;
            }
            
            requestItem.request.body = {
              mode: 'raw',
              raw: JSON.stringify(example, null, 2),
              options: {
                raw: {
                  language: 'json'
                }
              }
            };
          }
        }
        
        // Add response examples
        if (operation.responses) {
          for (const statusCode in operation.responses) {
            const response = operation.responses[statusCode];
            const content = response.content || {};
            const jsonContent = content['application/json'];
            
            if (jsonContent) {
              let example = {};
              
              // Try to get example from the schema
              if (jsonContent.schema) {
                example = generateExampleFromSchema(jsonContent.schema);
              }
              
              // If there are explicit examples, use the first one
              if (jsonContent.examples && Object.keys(jsonContent.examples).length > 0) {
                const firstExampleKey = Object.keys(jsonContent.examples)[0];
                example = jsonContent.examples[firstExampleKey].value;
              }
              
              requestItem.response.push({
                name: `${statusCode} - ${response.description || 'Response'}`,
                originalRequest: {
                  method: requestItem.request.method,
                  header: requestItem.request.header,
                  url: requestItem.request.url,
                  body: requestItem.request.body
                },
                status: statusCode,
                code: parseInt(statusCode, 10),
                _postman_previewlanguage: "json",
                header: [
                  {
                    key: "Content-Type",
                    value: "application/json"
                  }
                ],
                cookie: [],
                body: JSON.stringify(example, null, 2)
              });
            }
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

// Create an environment template based on the API and dependency map
function createEnvironmentTemplate(spec) {
  const info = spec.info || {};
  
  // Get base URL from servers if available
  let baseUrl = 'http://localhost:3000';
  if (spec.servers && spec.servers.length > 0) {
    baseUrl = spec.servers[0].url;
    // Make sure it's a valid URL
    if (baseUrl.startsWith('//')) {
      baseUrl = 'http:' + baseUrl;
    }
  }
  
  // Create environment template
  const environment = {
    id: "midaz-environment",
    name: "Midaz Development Environment",
    values: [
      {
        key: "baseUrl",
        value: baseUrl,
        type: "default",
        enabled: true
      },
      {
        key: "authToken",
        value: "",
        type: "secret",
        enabled: true
      }
    ]
  };
  
  // Add common variables from dependency map
  const allVariables = new Set();
  for (const endpoint in dependencyMap) {
    const dependencies = dependencyMap[endpoint];
    
    if (dependencies.provides) {
      dependencies.provides.forEach(variable => allVariables.add(variable));
    }
    
    if (dependencies.requires) {
      dependencies.requires.forEach(variable => allVariables.add(variable));
    }
  }
  
  // Add all unique variables to the environment
  allVariables.forEach(variable => {
    environment.values.push({
      key: variable,
      value: "",
      type: "default",
      enabled: true
    });
  });
  
  return environment;
}

// Fix the OpenAPI spec
const fixedSpec = fixPathParameters(openApiSpec);

// Convert the OpenAPI spec to a Postman collection
const postmanCollection = createPostmanCollection(fixedSpec);

// Write the Postman collection to the output file
fs.writeFileSync(outputFile, JSON.stringify(postmanCollection, null, 2));
console.log(`Successfully converted ${inputFile} to ${outputFile}`);

// Create and write the environment template if requested
if (envOutputFile) {
  const environmentTemplate = createEnvironmentTemplate(fixedSpec);
  fs.writeFileSync(envOutputFile, JSON.stringify(environmentTemplate, null, 2));
  console.log(`Created environment template at ${envOutputFile}`);
}

process.exit(0);