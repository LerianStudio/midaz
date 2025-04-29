#!/usr/bin/env node

/**
 * OpenAPI to Postman Converter
 * 
 * This script provides a comprehensive tool that:
 * 1. Enhances OpenAPI specs with better examples and descriptions
 * 2. Converts the enhanced specs to Postman collections
 * 
 * Usage: node convert-openapi.js <input-file> <output-file> [--env <env-output-file>]
 * 
 * =====================================================================
 * CODE STRUCTURE INDEX
 * =====================================================================
 * 
 * 1. IMPORTS AND SETUP (Line ~18)
 *    - Required Node.js modules
 * 
 * 2. COMMAND LINE ARGUMENT HANDLING (Line ~25)
 *    - parseCommandLineArgs(): Parse and validate CLI arguments
 *    - ensureDirectoriesExist(): Create output directories if needed
 *    - readOpenApiSpec(): Read and parse OpenAPI spec file
 * 
 * 3. DEPENDENCY MAP AND SCRIPT GENERATION (Line ~100)
 *    - DEPENDENCY_MAP: Maps API endpoints to their dependencies
 *    - generatePreRequestScript(): Create Postman pre-request scripts
 *    - generateTestScript(): Create Postman test scripts
 * 
 * 4. POSTMAN COLLECTION CREATION (Line ~290)
 *    - createPostmanCollection(): Convert OpenAPI spec to Postman collection
 *    - createRequestItem(): Create Postman request item from OpenAPI operation
 *    - createEnvironmentTemplate(): Generate Postman environment template
 * 
 * 5. EXAMPLE GENERATION (Line ~1080)
 *    - generateSendExample(): Generate example for Send objects
 *    - generateAddressExample(): Generate example for Address objects
 *    - generateObjectExample(): Generate examples for complex objects
 *    - generateArrayExample(): Generate examples for arrays
 * 
 * 6. SCHEMA DEFINITIONS (Line ~565)
 *    - ENHANCED_PAGINATION_SCHEMA: Improved pagination schema
 *    - ENHANCED_ERROR_SCHEMA: Improved error schema
 *    - STANDARD_ERROR_RESPONSES: Standard API error responses
 * 
 * 7. OPENAPI ENHANCEMENT (Line ~1380)
 *    - updateOpenApiSpec(): Enhance OpenAPI spec with better components
 *    - updateEndpoints(): Update endpoints to use standard responses
 *    - fixPathParameters(): Fix path parameter formats
 * 
 * 8. MAIN EXECUTION (Line ~1520)
 *    - main(): Main execution function
 *    - ensureExamplesFollowStandards(): Ensure examples follow project standards
 * 
 * =====================================================================
 * PROJECT STANDARDS
 * =====================================================================
 * 
 * - Status fields are represented as objects with a Code field: {"code": "ACTIVE"}
 * - USD is used as the standard currency example
 * - Examples are realistic and consistent across all models
 * - Address examples include detailed information with standard fields
 */

//=============================================================================
// IMPORTS AND SETUP
//=============================================================================

const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');

//=============================================================================
// COMMAND LINE ARGUMENT HANDLING
//=============================================================================

/**
 * Parse and validate command line arguments
 * @returns {Object} Object containing input/output file paths
 */
function parseCommandLineArgs() {
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

  return { inputFile, outputFile, envOutputFile };
}

/**
 * Ensure all required directories exist
 * @param {string} outputFile - Path to the output file
 * @param {string|null} envOutputFile - Path to the environment output file
 */
function ensureDirectoriesExist(outputFile, envOutputFile) {
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
}

/**
 * Read and parse the OpenAPI specification file
 * @param {string} inputFile - Path to the input file
 * @returns {Object} Parsed OpenAPI specification
 */
function readOpenApiSpec(inputFile) {
  // Check if input file exists
  if (!fs.existsSync(inputFile)) {
    console.error(`Input file not found: ${inputFile}`);
    process.exit(1);
  }

  try {
    const fileContent = fs.readFileSync(inputFile, 'utf8');
    
    if (inputFile.endsWith('.json')) {
      return JSON.parse(fileContent);
    } else if (inputFile.endsWith('.yaml') || inputFile.endsWith('.yml')) {
      return yaml.load(fileContent);
    } else {
      console.error('Input file must be JSON or YAML format');
      process.exit(1);
    }
  } catch (error) {
    console.error(`Error reading/parsing input file: ${error.message}`);
    process.exit(1);
  }
}

