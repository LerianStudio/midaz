#!/usr/bin/env node

/**
 * OpenAPI to Postman Converter
 * 
 * This script provides a comprehensive tool that:
 * 1. Enhances OpenAPI specs with better examples and descriptions
 * 2. Converts the enhanced specs to Postman collections
 * 
 * Usage: node convert-openapi.js <input-file> <output-file> [--env <env-output-file>]
 */

const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');

// Check arguments
const args = process.argv.slice(2);
if (args.length < 2) {
  console.error('Usage: node enhanced-convert-openapi.js <input-file> <output-file> [--env <env-output-file>]');
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
let openApiSpec;
try {
  const fileContent = fs.readFileSync(inputFile, 'utf8');
  if (inputFile.endsWith('.json')) {
    openApiSpec = JSON.parse(fileContent);
  } else if (inputFile.endsWith('.yaml') || inputFile.endsWith('.yml')) {
    openApiSpec = yaml.load(fileContent);
  } else {
    console.error('Input file must be JSON or YAML format');
    process.exit(1);
  }
} catch (error) {
  console.error(`Error reading/parsing input file: ${error.message}`);
  process.exit(1);
}

//=============================================================================
// PART 1: ENHANCE OPENAPI SPECIFICATION
//=============================================================================

console.log('Enhancing OpenAPI specification...');

// Enhanced pagination schema
const enhancedPaginationSchema = {
  description: `Pagination is a standardized structure used across all list endpoints to control and navigate 
through paginated results. It provides information about the current page, result limitations, 
and navigation cursors for moving through large result sets.

This structure supports both cursor-based and offset-based pagination:
- For cursor-based pagination, use the 'prev_cursor' and 'next_cursor' fields
- For offset-based pagination, use the 'page' and 'limit' fields`,
  properties: {
    limit: {
      description: `Maximum number of items to return per page. This is enforced by the server 
and may be lower than requested if it exceeds the server-defined maximum.
Default is 10, maximum allowed is defined by MAX_PAGINATION_LIMIT (typically 100).`,
      example: 10,
      type: "integer",
      minimum: 1,
      maximum: 100
    },
    page: {
      description: `Current page number in offset-based pagination. Pages are 1-indexed.
Use with 'limit' to navigate through large result sets when order is not critical.`,
      example: 1,
      type: "integer",
      minimum: 1
    },
    next_cursor: {
      description: `Opaque cursor value for retrieving the next page of results. 
Pass this value to the same endpoint to get the next set of items.
Will be null or empty when there are no more pages after the current one.`,
      example: "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA==",
      type: "string",
      "x-omitempty": true
    },
    prev_cursor: {
      description: `Opaque cursor value for retrieving the previous page of results.
Pass this value to the same endpoint to get the previous set of items.
Will be null or empty when currently on the first page.`,
      example: "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA==",
      type: "string",
      "x-omitempty": true
    },
    total_items: {
      description: `Total number of items across all pages that match the current query.
May be omitted for queries that would be too expensive to calculate the total.`,
      type: "integer",
      "x-omitempty": true
    },
    total_pages: {
      description: `Total number of pages available based on the current limit.
May be omitted for queries that would be too expensive to calculate the total.`,
      type: "integer",
      "x-omitempty": true
    }
  },
  type: "object",
  example: {
    prev_cursor: null,
    next_cursor: "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA==",
    limit: 10,
    page: 1,
    total_items: 42,
    total_pages: 5
  }
};

// Enhanced error schema
const enhancedErrorSchema = {
  description: `Standardized error response format used across all API endpoints for error situations.
Provides structured information about errors including code, message, details, and 
field-specific validation issues.`,
  required: [
    "code",
    "message"
  ],
  properties: {
    code: {
      description: `Error code identifying the specific error condition.
Uses a standardized prefix pattern: ERR_[CATEGORY]_[SPECIFIC]`,
      example: "ERR_INVALID_INPUT",
      type: "string",
      maxLength: 50
    },
    message: {
      description: `Human-readable error message explaining what went wrong.
This message is suitable for displaying to end users.`,
      example: "The provided input data is invalid.",
      type: "string"
    },
    details: {
      description: `Additional error details that might help developers debug the issue.
Not intended for end-user display.`,
      example: "Validation failed for the following fields: legalName, legalDocument",
      type: "string",
      "x-omitempty": true
    },
    entityType: {
      description: `Type of entity associated with the error, if applicable.
Helps identify which resource type had an issue.`,
      example: "Organization",
      type: "string",
      maxLength: 100,
      "x-omitempty": true
    },
    entityId: {
      description: `ID of the entity associated with the error, if applicable.
Helps identify which specific resource instance had an issue.`,
      example: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
      type: "string",
      format: "uuid",
      "x-omitempty": true
    },
    fields: {
      description: `Map of field names to specific validation errors.
Provided when the error relates to input validation issues.`,
      type: "object",
      additionalProperties: {
        type: "string"
      },
      example: {
        legalName: "Legal name is required and must be between 3 and 150 characters",
        legalDocument: "Legal document must be a valid identification number"
      },
      "x-omitempty": true
    },
    requestId: {
      description: `Unique identifier for this request, useful for tracking and debugging.
This ID can be provided to support for troubleshooting.`,
      example: "req_abc123def456",
      type: "string",
      "x-omitempty": true
    },
    timestamp: {
      description: `ISO8601 timestamp indicating when the error occurred.`,
      example: "2023-04-01T12:34:56Z",
      type: "string",
      format: "date-time"
    }
  },
  example: {
    code: "ERR_INVALID_INPUT",
    message: "The provided input data is invalid.",
    details: "Validation failed for the following fields: legalName, legalDocument",
    entityType: "Organization",
    fields: {
      legalName: "Legal name is required and must be between 3 and 150 characters",
      legalDocument: "Legal document must be a valid identification number"
    },
    requestId: "req_abc123def456",
    timestamp: "2023-04-01T12:34:56Z"
  }
};

// Standard error responses
const standardErrorResponses = {
  BadRequest: {
    description: `Bad Request - The request was invalid or cannot be otherwise served.
This typically occurs due to missing required parameters, invalid parameter values,
or malformed request syntax.`,
    content: {
      "application/json": {
        schema: {
          $ref: "#/components/schemas/Error"
        },
        examples: {
          validation_error: {
            summary: "Validation Error",
            value: {
              code: "ERR_VALIDATION_FAILED",
              message: "Request validation failed",
              details: "One or more fields failed validation",
              fields: {
                legalName: "Must be between 3 and 150 characters",
                legalDocument: "Must be a valid document format"
              },
              requestId: "req_abc123def456",
              timestamp: "2023-04-01T12:34:56Z"
            }
          },
          missing_field: {
            summary: "Missing Required Field",
            value: {
              code: "ERR_MISSING_REQUIRED_FIELD",
              message: "Missing required field",
              details: "The request is missing one or more required fields",
              fields: {
                legalName: "This field is required"
              },
              requestId: "req_abc123def456",
              timestamp: "2023-04-01T12:34:56Z"
            }
          }
        }
      }
    }
  },
  Unauthorized: {
    description: `Unauthorized - Authentication is required and has failed or has not been provided.
This typically occurs when no authentication token is provided, or the provided token is invalid,
expired, or revoked.`,
    content: {
      "application/json": {
        schema: {
          $ref: "#/components/schemas/Error"
        },
        examples: {
          no_token: {
            summary: "No Authentication Token",
            value: {
              code: "ERR_NO_AUTH_TOKEN",
              message: "Authentication required",
              details: "No authentication token was provided in the request",
              requestId: "req_abc123def456",
              timestamp: "2023-04-01T12:34:56Z"
            }
          },
          invalid_token: {
            summary: "Invalid Authentication Token",
            value: {
              code: "ERR_INVALID_AUTH_TOKEN",
              message: "Invalid authentication token",
              details: "The provided authentication token is invalid or expired",
              requestId: "req_abc123def456",
              timestamp: "2023-04-01T12:34:56Z"
            }
          }
        }
      }
    }
  },
  Forbidden: {
    description: `Forbidden - The server understood the request but refuses to authorize it.
This typically occurs when the authenticated user does not have sufficient permissions
to perform the requested operation on the specified resource.`,
    content: {
      "application/json": {
        schema: {
          $ref: "#/components/schemas/Error"
        },
        examples: {
          insufficient_permissions: {
            summary: "Insufficient Permissions",
            value: {
              code: "ERR_INSUFFICIENT_PERMISSIONS",
              message: "Insufficient permissions",
              details: "You do not have the required permissions to perform this operation",
              entityType: "Organization",
              entityId: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
              requestId: "req_abc123def456",
              timestamp: "2023-04-01T12:34:56Z"
            }
          }
        }
      }
    }
  },
  NotFound: {
    description: `Not Found - The requested resource could not be found.
This typically occurs when the resource identified by the request URL does not exist,
or if the resource has been deleted or is otherwise unavailable.`,
    content: {
      "application/json": {
        schema: {
          $ref: "#/components/schemas/Error"
        },
        examples: {
          resource_not_found: {
            summary: "Resource Not Found",
            value: {
              code: "ERR_RESOURCE_NOT_FOUND",
              message: "Resource not found",
              details: "The requested resource does not exist or has been deleted",
              entityType: "Organization",
              entityId: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
              requestId: "req_abc123def456",
              timestamp: "2023-04-01T12:34:56Z"
            }
          }
        }
      }
    }
  },
  InternalServerError: {
    description: `Internal Server Error - An unexpected error occurred on the server.
This is a generic server-side error that typically indicates a problem with the server's
internal systems rather than with the client's request.`,
    content: {
      "application/json": {
        schema: {
          $ref: "#/components/schemas/Error"
        },
        examples: {
          internal_error: {
            summary: "Internal Server Error",
            value: {
              code: "ERR_INTERNAL_SERVER_ERROR",
              message: "An unexpected error occurred",
              details: "The server encountered an unexpected condition that prevented it from fulfilling the request",
              requestId: "req_abc123def456",
              timestamp: "2023-04-01T12:34:56Z"
            }
          }
        }
      }
    }
  }
};

// Request body examples can now come from the OpenAPI spec directly
const requestBodyExamples = {};

// Update the OpenAPI spec with enhanced components
if (!openApiSpec.components) {
  openApiSpec.components = {};
}
if (!openApiSpec.components.schemas) {
  openApiSpec.components.schemas = {};
}
if (!openApiSpec.components.responses) {
  openApiSpec.components.responses = {};
}

// Update pagination schema if it exists
if (openApiSpec.components.schemas.Pagination) {
  console.log('Enhancing Pagination schema...');
  openApiSpec.components.schemas.Pagination = enhancedPaginationSchema;
}

// Update error schema if it exists
if (openApiSpec.components.schemas.Error) {
  console.log('Enhancing Error schema...');
  openApiSpec.components.schemas.Error = enhancedErrorSchema;
}

// Add standard error responses
console.log('Adding standard error responses...');
for (const [key, value] of Object.entries(standardErrorResponses)) {
  openApiSpec.components.responses[key] = value;
}

// Add request body examples
console.log('Adding request body examples to endpoints...');
if (openApiSpec.paths) {
  for (const [pathKey, pathItem] of Object.entries(openApiSpec.paths)) {
    for (const [methodKey, operation] of Object.entries(pathItem)) {
      if (operation.requestBody && operation.requestBody.content && operation.requestBody.content['application/json']) {
        const contentJson = operation.requestBody.content['application/json'];
        if (contentJson.schema && contentJson.schema.$ref) {
          // Extract the schema name from the reference
          const schemaName = contentJson.schema.$ref.split('/').pop();
          
          // Check if we have examples for this schema
          if (requestBodyExamples[schemaName]) {
            console.log(`Adding examples to ${methodKey.toUpperCase()} ${pathKey} (${schemaName})...`);
            contentJson.examples = requestBodyExamples[schemaName];
          }
        }
      }
    }
  }
}

// Update endpoints to use standard responses
console.log('Updating endpoints to use standard error responses...');
if (openApiSpec.paths) {
  for (const [pathKey, pathItem] of Object.entries(openApiSpec.paths)) {
    for (const [methodKey, operation] of Object.entries(pathItem)) {
      if (operation.responses) {
        // Replace common error responses with references
        if (operation.responses['400']) {
          operation.responses['400'] = { '$ref': '#/components/responses/BadRequest' };
        }
        if (operation.responses['401']) {
          operation.responses['401'] = { '$ref': '#/components/responses/Unauthorized' };
        }
        if (operation.responses['403']) {
          operation.responses['403'] = { '$ref': '#/components/responses/Forbidden' };
        }
        if (operation.responses['404']) {
          operation.responses['404'] = { '$ref': '#/components/responses/NotFound' };
        }
        if (operation.responses['500']) {
          operation.responses['500'] = { '$ref': '#/components/responses/InternalServerError' };
        }
      }
    }
  }
}

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
const enhancedSpec = fixPathParameters(openApiSpec);

//=============================================================================
// PART 2: CONVERT TO POSTMAN COLLECTION
//=============================================================================

console.log('Converting enhanced OpenAPI spec to Postman collection...');

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
if (pm.request.method === "POST") {
  pm.test("Status code is 200 or 201", function () {
    pm.expect(pm.response.code).to.be.oneOf([200, 201]);
  });
} else if (pm.request.method === "DELETE") {
  pm.test("Status code is 204 No Content", function () {
    pm.expect(pm.response.code).to.equal(204);
  });
} else {
  pm.test("Status code is 200 OK", function () {
    pm.expect(pm.response.code).to.equal(200);
  });
}

// Validate response has the expected format
pm.test("Response has the correct structure", function() {
  // For DELETE operations that return 204 No Content, the body is empty by design
  if (pm.response.code === 204) {
    pm.expect(true).to.be.true; // Always pass for 204 responses
    return;
  }
  
  // For responses with content, validate JSON structure
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
        value: spec.servers && spec.servers.length > 0 ? spec.servers[0].url : "http://localhost",
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
        
        // Determine which service this endpoint belongs to
        let baseUrlVariable = "{{baseUrl}}";
        if (path.includes('/transactions') || path.includes('/operations') || path.includes('/balances') || 
            path.includes('/assetrates') || path.includes('/balance')) {
          baseUrlVariable = "{{transactionUrl}}";
        } else {
          baseUrlVariable = "{{onboardingUrl}}";
        }
        
        // Create request item
        const requestItem = {
          name: operation.summary || `${method.toUpperCase()} ${path}`,
          request: {
            method: method.toUpperCase(),
            header: [],
            url: {
              raw: `${baseUrlVariable}${path}`,
              host: [`${baseUrlVariable}`],
              path: path.split('/').filter(p => p).map(p => {
                // Handle path parameters to use environment variables (camelCase)
                if (p.startsWith('{') && p.endsWith('}')) {
                  const paramName = p.slice(1, -1);
                  if (paramName === 'organization_id') return '{{organizationId}}';
                  if (paramName === 'ledger_id') return '{{ledgerId}}';
                  if (paramName === 'account_id') return '{{accountId}}';
                  if (paramName === 'asset_id') return '{{assetId}}';
                  if (paramName === 'transaction_id') return '{{transactionId}}';
                  if (paramName === 'operation_id') return '{{operationId}}';
                  if (paramName === 'balance_id') return '{{balanceId}}';
                  if (paramName === 'id') {
                    // Try to determine the entity type from the path
                    if (path.includes('/organizations/') && !path.includes('/ledgers/')) return '{{organizationId}}';
                    if (path.includes('/ledgers/') && !path.includes('/accounts/') && !path.includes('/assets/')) return '{{ledgerId}}';
                    if (path.includes('/accounts/') && !path.includes('/balances/')) return '{{accountId}}';
                    if (path.includes('/assets/')) return '{{assetId}}';
                    if (path.includes('/portfolios/')) return '{{portfolioId}}';
                    if (path.includes('/segments/')) return '{{segmentId}}';
                    if (path.includes('/operations/')) return '{{operationId}}';
                    if (path.includes('/transactions/')) return '{{transactionId}}';
                    if (path.includes('/balances/')) return '{{balanceId}}';
                  }
                  // Convert snake_case to camelCase for other parameters
                  if (paramName.includes('_')) {
                    const camelCaseParam = paramName.replace(/_([a-z])/g, (match, p1) => p1.toUpperCase());
                    return `{{${camelCaseParam}}}`;
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
              // Map common parameter names to environment variables using camelCase
              let value = '';
              if (p.name === 'organization_id') value = '{{organizationId}}';
              else if (p.name === 'ledger_id') value = '{{ledgerId}}';
              else if (p.name === 'account_id') value = '{{accountId}}';
              else if (p.name === 'asset_id') value = '{{assetId}}';
              else if (p.name === 'transaction_id') value = '{{transactionId}}';
              else if (p.name === 'operation_id') value = '{{operationId}}';
              else if (p.name === 'balance_id') value = '{{balanceId}}';
              else if (p.name === 'id') {
                // Try to determine the entity type from the path
                if (path.includes('/organizations/') && !path.includes('/ledgers/')) value = '{{organizationId}}';
                if (path.includes('/ledgers/') && !path.includes('/accounts/') && !path.includes('/assets/')) value = '{{ledgerId}}';
                if (path.includes('/accounts/') && !path.includes('/balances/')) value = '{{accountId}}';
                if (path.includes('/assets/')) value = '{{assetId}}';
                if (path.includes('/portfolios/')) value = '{{portfolioId}}';
                if (path.includes('/segments/')) value = '{{segmentId}}';
                if (path.includes('/operations/')) value = '{{operationId}}';
                if (path.includes('/transactions/')) value = '{{transactionId}}';
                if (path.includes('/balances/')) value = '{{balanceId}}';
              }
              // Convert any other snake_case params to camelCase
              else if (p.name.includes('_')) {
                const camelCaseParam = p.name.replace(/_([a-z])/g, (match, p1) => p1.toUpperCase());
                value = `{{${camelCaseParam}}}`;
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
        
        // Add request body if present in OpenAPI 3.0 format
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
        // Handle body parameter in Swagger 2.0 format
        else if (operation.parameters) {
          const bodyParam = operation.parameters.find(p => p.in === 'body');
          if (bodyParam && bodyParam.schema) {
            let example = {};
            
            // Try to extract example from schema reference
            if (bodyParam.schema.$ref) {
              const schemaName = bodyParam.schema.$ref.split('/').pop();
              
              // Look for the schema definition in the spec
              if (spec.definitions && spec.definitions[schemaName]) {
                const schema = spec.definitions[schemaName];
                
                // Build example from schema properties
                if (schema.properties) {
                  example = {};
                  Object.keys(schema.properties).forEach(propName => {
                    const prop = schema.properties[propName];
                    
                    // Check if there's an explicit example for this property
                    if (prop.example !== undefined) {
                      // Skip parentOrganizationId from examples to avoid foreign key errors
                      if (propName !== 'parentOrganizationId') {
                        example[propName] = prop.example;
                      }
                    } 
                    // For refs, try to resolve them as well
                    else if (prop.allOf && prop.allOf[0] && prop.allOf[0].$ref) {
                      const refName = prop.allOf[0].$ref.split('/').pop();
                      if (spec.definitions && spec.definitions[refName]) {
                        const refSchema = spec.definitions[refName];
                        
                        // Create nested object examples
                        if (refSchema.properties) {
                          example[propName] = {};
                          Object.keys(refSchema.properties).forEach(refProp => {
                            if (refSchema.properties[refProp].example !== undefined) {
                              example[propName][refProp] = refSchema.properties[refProp].example;
                            }
                          });
                        }
                      }
                    }
                    // Handle metadata with a realistic example
                    else if (propName === 'metadata' && prop.additionalProperties !== undefined) {
                      example[propName] = {
                        "department": "finance",
                        "region": "north_america",
                        "type": "corporate"
                      };
                    }
                    // For status, construct based on common values
                    else if (propName === 'status' && !prop.example) {
                      example[propName] = {
                        "code": "ACTIVE",
                        "description": "Active status"
                      };
                    }
                    // Default values for common properties
                    else if (!prop.example) {
                      // Skip parentOrganizationId from examples to avoid foreign key errors
                      if (propName === 'parentOrganizationId') {
                        // Skip adding this field
                      }
                      else if (prop.type === 'string') {
                        if (prop.format === 'uuid') example[propName] = '00000000-0000-0000-0000-000000000000';
                        else example[propName] = `Example ${propName}`;
                      }
                      else if (prop.type === 'number' || prop.type === 'integer') example[propName] = 123;
                      else if (prop.type === 'boolean') example[propName] = true;
                      else if (prop.type === 'array') example[propName] = [];
                      else if (prop.type === 'object') example[propName] = {};
                    }
                  });
                }
              }
            }
            
            // Only special handling for DSL endpoint
            if (path.includes('/transactions')) {
              if (method === 'post') {
                // Check if this is the DSL endpoint
                if (path.includes('/dsl') || path.endsWith('/dsl')) {
                  console.log('DSL endpoint detected, adding text body example');
                  
                  // Create a transaction DSL example
                  const dslExample = `// Transaction DSL Example
// This is a simple transfer between two accounts

transaction {
  description "Fund transfer between accounts"
  reference "TRANSFER-REF-001"
  
  // Account debited $100
  debit {
    account "{{accountId}}"
    amount 100.00
    asset "USD"
  }
  
  // Account credited $100
  credit {
    account "00000000-0000-0000-0000-000000000002"
    amount 100.00
    asset "USD"
  }
}`;

                  // For DSL endpoint, set body directly
                  requestItem.request.body = {
                    mode: 'raw',
                    raw: dslExample,
                    options: {
                      raw: {
                        language: 'text'
                      }
                    }
                  };
                  
                  // Skip the normal JSON body creation for this endpoint
                  return;
                }
              }
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
            // Skip references
            if (response.$ref) continue;
            
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
  // Create environment template
  const environment = {
    id: "midaz-environment-id",
    name: "MIDAZ Environment",
    values: [
      {
        key: "baseUrl",
        value: "http://localhost",
        type: "default",
        enabled: true
      },
      {
        key: "onboardingPort",
        value: "3000",
        type: "default",
        enabled: true
      },
      {
        key: "transactionPort",
        value: "3001",
        type: "default",
        enabled: true
      },
      {
        key: "onboardingUrl",
        value: "{{baseUrl}}:{{onboardingPort}}",
        type: "default",
        enabled: true
      },
      {
        key: "transactionUrl",
        value: "{{baseUrl}}:{{transactionPort}}",
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
  
  // Additional variables not covered by the dependency map
  const additionalVariables = [
    "portfolioId", 
    "segmentId", 
    "assetRateId", 
    "balanceId"
  ];
  
  // Add additional variables
  additionalVariables.forEach(variable => allVariables.add(variable));
  
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

// Convert the OpenAPI spec to a Postman collection
const postmanCollection = createPostmanCollection(enhancedSpec);

// Create E2E workflow folder
function createE2EWorkflow(collection) {
  // Define the workflow sequence
  const workflowSequence = [
    // Organization flow
    { operation: "GET", path: "/v1/organizations", name: "1. List Organizations" },
    { operation: "POST", path: "/v1/organizations", name: "2. Create Organization" },
    { operation: "GET", path: "/v1/organizations/{id}", name: "3. Get Organization" },
    { operation: "PATCH", path: "/v1/organizations/{id}", name: "4. Update Organization" },
    
    // Ledger flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers", name: "5. List Ledgers" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers", name: "6. Create Ledger" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "7. Get Ledger" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "8. Update Ledger" },
    
    // Asset flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets", name: "9. List Assets" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets", name: "10. Create USD Asset" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "11. Get Asset" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "12. Update Asset" },
    
    // Asset Rate flow
    { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates", name: "13. Create AssetRate" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code}", name: "14. Get AssetRate by Code" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id}", name: "15. Get AssetRate by ExternalID" },
    
    // Account flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "16. List Accounts" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "17. Create Account" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "18. Get Account" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}", name: "19. Get Account by Alias" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "20. Update Account" },
    
    // Portfolio flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "21. List Portfolios" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "22. Create Portfolio" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "23. Get Portfolio" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "24. Update Portfolio" },
    
    // Segment flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "25. List Segments" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "26. Create Segment" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "27. Get Segment" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "28. Update Segment" },
    
    // Transaction flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions", name: "29. List Transactions" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json", name: "30. Create Transaction using JSON" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "31. Get Transaction" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "32. Update Transaction" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit", name: "33. Commit Transaction" },
    
    // Balance flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances", name: "34. Get Account Balances" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances", name: "35. List All Balances" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "36. Get Balance by ID" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances", name: "37. Update Balance" },
    
    // Operation flow
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/operations", name: "38. List Operations" },
    { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/operations/{id}", name: "39. Get Operation" },
    { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/operations/{id}", name: "40. Update Operation" },
    
    // Additional transaction types
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/templates", name: "41. Create Transaction Template" },
    { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert", name: "42. Revert Transaction" },
    
    // Delete flow (reverse order of creation to handle dependencies properly)
    { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "43. Delete Balance" },
    { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "44. Delete Account" },
    { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "45. Delete Segment" },
    { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "46. Delete Portfolio" },
    { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "47. Delete Asset" },
    { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "48. Delete Ledger" },
    { operation: "DELETE", path: "/v1/organizations/{id}", name: "49. Delete Organization" }
  ];

  const workflowFolder = {
    name: "E2E Flow",
    description: "Complete workflow that demonstrates the entire API flow from creating an organization to funding an account and cleaning up resources",
    item: []
  };

  // Find and clone the requests for each step in the workflow
  workflowSequence.forEach((step, index) => {
    const matchingRequest = findRequestByPathAndMethod(collection, step.path, step.operation);
    
    if (matchingRequest) {
      // Clone the request
      const clonedRequest = JSON.parse(JSON.stringify(matchingRequest));
      
      // Update name to include sequence number
      clonedRequest.name = step.name;
      
      // Special case for creating USD asset
      if (step.name === "10. Create USD Asset") {
        // Modify the request body to ensure it creates a USD asset
        if (clonedRequest.request && clonedRequest.request.body) {
          try {
            const bodyObj = JSON.parse(clonedRequest.request.body.raw);
            bodyObj.code = "USD";
            bodyObj.name = "US Dollar";
            clonedRequest.request.body.raw = JSON.stringify(bodyObj, null, 2);
          } catch (e) {
            console.log("Could not parse body for USD asset");
          }
        }
      }
      
      // Special case for creating a transaction to fund account
      if (step.name === "30. Create Transaction using JSON") {
        if (clonedRequest.request && clonedRequest.request.body) {
          // Set transaction body for funding from external source
          const fundingTxBody = {
            "description": "Initial funding from external source",
            "reference": "FUNDING-001",
            "operations": [
              {
                "sourceAccountId": "@external/USD",
                "destinationAccountId": "{{accountId}}",
                "amount": "1000.00",
                "assetCode": "USD"
              }
            ]
          };
          clonedRequest.request.body.raw = JSON.stringify(fundingTxBody, null, 2);
        }
      }

      // Special case for creating AssetRate with USD
      if (step.name === "13. Create AssetRate") {
        if (clonedRequest.request && clonedRequest.request.body) {
          // Set asset rate body for USD
          const assetRateBody = {
            "externalId": "USD-{{$timestamp}}",
            "sourceAssetCode": "USD",
            "rate": 1.0,
            "effectiveDate": new Date().toISOString()
          };
          
          try {
            clonedRequest.request.body.raw = JSON.stringify(assetRateBody, null, 2);
          } catch (e) {
            console.log("Could not set body for AssetRate");
          }
        }
      }
      
      // Special case for create portfolio with relevant values
      if (step.name === "22. Create Portfolio") {
        if (clonedRequest.request && clonedRequest.request.body) {
          try {
            const bodyObj = JSON.parse(clonedRequest.request.body.raw);
            bodyObj.name = "Test Portfolio";
            bodyObj.description = "Portfolio created during E2E test";
            clonedRequest.request.body.raw = JSON.stringify(bodyObj, null, 2);
          } catch (e) {
            console.log("Could not parse body for Portfolio");
          }
        }
      }

      // Special case for create segment with relevant values
      if (step.name === "26. Create Segment") {
        if (clonedRequest.request && clonedRequest.request.body) {
          try {
            const bodyObj = JSON.parse(clonedRequest.request.body.raw);
            bodyObj.name = "Test Segment";
            bodyObj.description = "Segment created during E2E test";
            clonedRequest.request.body.raw = JSON.stringify(bodyObj, null, 2);
          } catch (e) {
            console.log("Could not parse body for Segment");
          }
        }
      }
      
      // Add flow control to test script
      if (clonedRequest.event) {
        const testEvent = clonedRequest.event.find(e => e.listen === "test");
        if (testEvent && testEvent.script) {
          let testScript = testEvent.script.exec.join("\n");
          
          // Special case for saving IDs in variables for use in subsequent tests
          if (step.name === "2. Create Organization") {
            testScript += `
// Save the created organization ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("organizationId", jsonData.id);
    console.log("organizationId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract organizationId: ", error);
}`;
          } else if (step.name === "6. Create Ledger") {
            testScript += `
// Save the created ledger ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("ledgerId", jsonData.id);
    console.log("ledgerId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract ledgerId: ", error);
}`;
          } else if (step.name === "10. Create USD Asset") {
            testScript += `
// Save the created asset ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("assetId", jsonData.id);
    console.log("assetId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract assetId: ", error);
}`;
          } else if (step.name === "13. Create AssetRate") {
            testScript += `
// Save the asset rate external ID to use in subsequent requests
try {
  // Extract the external ID from the request body
  var requestBody = JSON.parse(pm.request.body.raw);
  if (requestBody && requestBody.externalId) {
    pm.environment.set("assetRateId", requestBody.externalId);
    console.log("assetRateId set to: " + requestBody.externalId);
  }
} catch (error) {
  console.error("Failed to extract assetRateId: ", error);
}`;
          } else if (step.name === "17. Create Account") {
            testScript += `
// Save the created account ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("accountId", jsonData.id);
    console.log("accountId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract accountId: ", error);
}`;
          } else if (step.name === "22. Create Portfolio") {
            testScript += `
// Save the created portfolio ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("portfolioId", jsonData.id);
    console.log("portfolioId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract portfolioId: ", error);
}`;
          } else if (step.name === "26. Create Segment") {
            testScript += `
// Save the created segment ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("segmentId", jsonData.id);
    console.log("segmentId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract segmentId: ", error);
}`;
          } else if (step.name === "30. Create Transaction using JSON") {
            testScript += `
// Save the created transaction ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("transactionId", jsonData.id);
    console.log("transactionId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract transactionId: ", error);
}`;
          } else if (step.name === "36. Get Balance by ID") {
            testScript += `
// Save the balance ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("balanceId", jsonData.id);
    console.log("balanceId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract balanceId: ", error);
}`;
          } else if (step.name === "39. Get Operation") {
            testScript += `
// Save the operation ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("operationId", jsonData.id);
    console.log("operationId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract operationId: ", error);
}`;
          }
          
          // Add execution control for workflow
          if (index < workflowSequence.length - 1) {
            // Set next request if not the last step
            testScript += `\n\n// Set next request in workflow\npm.execution.setNextRequest("${workflowSequence[index + 1].name}");`;
          } else {
            // End the workflow on the last step
            testScript += `\n\n// This is the last step in the workflow\nconsole.log("E2E workflow completed successfully!");`;
          }
          
          testEvent.script.exec = testScript.split("\n");
        }
      }
      
      workflowFolder.item.push(clonedRequest);
    } else {
      console.log(`Warning: Could not find request for workflow step: ${step.name}`);
    }
  });
  
  // Add the workflow folder to the collection
  collection.item.push(workflowFolder);
  
  return collection;
}

// Helper function to find a request in the collection by path and method
function findRequestByPathAndMethod(collection, path, method) {
  let result = null;
  
  // Search through all folders
  if (collection.item) {
    for (const folder of collection.item) {
      if (folder.item) {
        for (const request of folder.item) {
          if (request.request && 
              request.request.method === method &&
              request.request.url && 
              request.request.url.raw && 
              request.request.url.raw.includes(path)) {
            return request;
          }
        }
      }
    }
  }
  
  return result;
}

// Post-process the collection
function postProcessCollection(collection) {
  // Add DSL example for transaction DSL endpoint
  if (collection.item) {
    collection.item.forEach(folder => {
      if (folder.item) {
        folder.item.forEach(item => {
          if (item.name && item.name.includes('DSL') && 
              item.request && item.request.url && 
              item.request.url.path && 
              Array.isArray(item.request.url.path) && 
              item.request.url.path.includes('dsl')) {
            
            console.log('Post-processing: Adding DSL body to DSL endpoint');
            
            // Force set body for DSL endpoint
            item.request.body = {
              mode: 'raw',
              raw: `// Transaction DSL Example
// This is a simple transfer between two accounts

transaction {
  description "Fund transfer between accounts"
  reference "TRANSFER-REF-001"
  
  // Account debited $100
  debit {
    account "{{accountId}}"
    amount 100.00
    asset "USD"
  }
  
  // Account credited $100
  credit {
    account "00000000-0000-0000-0000-000000000002"
    amount 100.00
    asset "USD"
  }
}`,
              options: {
                raw: {
                  language: 'text'
                }
              }
            };
          }
        });
      }
    });
  }
  
  // Check if this is the merged collection from sync-postman.sh
  if (collection.info && collection.info.name === "MIDAZ") {
    // Add E2E workflow folder only for the merged collection
    collection = createE2EWorkflow(collection);
  }
  
  return collection;
}

// Process the collection before writing
const processedCollection = postProcessCollection(postmanCollection);

// Write the Postman collection to the output file
try {
  fs.writeFileSync(outputFile, JSON.stringify(processedCollection, null, 2));
  console.log(`Successfully converted ${inputFile} to ${outputFile}`);
} catch (error) {
  console.error(`Error writing output file: ${error.message}`);
  process.exit(1);
}

// Create and write the environment template if requested
if (envOutputFile) {
  const environmentTemplate = createEnvironmentTemplate(enhancedSpec);
  fs.writeFileSync(envOutputFile, JSON.stringify(environmentTemplate, null, 2));
  console.log(`Created environment template at ${envOutputFile}`);
}

process.exit(0);