//=============================================================================
// DEPENDENCY MAP AND SCRIPT GENERATION
//=============================================================================

// Dependency map for API endpoints
const DEPENDENCY_MAP = {
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
    provides: ["accountId", "accountAlias"],
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
    provides: ["balanceId"],
    requires: ["organizationId", "ledgerId", "accountId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "accountId", "balanceId"]
  },
  
  // Asset Rate endpoints
  "POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates": {
    provides: ["assetRateId"],
    requires: ["organizationId", "ledgerId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "assetRateId"]
  },
  
  // Portfolio endpoints
  "POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios": {
    provides: ["portfolioId"],
    requires: ["organizationId", "ledgerId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "portfolioId"]
  },
  
  // Segment endpoints
  "POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments": {
    provides: ["segmentId"],
    requires: ["organizationId", "ledgerId"]
  },
  "GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}": {
    provides: [],
    requires: ["organizationId", "ledgerId", "segmentId"]
  }
};

/**
 * Generate a pre-request script based on the endpoint dependencies
 * @param {Object} operation - The operation object from the OpenAPI spec
 * @param {string} path - The path of the endpoint
 * @param {string} method - The HTTP method of the endpoint
 * @returns {string} The pre-request script
 */
function generatePreRequestScript(operation, path, method) {
  let script = '';
  
  // Get the endpoint key for the dependency map
  const endpointKey = `${method.toUpperCase()} ${path}`;
  const dependencies = DEPENDENCY_MAP[endpointKey] || { requires: [], provides: [] };
  
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

/**
 * Generate a test script based on the endpoint and its expected responses
 * @param {Object} operation - The operation object from the OpenAPI spec
 * @param {string} path - The path of the endpoint
 * @param {string} method - The HTTP method of the endpoint
 * @returns {string} The test script
 */
function generateTestScript(operation, path, method) {
  let script = '';
  
  // Get the endpoint key for the dependency map
  const endpointKey = `${method.toUpperCase()} ${path}`;
  const dependencies = DEPENDENCY_MAP[endpointKey] || { requires: [], provides: [] };
  
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
      // Special handling for accountAlias which is stored as 'alias' in the response
      if (variable === 'accountAlias') {
        script += `
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.alias) {
    pm.environment.set("${variable}", jsonData.alias);
    console.log("${variable} set to: " + jsonData.alias);
  }
} catch (error) {
  console.error("Failed to extract ${variable}: ", error);
}`;
      } 
      // Special handling for balanceId which comes from a list response
      else if (variable === 'balanceId') {
        script += `
try {
  var jsonData = pm.response.json();
  // Check if response is an array with at least one item
  if (Array.isArray(jsonData) && jsonData.length > 0 && jsonData[0].id) {
    pm.environment.set("${variable}", jsonData[0].id);
    console.log("${variable} set to: " + jsonData[0].id);
  } 
  // Check if response has a data array with at least one item
  else if (jsonData && Array.isArray(jsonData.data) && jsonData.data.length > 0 && jsonData.data[0].id) {
    pm.environment.set("${variable}", jsonData.data[0].id);
    console.log("${variable} set to: " + jsonData.data[0].id);
  }
} catch (error) {
  console.error("Failed to extract ${variable}: ", error);
}`;
      } 
      // Default handling for other variables
      else {
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

//=============================================================================
// POSTMAN COLLECTION CREATION
//=============================================================================

/**
 * Create a Postman collection from the OpenAPI spec
 * @param {Object} spec - The OpenAPI spec
 * @returns {Object} The Postman collection
 */
function createPostmanCollection(spec) {
  // Create collection object
  const collection = {
    info: {
      name: spec.info.title || 'MIDAZ API',
      description: spec.info.description || 'API documentation for MIDAZ',
      schema: 'https://schema.getpostman.com/json/collection/v2.1.0/collection.json',
      version: spec.info.version || '1.0.0'
    },
    item: [],
    variable: [
      {
        key: "environment",
        value: "MIDAZ",
        type: "string",
        description: "This collection requires the MIDAZ environment to be selected for proper functionality."
      }
    ]
  };
  
  // Add description about required environment
  if (collection.info.description) {
    collection.info.description += `\n\n**IMPORTANT**: This collection requires the **MIDAZ Environment** to be selected for proper functionality. Please ensure you have imported and selected the MIDAZ environment before using this collection.`;
  } else {
    collection.info.description = `**IMPORTANT**: This collection requires the **MIDAZ Environment** to be selected for proper functionality. Please ensure you have imported and selected the MIDAZ environment before using this collection.`;
  }

  // Group endpoints by tags
  const tagGroups = {};
  
  // Process all paths in the spec
  for (const path in spec.paths) {
    const pathItem = spec.paths[path];
    
    // Process all operations for this path
    for (const method in pathItem) {
      if (method === 'parameters' || method === 'servers' || method === 'summary' || method === 'description') {
        continue; // Skip non-operation properties
      }
      
      const operation = pathItem[method];
      
      // Get tags for this operation
      const tags = operation.tags || ['default'];
      
      // Add operation to each tag group
      tags.forEach(tag => {
        if (!tagGroups[tag]) {
          tagGroups[tag] = {
            name: tag,
            description: getTagDescription(spec, tag),
            item: []
          };
        }
        
        // Create request item for this operation
        const requestItem = createRequestItem(operation, path, method, spec);
        
        // Add request item to tag group
        tagGroups[tag].item.push(requestItem);
      });
    }
  }
  
  // Add tag groups to collection
  for (const tag in tagGroups) {
    collection.item.push(tagGroups[tag]);
  }
  
  return collection;
}

/**
 * Get the description for a tag from the OpenAPI spec
 * @param {Object} spec - The OpenAPI spec
 * @param {string} tagName - The name of the tag
 * @returns {string} The description for the tag
 */
function getTagDescription(spec, tagName) {
  // Default descriptions for common tags
  const defaultDescriptions = {
    'Organizations': 'Endpoints for managing organizations, which are the top-level entities in the MIDAZ system.',
    'Ledgers': 'Endpoints for managing ledgers, which are financial record-keeping systems for tracking assets, accounts, and transactions within an organization.',
    'Accounts': 'Endpoints for managing accounts, which represent individual financial entities like bank accounts, credit cards, or expense categories within a ledger.',
    'Assets': 'Endpoints for managing assets, which represent the types of value that can be transferred between accounts.',
    'Transactions': 'Endpoints for managing transactions, which represent the movement of value between accounts.',
    'Operations': 'Endpoints for managing operations, which are the individual debit and credit entries that make up a transaction.',
    'Balances': 'Endpoints for retrieving account balances, which represent the current value of an account.',
    'Asset Rates': 'Endpoints for managing asset exchange rates, which are used to convert between different asset types.',
    'Portfolios': 'Endpoints for managing portfolios, which are collections of accounts grouped for reporting or management purposes.',
    'Segments': 'Endpoints for managing segments, which are used to categorize accounts for reporting or management purposes.',
    'default': 'API endpoints for the MIDAZ financial system.'
  };
  
  // Check if the tag has a description in the spec
  if (spec.tags) {
    const tag = spec.tags.find(t => t.name === tagName);
    if (tag && tag.description) {
      return tag.description;
    }
  }
  
  // Return default description if available, otherwise a generic description
  return defaultDescriptions[tagName] || `Endpoints related to ${tagName}.`;
}

/**
 * Create a request item for the Postman collection
 * @param {Object} operation - The operation object from the OpenAPI spec
 * @param {string} path - The path of the endpoint
 * @param {string} method - The HTTP method of the endpoint
 * @param {Object} spec - The full OpenAPI spec
 * @returns {Object} The request item
 */
function createRequestItem(operation, path, method, spec) {
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
      url: createUrl(path, baseUrlVariable),
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
  addParameters(requestItem, operation, path);
  
  // Add request body
  addRequestBody(requestItem, operation, spec);
  
  // Add response examples
  addResponseExamples(requestItem, operation, spec);
  
  return requestItem;
}

/**
 * Create a URL object for the Postman collection
 * @param {string} path - The path of the endpoint
 * @param {string} baseUrlVariable - The base URL variable
 * @returns {Object} The URL object
 */
function createUrl(path, baseUrlVariable) {
  return {
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
          if (path.includes('/ledgers/') && !path.includes('/accounts/') && !path.includes('/assets/') && !path.includes('/portfolios/') && !path.includes('/segments/')) return '{{ledgerId}}';
          if (path.includes('/accounts/') && !path.includes('/balances/') && !path.includes('/portfolios/') && !path.includes('/segments/')) return '{{accountId}}';
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
        // Convert any other snake_case params to camelCase
        if (paramName.includes('_')) {
          const camelCaseParam = paramName.replace(/_([a-z])/g, (match, p1) => p1.toUpperCase());
          return `{{${camelCaseParam}}}`;
        }
        return `{{${paramName}}}`;
      }
      return p;
    })
  };
}

/**
 * Add parameters to the request item
 * @param {Object} requestItem - The request item
 * @param {Object} operation - The operation object from the OpenAPI spec
 * @param {string} path - The path of the endpoint
 */
function addParameters(requestItem, operation, path) {
  if (!operation.parameters) return;
  
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
        if (path.includes('/ledgers/') && !path.includes('/accounts/') && !path.includes('/assets/') && !path.includes('/portfolios/') && !path.includes('/segments/')) value = '{{ledgerId}}';
        if (path.includes('/accounts/') && !path.includes('/balances/') && !path.includes('/portfolios/') && !path.includes('/segments/')) value = '{{accountId}}';
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

/**
 * Add request body to the request item
 * @param {Object} requestItem - The request item
 * @param {Object} operation - The operation object from the OpenAPI spec
 * @param {Object} spec - The full OpenAPI spec
 */
function addRequestBody(requestItem, operation, spec) {
  // Add request body if present in OpenAPI 3.0 format
  if (operation.requestBody) {
    const content = operation.requestBody.content || {};
    const jsonContent = content['application/json'];
    
    if (jsonContent) {
      let example = {};
      
      // Try to get example from the schema
      if (jsonContent.schema) {
        example = generateExampleFromSchema(jsonContent.schema, spec);
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
            example = buildExampleFromProperties(schema.properties, spec);
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
}

/**
 * Add response examples to the request item
 * @param {Object} requestItem - The request item
 * @param {Object} operation - The operation object from the OpenAPI spec
 * @param {Object} spec - The full OpenAPI spec
 */
function addResponseExamples(requestItem, operation, spec) {
  if (!operation.responses) return;
  
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
        example = generateExampleFromSchema(jsonContent.schema, spec);
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

/**
 * Create an environment template based on the API and dependency map
 * @param {Object} spec - The OpenAPI spec
 * @returns {Object} The environment template
 */
function createEnvironmentTemplate(spec) {
  const environment = {
    name: 'MIDAZ',
    values: [
      // Authentication
      {
        key: 'authToken',
        value: '',
        type: 'secret',
        enabled: true
      },
      
      // Base URLs
      {
        key: 'baseUrl',
        value: 'http://localhost',
        type: 'default',
        enabled: true
      },
      {
        key: 'onboardingPort',
        value: '3000',
        type: 'default',
        enabled: true
      },
      {
        key: 'transactionPort',
        value: '3001',
        type: 'default',
        enabled: true
      },
      {
        key: 'onboardingUrl',
        value: '{{baseUrl}}:{{onboardingPort}}',
        type: 'default',
        enabled: true
      },
      {
        key: 'transactionUrl',
        value: '{{baseUrl}}:{{transactionPort}}',
        type: 'default',
        enabled: true
      },
      
      // Resource IDs
      {
        key: 'organizationId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'ledgerId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'accountId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'accountAlias',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'assetId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'assetRateId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'balanceId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'operationId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'portfolioId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'segmentId',
        value: '',
        type: 'default',
        enabled: true
      },
      {
        key: 'transactionId',
        value: '',
        type: 'default',
        enabled: true
      }
    ]
  };
  
  return environment;
}

//=============================================================================
// SCHEMA DEFINITIONS
//=============================================================================

// Enhanced pagination schema
const ENHANCED_PAGINATION_SCHEMA = {
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
const ENHANCED_ERROR_SCHEMA = {
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
const STANDARD_ERROR_RESPONSES = {
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
const REQUEST_BODY_EXAMPLES = {};

//=============================================================================
// EXAMPLE GENERATION
//=============================================================================

/**
 * Generate an example for a Send object based on the Midaz API documentation standards
 * @returns {Object} Example Send object
 */
function generateSendExample() {
  return {
    asset: "USD",
    value: 10000,
    scale: 2,
    source: {
      from: [
        {
          account: "account_123",
          amount: {
            asset: "USD",
            value: 10000,
            scale: 2
          },
          description: "Funding from source account",
          chartOfAccounts: "ASSETS",
          metadata: {
            externalId: "ext_123"
          }
        }
      ]
    },
    distribute: {
      to: [
        {
          account: "account_456",
          amount: {
            asset: "USD",
            value: 10000,
            scale: 2
          },
          description: "Transfer to destination account",
          chartOfAccounts: "LIABILITIES",
          metadata: {
            externalId: "ext_456"
          }
        }
      ]
    }
  };
}

/**
 * Generate an example for a complex object based on schema properties
 * @param {Object} schema - The schema object from the OpenAPI spec
 * @returns {Object} Example object
 */
function generateObjectExample(schema) {
  const example = {};
  
  if (!schema || !schema.properties) {
    return example;
  }
  
  for (const [propName, propSchema] of Object.entries(schema.properties)) {
    // Use existing example if available
    if (propSchema.example !== undefined) {
      example[propName] = propSchema.example;
      continue;
    }
    
    // Handle different property types
    switch (propSchema.type) {
      case 'string':
        if (propSchema.format === 'uuid') {
          example[propName] = null;
        } else if (propSchema.format === 'date-time') {
          example[propName] = new Date().toISOString();
        } else if (propName.toLowerCase().includes('status')) {
          // Follow project standard for status fields - always use {"code": "ACTIVE"}
          example[propName] = { code: "ACTIVE" };
        } else if (propName.toLowerCase().includes('currency') || propName.toLowerCase().includes('asset')) {
          // Follow project standard for currency/asset examples
          example[propName] = "USD";
        } else if (propName.toLowerCase() === 'account') {
          example[propName] = "account_123";
        } else {
          example[propName] = `Example ${propName}`;
        }
        break;
      case 'integer':
      case 'number':
        example[propName] = propName.toLowerCase().includes('scale') ? 2 : 100;
        break;
      case 'boolean':
        example[propName] = false;
        break;
      case 'array':
        example[propName] = generateArrayExample(propSchema);
        break;
      case 'object':
        if (propName.toLowerCase() === 'address') {
          // Generate detailed address example
          example[propName] = generateAddressExample();
        } else if (propName.toLowerCase() === 'status') {
          // Follow project standard for status fields - always use {"code": "ACTIVE"}
          example[propName] = { code: "ACTIVE" };
        } else if (propSchema.properties) {
          example[propName] = generateObjectExample(propSchema);
        } else {
          example[propName] = { key: "value" };
        }
        break;
      default:
        if (propSchema.$ref) {
          // Handle reference to another schema
          const refName = propSchema.$ref.split('/').pop();
          
          // Special handling for Send schema
          if (refName === 'Send') {
            example[propName] = generateSendExample();
          } else if (refName.toLowerCase().includes('status')) {
            // Follow project standard for status fields - always use {"code": "ACTIVE"}
            example[propName] = { code: "ACTIVE" };
          } else if (refName.toLowerCase() === 'address') {
            // Generate detailed address example
            example[propName] = generateAddressExample();
          } else {
            example[propName] = null;
          }
        } else if (propSchema.properties) {
          example[propName] = generateObjectExample(propSchema);
        } else {
          example[propName] = null;
        }
    }
  }
  
  return example;
}

/**
 * Generate a detailed address example
 * @returns {Object} Example address object
 */
function generateAddressExample() {
  return {
    city: "New York",
    country: "US",
    line1: "123 Financial Avenue",
    line2: "Suite 1500",
    state: "NY",
    zipCode: "10001"
  };
}

/**
 * Generate an example for an array based on schema properties
 * @param {Object} schema - The array schema object from the OpenAPI spec
 * @returns {Array} Example array
 */
function generateArrayExample(schema) {
  if (!schema || !schema.items) {
    return [];
  }
  
  const itemSchema = schema.items;
  let exampleItem;
  
  // Use existing example if available
  if (itemSchema.example !== undefined) {
    exampleItem = itemSchema.example;
  } else {
    switch (itemSchema.type) {
      case 'string':
        if (itemSchema.format === 'uuid') {
          exampleItem = null;
        } else {
          exampleItem = 'Example string';
        }
        break;
      case 'integer':
      case 'number':
        exampleItem = 100;
        break;
      case 'boolean':
        exampleItem = false;
        break;
      case 'object':
        exampleItem = generateObjectExample(itemSchema);
        break;
      default:
        if (itemSchema.$ref) {
          // Handle reference to another schema
          const refName = itemSchema.$ref.split('/').pop();
          if (refName === 'Send') {
            exampleItem = generateSendExample();
          } else {
            exampleItem = null;
          }
        } else {
          exampleItem = null;
        }
    }
  }
  
  // Return an array with one example item
  return [exampleItem];
}

/**
 * Generate an example from a schema
 * @param {Object} schema - The schema object from the OpenAPI spec
 * @param {Object} spec - The full OpenAPI spec
 * @returns {Object} Example object
 */
function generateExampleFromSchema(schema, spec) {
  if (schema.$ref) {
    // Extract the schema name from the reference
    const schemaName = schema.$ref.split('/').pop();
    
    // Look for the schema definition in the spec
    if (spec.components && spec.components.schemas && spec.components.schemas[schemaName]) {
      const schemaDefinition = spec.components.schemas[schemaName];
      
      // Build example from schema properties
      if (schemaDefinition.properties) {
        return generateObjectExample(schemaDefinition);
      }
    }
  } else if (schema.properties) {
    return generateObjectExample(schema);
  }
  
  return {};
}

/**
 * Build an example from properties
 * @param {Object} properties - The properties object from the OpenAPI spec
 * @param {Object} spec - The full OpenAPI spec
 * @returns {Object} Example object
 */
function buildExampleFromProperties(properties, spec) {
  const example = {};
  
  for (const [propName, propSchema] of Object.entries(properties)) {
    // Use existing example if available
    if (propSchema.example !== undefined) {
      example[propName] = propSchema.example;
      continue;
    }
    
    // Handle different property types
    switch (propSchema.type) {
      case 'string':
        if (propSchema.format === 'uuid') {
          example[propName] = null;
        } else if (propSchema.format === 'date-time') {
          example[propName] = new Date().toISOString();
        } else if (propName.toLowerCase().includes('status')) {
          // Follow project standard for status fields - always use {"code": "ACTIVE"}
          example[propName] = { code: "ACTIVE" };
        } else if (propName.toLowerCase().includes('currency') || propName.toLowerCase().includes('asset')) {
          // Follow project standard for currency/asset examples
          example[propName] = "USD";
        } else {
          example[propName] = `Example ${propName}`;
        }
        break;
      case 'integer':
      case 'number':
        example[propName] = propName.toLowerCase().includes('scale') ? 2 : 100;
        break;
      case 'boolean':
        example[propName] = false;
        break;
      case 'array':
        example[propName] = generateArrayExample(propSchema);
        break;
      case 'object':
        if (propName.toLowerCase() === 'address') {
          // Generate detailed address example
          example[propName] = generateAddressExample();
        } else if (propName.toLowerCase() === 'status') {
          // Follow project standard for status fields - always use {"code": "ACTIVE"}
          example[propName] = { code: "ACTIVE" };
        } else if (propSchema.properties) {
          example[propName] = generateObjectExample(propSchema);
        } else {
          example[propName] = { key: "value" };
        }
        break;
      default:
        if (propSchema.$ref) {
          // Handle reference to another schema
          const refName = propSchema.$ref.split('/').pop();
          
          // Special handling for Send schema
          if (refName === 'Send') {
            example[propName] = generateSendExample();
          } else if (refName.toLowerCase().includes('status')) {
            // Follow project standard for status fields - always use {"code": "ACTIVE"}
            example[propName] = { code: "ACTIVE" };
          } else if (refName.toLowerCase() === 'address') {
            // Generate detailed address example
            example[propName] = generateAddressExample();
          } else {
            example[propName] = null;
          }
        } else if (propSchema.properties) {
          example[propName] = generateObjectExample(propSchema);
        } else {
          example[propName] = null;
        }
    }
  }
  
  return example;
}

//=============================================================================
// OPENAPI ENHANCEMENT
//=============================================================================

/**
 * Update the OpenAPI spec with enhanced components
 * @param {Object} openApiSpec - The OpenAPI spec
 * @returns {Object} The enhanced OpenAPI spec
 */
function updateOpenApiSpec(openApiSpec) {
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
    openApiSpec.components.schemas.Pagination = ENHANCED_PAGINATION_SCHEMA;
  }

  // Update error schema if it exists
  if (openApiSpec.components.schemas.Error) {
    console.log('Enhancing Error schema...');
    openApiSpec.components.schemas.Error = ENHANCED_ERROR_SCHEMA;
  }

  // Add standard error responses
  console.log('Adding standard error responses...');
  for (const [key, value] of Object.entries(STANDARD_ERROR_RESPONSES)) {
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
            if (REQUEST_BODY_EXAMPLES[schemaName]) {
              console.log(`Adding examples to ${methodKey.toUpperCase()} ${pathKey} (${schemaName})...`);
              contentJson.examples = REQUEST_BODY_EXAMPLES[schemaName];
            }
          }
        }
      }
    }
  }

  return openApiSpec;
}

/**
 * Update endpoints to use standard responses
 * @param {Object} openApiSpec - The OpenAPI spec
 * @returns {Object} The updated OpenAPI spec
 */
function updateEndpoints(openApiSpec) {
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

  return openApiSpec;
}

/**
 * Fix path parameter formats (replace :param with {param})
 * @param {Object} spec - The OpenAPI spec
 * @returns {Object} The fixed OpenAPI spec
 */
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
      .replace(/:portfolio_id/g, '{portfolio_id}')
      .replace(/:segment_id/g, '{segment_id}')
      .replace(/:id/g, '{id}');
    
    fixedPaths[fixedPath] = paths[path];
  }
  
  spec.paths = fixedPaths;
  return spec;
}

// Main function
function main() {
  const { inputFile, outputFile, envOutputFile } = parseCommandLineArgs();
  ensureDirectoriesExist(outputFile, envOutputFile);
  
  // Read and parse the OpenAPI spec
  const spec = readOpenApiSpec(inputFile);
  
  // Fix path parameters format (replace :param with {param})
  const fixedSpec = fixPathParameters(spec);
  
  // Update the OpenAPI spec with enhanced components
  const enhancedSpec = updateOpenApiSpec(fixedSpec);
  
  // Update endpoints to use standard responses
  const finalSpec = updateEndpoints(enhancedSpec);
  
  // Create Postman collection
  console.log('Creating Postman collection...');
  const collection = createPostmanCollection(finalSpec);
  
  // Create Postman environment
  let environment = null;
  if (envOutputFile) {
    console.log('Creating Postman environment...');
    environment = createEnvironmentTemplate(finalSpec);
  }
  
  // Ensure all examples follow project standards
  console.log('Ensuring examples follow project standards...');
  ensureExamplesFollowStandards(collection);
  
  // Write the Postman collection to file
  console.log(`Writing Postman collection to ${outputFile}...`);
  fs.writeFileSync(outputFile, JSON.stringify(collection, null, 2));
  
  // Write the Postman environment to file if requested
  if (envOutputFile && environment) {
    console.log(`Writing Postman environment to ${envOutputFile}...`);
    fs.writeFileSync(envOutputFile, JSON.stringify(environment, null, 2));
  }
  
  console.log('Done!');
}

/**
 * Ensure all examples in the Postman collection follow project standards
 * @param {Object} collection - The Postman collection
 */
function ensureExamplesFollowStandards(collection) {
  // Process all items recursively
  if (collection.item && Array.isArray(collection.item)) {
    collection.item.forEach(item => {
      if (item.item && Array.isArray(item.item)) {
        // If this is a folder, process its items
        ensureExamplesFollowStandards(item);
      } else if (item.request && item.request.body && item.request.body.raw) {
        // If this is a request with a body, ensure the body follows standards
        try {
          const body = JSON.parse(item.request.body.raw);
          
          // Fix status fields
          if (body.status === null) {
            body.status = { code: "ACTIVE" };
          }
          
          // Fix address fields
          if (body.address === null) {
            body.address = generateAddressExample();
          }
          
          // Update the request body
          item.request.body.raw = JSON.stringify(body, null, 2);
        } catch (error) {
          // If the body is not valid JSON, skip it
          console.log(`Warning: Could not parse request body for ${item.name}`);
        }
      }
    });
  }
}

// Run the main function
main();