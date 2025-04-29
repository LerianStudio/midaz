#!/usr/bin/env node

/**
 * E2E Flow Generator for Postman Collections
 * 
 * This script creates a comprehensive end-to-end testing workflow by:
 * 1. Adding an "E2E Flow" folder to a Postman collection
 * 2. Organizing API endpoints in a logical sequence for complete system testing
 * 3. Customizing request parameters and test scripts for proper execution
 * 
 * Usage: node create-e2e-flow.js <collection-file>
 */

const fs = require('fs');

// Parse command line arguments
const args = process.argv.slice(2);
if (args.length < 1) {
  console.error('Usage: node create-e2e-flow.js <collection-file>');
  process.exit(1);
}

const collectionFile = args[0];

// Validate collection file exists
if (!fs.existsSync(collectionFile)) {
  console.error(`Collection file not found: ${collectionFile}`);
  process.exit(1);
}

// Load and parse the Postman collection
let collection;
try {
  const fileContent = fs.readFileSync(collectionFile, 'utf8');
  collection = JSON.parse(fileContent);
  
  // Clean up any existing E2E Flow content to avoid duplicates
  if (collection.item) {
    // Remove any folders named "E2E Flow"
    collection.item = collection.item.filter(item => item.name !== "E2E Flow");
    
    // Remove any numbered requests that might be from previous E2E flow generations
    // (Requests with pattern "XX. Name" are considered E2E flow items)
    for (let i = 0; i < collection.item.length; i++) {
      if (collection.item[i].item) {
        collection.item[i].item = collection.item[i].item.filter(item => {
          return !(item.name && /^\d+\.\s+/.test(item.name));
        });
      }
    }
  }
} catch (error) {
  console.error(`Error reading/parsing collection file: ${error.message}`);
  process.exit(1);
}

// Define the complete E2E testing workflow sequence
// Each step includes the HTTP method, API path, display name, and optional path pattern for matching
const workflowSteps = [
  // Organization management flow
  { operation: "GET", path: "/v1/organizations", name: "1. List Organizations" },
  { operation: "POST", path: "/v1/organizations", name: "2. Create Organization" },
  { operation: "GET", path: "/v1/organizations/{id}", name: "3. Get Organization" },
  { operation: "PATCH", path: "/v1/organizations/{id}", name: "4. Update Organization", pathPattern: "/organizations/{id}$" },
  
  // Ledger management flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers", name: "5. List Ledgers" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers", name: "6. Create Ledger" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "7. Get Ledger" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "8. Update Ledger", pathPattern: "/ledgers/{id}$" },
  
  // Asset management flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets", name: "9. List Assets" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets", name: "10. Create BRL Asset" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "11. Get Asset" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "12. Update Asset", pathPattern: "/assets/{id}$" },
  
  // Account management flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "13. List Accounts" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "14. Create Account" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "15. Get Account" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}", name: "16. Get Account by Alias" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "17. Update Account", pathPattern: "/accounts/{id}$" },
  
  // Portfolio management flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "18. List Portfolios" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "19. Create Portfolio" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "20. Get Portfolio" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "21. Update Portfolio", pathPattern: "/portfolios/{id}$" },
  
  // Segment management flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "22. List Segments" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "23. Create Segment" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "24. Get Segment" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "25. Update Segment", pathPattern: "/segments/{id}$" },
  
  // Transaction management flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions", name: "26. List Transactions" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json", name: "27. Create Transaction using JSON" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "28. Get Transaction" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "29. Update Transaction", pathPattern: "/transactions/{transaction_id}$" },
  
  // Balance management flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances", name: "30. Get Account Balances" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances", name: "31. List All Balances" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "32. Get Balance by ID" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "33. Update Balance", pathPattern: "/balances/{balance_id}$" },
  
  // Operations flow (account-scoped since global operations endpoints aren't available)
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations", name: "34. List Account Operations" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations/{id}", name: "35. Get Account Operation" },
  
  // Special transaction operations
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}/revert", name: "36. Revert Transaction" },
  
  // Resource cleanup flow (in reverse dependency order)
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "37. Delete Balance" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "38. Delete Account" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "39. Delete Segment" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "40. Delete Portfolio" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "41. Delete Asset" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "42. Delete Ledger" },
  { operation: "DELETE", path: "/v1/organizations/{id}", name: "43. Delete Organization" }
];

// Define the container folder for the E2E workflow
const workflowFolder = {
  name: "E2E Flow",
  description: "Complete workflow that demonstrates the entire API flow from creating an organization to funding an account and cleaning up resources",
  item: []
};

// Sample request bodies and examples for specific endpoints
const endpointExamples = {
  // Transaction JSON example with source and destination accounts
  transactionJsonExample: {
    "chartOfAccountsGroupName": "PIX_TRANSACTIONS",
    "description": "New Transaction",
    "metadata": {
      "reference": "TRANSACTION-001", 
      "source": "api"
    },
    "send": {
      "asset": "USD",
      "value": 100,
      "scale": 2,
      "source": {
        "from": [
          {
            "account": "@external/USD",
            "amount": {
              "asset": "USD",
              "value": 100,
              "scale": 2
            },
            "description": "Debit Operation",
            "chartOfAccounts": "PIX_DEBIT",
            "metadata": {
              "operation": "funding",
              "type": "external"
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "account": "{{accountAlias}}",
            "amount": {
              "asset": "USD",
              "value": 100,
              "scale": 2
            },
            "description": "Credit Operation",
            "chartOfAccounts": "PIX_CREDIT",
            "metadata": {
              "operation": "funding",
              "type": "account"
            }
          }
        ]
      }
    }
  },
  
  // Asset rate definition example with timestamp
  assetRateExample: {
    "externalId": "USD-{{$timestamp}}",
    "sourceAssetCode": "USD",
    "rate": 1.0,
    "effectiveDate": new Date().toISOString()
  },
  
  // Simple funding transaction example for quick account funding
  fundingTransactionExample: {
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
  }
};

/**
 * Customizes DSL transaction endpoints in the Postman collection
 * 
 * Modifies the request to use proper file upload format for DSL endpoints,
 * replacing any existing body with a formdata structure that includes
 * a file upload field for the transaction DSL file.
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeDslEndpoint(request) {
  if (!request) return;
  
  console.log('Customizing: Adding DSL body to DSL endpoint');
  
  // Add proper file upload for DSL endpoints
  request.body = {
    mode: 'formdata',
    formdata: [
      {
        key: 'transaction',
        type: 'file',
        src: null,
        description: 'DSL file containing transaction definition'
      }
    ]
  };
}

/**
 * Customizes JSON transaction endpoints in the Postman collection
 * 
 * Updates the request body with a complete transaction example that includes
 * source and destination accounts with proper metadata.
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeTransactionJsonEndpoint(request) {
  if (!request || !request.body) return;
  
  console.log('Customizing: Adding complete send example to Transaction JSON endpoint');
  
  // Update the request body with the full example
  if (request.body.mode === 'raw') {
    request.body.raw = JSON.stringify(endpointExamples.transactionJsonExample, null, 2);
  }
  
  // Add test script to extract transaction ID
  if (request.event && request.event.length > 0) {
    // Find the test script event
    const testEvent = request.event.find(e => e.listen === 'test');
    if (testEvent && testEvent.script) {
      // Add script to extract transaction ID
      const transactionIdScript = `
// Extract transaction ID and store it in the environment
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("transactionId", jsonData.id);
    console.log("transactionId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract transactionId: ", error);
}`;
      
      // Add the script to the end of the existing test script
      if (Array.isArray(testEvent.script.exec)) {
        testEvent.script.exec.push(...transactionIdScript.split('\n'));
      }
    }
  }
}

/**
 * Customizes account creation endpoints in the Postman collection
 * 
 * Makes several important modifications to account creation requests:
 * 1. Changes asset code from USD to BRL if needed
 * 2. Removes invalid zero UUIDs for parent account, portfolio, and segment
 * 3. Adds test scripts to extract and store account alias for later use
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeAccountEndpoint(request) {
  if (!request || !request.body) return;
  
  console.log('Customizing: Updating Account creation payload');
  
  // Update the request body with BRL as asset code and remove problematic IDs
  if (request.body.mode === 'raw') {
    try {
      const bodyObj = JSON.parse(request.body.raw);
      
      // Change assetCode from USD to BRL
      if (bodyObj.assetCode === "USD") {
        console.log('Changing asset code from USD to BRL');
        bodyObj.assetCode = "BRL";
      }
      
      // Remove parentAccountId if zero UUID (which doesn't exist)
      if (bodyObj.parentAccountId === "00000000-0000-0000-0000-000000000000") {
        console.log('Removing parentAccountId zero UUID');
        delete bodyObj.parentAccountId;
      }
      
      // Remove portfolioId if zero UUID (which doesn't exist)
      if (bodyObj.portfolioId === "00000000-0000-0000-0000-000000000000") {
        console.log('Removing portfolioId zero UUID');
        delete bodyObj.portfolioId;
      }
      
      // Remove segmentId if zero UUID (which doesn't exist)
      if (bodyObj.segmentId === "00000000-0000-0000-0000-000000000000") {
        console.log('Removing segmentId zero UUID');
        delete bodyObj.segmentId;
      }
      
      // Update the request body
      request.body.raw = JSON.stringify(bodyObj, null, 2);
      
      // Add test script to extract account alias and store it in the environment
      if (request.event && request.event.length > 0) {
        // Find the test script event
        const testEvent = request.event.find(e => e.listen === 'test');
        if (testEvent && testEvent.script) {
          // Add script to extract account alias
          const accountAliasScript = `
// Extract account alias and store it in the environment
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.alias) {
    pm.environment.set("accountAlias", jsonData.alias);
    console.log("accountAlias set to: " + jsonData.alias);
  }
  // Also extract the accountId
  if (jsonData && jsonData.id) {
    pm.environment.set("accountId", jsonData.id);
    console.log("accountId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract account data: ", error);
}`;
          
          // Add the script to the end of the existing test script
          if (Array.isArray(testEvent.script.exec)) {
            // Remove any existing accountAlias extraction to avoid duplicates
            testEvent.script.exec = testEvent.script.exec.filter(
              line => !line.includes('accountAlias') && !line.includes('account alias')
            );
            // Add the new script
            testEvent.script.exec.push(...accountAliasScript.split('\n'));
          }
        }
      }
    } catch (e) {
      console.log('Error updating account payload:', e.message);
    }
  }
}

/**
 * Customizes account update endpoints in the Postman collection
 * 
 * Removes invalid zero UUIDs for portfolio and segment IDs from
 * account update requests to prevent API errors.
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeAccountUpdateEndpoint(request) {
  if (!request || !request.body) return;
  
  console.log('Customizing: Updating Account update payload');
  
  // Update the request body to remove problematic IDs
  if (request.body.mode === 'raw') {
    try {
      const bodyObj = JSON.parse(request.body.raw);
      
      // Remove portfolioId if zero UUID (which doesn't exist)
      if (bodyObj.portfolioId === "00000000-0000-0000-0000-000000000000") {
        console.log('Removing portfolioId zero UUID from update payload');
        delete bodyObj.portfolioId;
      }
      
      // Remove segmentId if zero UUID (which doesn't exist)
      if (bodyObj.segmentId === "00000000-0000-0000-0000-000000000000") {
        console.log('Removing segmentId zero UUID from update payload');
        delete bodyObj.segmentId;
      }
      
      // Update the request body
      request.body.raw = JSON.stringify(bodyObj, null, 2);
    } catch (e) {
      console.log('Error updating account update payload:', e.message);
    }
  }
}

/**
 * Customizes portfolio creation endpoints in the Postman collection
 * 
 * Adds test script to extract portfolioId for use in other requests
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizePortfolioCreateEndpoint(request) {
  if (!request) return;
  
  console.log('Customizing: Adding portfolioId extraction to Portfolio creation');
  
  // Add test script to extract portfolio ID
  if (request.event && request.event.length > 0) {
    // Find the test script event
    const testEvent = request.event.find(e => e.listen === 'test');
    if (testEvent && testEvent.script) {
      // Add script to extract portfolio ID
      const portfolioIdScript = `
// Extract portfolio ID and store it in the environment
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("portfolioId", jsonData.id);
    console.log("portfolioId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract portfolioId: ", error);
}`;
      
      // Add the script to the end of the existing test script
      if (Array.isArray(testEvent.script.exec)) {
        // Remove any existing portfolioId extraction to avoid duplicates
        testEvent.script.exec = testEvent.script.exec.filter(
          line => !line.includes('portfolioId') && !line.includes('portfolio ID')
        );
        // Add the new script
        testEvent.script.exec.push(...portfolioIdScript.split('\n'));
      }
    }
  }
}

/**
 * Customizes segment creation endpoints in the Postman collection
 * 
 * Adds test script to extract segmentId for use in other requests
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeSegmentCreateEndpoint(request) {
  if (!request) return;
  
  console.log('Customizing: Adding segmentId extraction to Segment creation');
  
  // Add test script to extract segment ID
  if (request.event && request.event.length > 0) {
    // Find the test script event
    const testEvent = request.event.find(e => e.listen === 'test');
    if (testEvent && testEvent.script) {
      // Add script to extract segment ID
      const segmentIdScript = `
// Extract segment ID and store it in the environment
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("segmentId", jsonData.id);
    console.log("segmentId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract segmentId: ", error);
}`;
      
      // Add the script to the end of the existing test script
      if (Array.isArray(testEvent.script.exec)) {
        // Remove any existing segmentId extraction to avoid duplicates
        testEvent.script.exec = testEvent.script.exec.filter(
          line => !line.includes('segmentId') && !line.includes('segment ID')
        );
        // Add the new script
        testEvent.script.exec.push(...segmentIdScript.split('\n'));
      }
    }
  }
}

/**
 * Customizes balance endpoints to extract balanceId
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeBalanceEndpoint(request) {
  if (!request) return;
  
  console.log('Customizing: Adding balanceId extraction to balance endpoints');
  
  // Add test script to extract balance ID
  if (request.event && request.event.length > 0) {
    // Find the test script event
    const testEvent = request.event.find(e => e.listen === 'test');
    if (testEvent && testEvent.script) {
      // Add script to extract balance ID
      const balanceIdScript = `
// Extract balance ID from the first item in the response if it's an array
try {
  var jsonData = pm.response.json();
  if (Array.isArray(jsonData) && jsonData.length > 0 && jsonData[0].id) {
    pm.environment.set("balanceId", jsonData[0].id);
    console.log("balanceId set to: " + jsonData[0].id);
  } 
  // For single balance response
  else if (jsonData && jsonData.id) {
    pm.environment.set("balanceId", jsonData.id);
    console.log("balanceId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract balanceId: ", error);
}`;
      
      // Add the script to the end of the existing test script
      if (Array.isArray(testEvent.script.exec)) {
        // Remove any existing balanceId extraction to avoid duplicates
        testEvent.script.exec = testEvent.script.exec.filter(
          line => !line.includes('balanceId') && !line.includes('balance ID')
        );
        // Add the new script
        testEvent.script.exec.push(...balanceIdScript.split('\n'));
      }
    }
  }
}

/**
 * Customizes operation list endpoint to extract operationId
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeOperationEndpoint(request) {
  if (!request) return;
  
  console.log('Customizing: Adding operationId extraction to operation list endpoint');
  
  // Add test script to extract operation ID
  if (request.event && request.event.length > 0) {
    // Find the test script event
    const testEvent = request.event.find(e => e.listen === 'test');
    if (testEvent && testEvent.script) {
      // Add script to extract operation ID
      const operationIdScript = `
// Extract operation ID from the first item in the response if it's an array
try {
  var jsonData = pm.response.json();
  if (Array.isArray(jsonData) && jsonData.length > 0 && jsonData[0].id) {
    pm.environment.set("operationId", jsonData[0].id);
    console.log("operationId set to: " + jsonData[0].id);
  } 
  // For single operation response
  else if (jsonData && jsonData.id) {
    pm.environment.set("operationId", jsonData.id);
    console.log("operationId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract operationId: ", error);
}`;
      
      // Add the script to the end of the existing test script
      if (Array.isArray(testEvent.script.exec)) {
        // Remove any existing operationId extraction to avoid duplicates
        testEvent.script.exec = testEvent.script.exec.filter(
          line => !line.includes('operationId') && !line.includes('operation ID')
        );
        // Add the new script
        testEvent.script.exec.push(...operationIdScript.split('\n'));
      }
    }
  }
}

/**
 * Customizes portfolio update endpoints in the Postman collection
 * 
 * Fixes several common issues with portfolio update requests:
 * 1. Ensures URL path uses the correct ID parameter format
 * 2. Adds pre-request script to set the ID variable from portfolioId environment variable
 * 3. Corrects path components that might use incorrect variable references
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizePortfolioUpdateEndpoint(request) {
  if (!request || !request.url) return;
  
  console.log('Customizing: Updating Portfolio URL to use proper ID');
  
  // Keep the variable name as "id" but set its value to {{portfolioId}}
  if (request.url.variable) {
    for (let i = 0; i < request.url.variable.length; i++) {
      if (request.url.variable[i].key === "id") {
        // Update the URL variable value to use portfolioId but keep the key as "id"
        request.url.variable[i].value = "{{portfolioId}}";
        console.log('Updated URL variable value to use portfolioId');
      }
    }
  }
  
  // Update URL raw if needed
  if (request.url.raw) {
    // The API expects portfolios/{id} so keep it that way but set the value properly
    // Fix any instances where the path is wrong
    request.url.raw = request.url.raw.replace(/portfolios\/\{\{portfolioId\}\}/g, "portfolios/{id}");
    
    // Replace portfolios/{{ledgerId}} with portfolios/{id}
    request.url.raw = request.url.raw.replace(/portfolios\/\{\{ledgerId\}\}/g, "portfolios/{id}");
    
    console.log('Updated raw URL to use correct format: portfolios/{id}');
  }
  
  // Update path components - CRITICAL FIX
  if (request.url.path) {
    // Find the last element in the path array (which should be the entity ID)
    const lastIndex = request.url.path.length - 1;
    
    // Check if the last element is using ledgerId incorrectly
    if (request.url.path[lastIndex] === "{{ledgerId}}") {
      request.url.path[lastIndex] = "{id}";
      console.log('Fixed incorrect ledgerId in path - replaced with {id}');
    }
    
    // Also update any {{portfolioId}} in the path to {id}
    for (let i = 0; i < request.url.path.length; i++) {
      if (request.url.path[i] === "{{portfolioId}}") {
        request.url.path[i] = "{id}";
        console.log('Fixed path component to use {id} instead of {{portfolioId}}');
      }
    }
  }
  
  // Add a pre-request script to ensure the id variable gets the portfolioId value
  if (request.event && request.event.length > 0) {
    // Find the pre-request script event
    const preRequestEvent = request.event.find(e => e.listen === 'prerequest');
    if (preRequestEvent && preRequestEvent.script) {
      // Add script to set the id variable
      const idSetupScript = `
// Set the id path parameter to use portfolioId
pm.variables.set("id", pm.environment.get("portfolioId"));
console.log("Set id path parameter to use portfolioId value: " + pm.environment.get("portfolioId"));`;
      
      // Add the script to the end of the existing pre-request script
      if (Array.isArray(preRequestEvent.script.exec)) {
        // Remove any existing id setup for portfolioId to avoid duplicates
        preRequestEvent.script.exec = preRequestEvent.script.exec.filter(
          line => !line.includes('pm.variables.set("id"') && 
                  !line.includes('portfolioId')
        );
        // Add the new script
        preRequestEvent.script.exec.push(...idSetupScript.split('\n'));
      }
    } else {
      // Create a new pre-request script if none exists
      request.event.push({
        listen: "prerequest",
        script: {
          type: "text/javascript",
          exec: [
            "// Set the id path parameter to use portfolioId",
            "pm.variables.set(\"id\", pm.environment.get(\"portfolioId\"));",
            "console.log(\"Set id path parameter to use portfolioId value: \" + pm.environment.get(\"portfolioId\"));"
          ]
        }
      });
    }
    console.log('Added pre-request script to set id parameter');
  }
}

/**
 * Customizes segment update endpoints in the Postman collection
 * 
 * Similar to portfolio updates, this function fixes common issues with segment updates:
 * 1. Ensures URL path uses the correct ID parameter format
 * 2. Adds pre-request script to set the ID variable from segmentId environment variable
 * 3. Corrects path components that might use incorrect variable references
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeSegmentUpdateEndpoint(request) {
  if (!request || !request.url) return;
  
  console.log('Customizing: Updating Segment URL to use proper ID');
  
  // Keep the variable name as "id" but set its value to {{segmentId}}
  if (request.url.variable) {
    for (let i = 0; i < request.url.variable.length; i++) {
      if (request.url.variable[i].key === "id") {
        // Update the URL variable value to use segmentId but keep the key as "id"
        request.url.variable[i].value = "{{segmentId}}";
        console.log('Updated URL variable value to use segmentId');
      }
    }
  }
  
  // Update URL raw if needed
  if (request.url.raw) {
    // The API expects segments/{id} so keep it that way but set the value properly
    // Fix any instances where the path is wrong
    request.url.raw = request.url.raw.replace(/segments\/\{\{segmentId\}\}/g, "segments/{id}");
    
    // Replace segments/{{ledgerId}} with segments/{id}
    request.url.raw = request.url.raw.replace(/segments\/\{\{ledgerId\}\}/g, "segments/{id}");
    
    console.log('Updated raw URL to use correct format: segments/{id}');
  }
  
  // Update path components - CRITICAL FIX
  if (request.url.path) {
    // Find the last element in the path array (which should be the entity ID)
    const lastIndex = request.url.path.length - 1;
    
    // Check if the last element is using ledgerId incorrectly
    if (request.url.path[lastIndex] === "{{ledgerId}}") {
      request.url.path[lastIndex] = "{id}";
      console.log('Fixed incorrect ledgerId in path - replaced with {id}');
    }
    
    // Also update any {{segmentId}} in the path to {id}
    for (let i = 0; i < request.url.path.length; i++) {
      if (request.url.path[i] === "{{segmentId}}") {
        request.url.path[i] = "{id}";
        console.log('Fixed path component to use {id} instead of {{segmentId}}');
      }
    }
  }
  
  // Add a pre-request script to ensure the id variable gets the segmentId value
  if (request.event && request.event.length > 0) {
    // Find the pre-request script event
    const preRequestEvent = request.event.find(e => e.listen === 'prerequest');
    if (preRequestEvent && preRequestEvent.script) {
      // Add script to set the id variable
      const idSetupScript = `
// Set the id path parameter to use segmentId
pm.variables.set("id", pm.environment.get("segmentId"));
console.log("Set id path parameter to use segmentId value: " + pm.environment.get("segmentId"));`;
      
      // Add the script to the end of the existing pre-request script
      if (Array.isArray(preRequestEvent.script.exec)) {
        // Remove any existing id setup for segmentId to avoid duplicates
        preRequestEvent.script.exec = preRequestEvent.script.exec.filter(
          line => !line.includes('pm.variables.set("id"') &&
                  !line.includes('segmentId')
        );
        // Add the new script
        preRequestEvent.script.exec.push(...idSetupScript.split('\n'));
      }
    } else {
      // Create a new pre-request script if none exists
      request.event.push({
        listen: "prerequest",
        script: {
          type: "text/javascript",
          exec: [
            "// Set the id path parameter to use segmentId",
            "pm.variables.set(\"id\", pm.environment.get(\"segmentId\"));",
            "console.log(\"Set id path parameter to use segmentId value: \" + pm.environment.get(\"segmentId\"));"
          ]
        }
      });
    }
    console.log('Added pre-request script to set id parameter');
  }
}

/**
 * Customizes transaction update endpoints in the Postman collection
 * 
 * Ensures transaction update requests use the correct transaction_id parameter
 * by adding a pre-request script that sets it from the transactionId environment variable.
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeTransactionUpdateEndpoint(request) {
  if (!request || !request.url) return;
  
  console.log('Customizing: Updating Transaction URL to use proper ID');
  
  // Add a pre-request script to ensure the transaction_id variable gets the transactionId value
  if (request.event && request.event.length > 0) {
    // Find the pre-request script event
    const preRequestEvent = request.event.find(e => e.listen === 'prerequest');
    if (preRequestEvent && preRequestEvent.script) {
      // Add script to set the transaction_id variable
      const idSetupScript = `
// Set the transaction_id path parameter to use transactionId
pm.variables.set("transaction_id", pm.environment.get("transactionId"));
console.log("Set transaction_id path parameter to use transactionId value: " + pm.environment.get("transactionId"));`;
      
      // Add the script to the end of the existing pre-request script
      if (Array.isArray(preRequestEvent.script.exec)) {
        // Remove any existing transaction_id setup to avoid duplicates
        preRequestEvent.script.exec = preRequestEvent.script.exec.filter(
          line => !line.includes('pm.variables.set("transaction_id"') &&
                  !line.includes('transactionId')
        );
        // Add the new script
        preRequestEvent.script.exec.push(...idSetupScript.split('\n'));
      }
    } else {
      // Create a new pre-request script if none exists
      request.event.push({
        listen: "prerequest",
        script: {
          type: "text/javascript",
          exec: [
            "// Set the transaction_id path parameter to use transactionId",
            "pm.variables.set(\"transaction_id\", pm.environment.get(\"transactionId\"));",
            "console.log(\"Set transaction_id path parameter to use transactionId value: \" + pm.environment.get(\"transactionId\"));"
          ]
        }
      });
    }
    console.log('Added pre-request script to set transaction_id parameter');
  }
}

/**
 * Customizes balance update endpoints in the Postman collection
 * 
 * Ensures balance update requests use the correct balance_id parameter
 * by adding a pre-request script that sets it from the balanceId environment variable.
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeBalanceUpdateEndpoint(request) {
  if (!request || !request.url) return;
  
  console.log('Customizing: Updating Balance URL to use proper ID');
  
  // Add a pre-request script to ensure the balance_id variable gets the balanceId value
  if (request.event && request.event.length > 0) {
    // Find the pre-request script event
    const preRequestEvent = request.event.find(e => e.listen === 'prerequest');
    if (preRequestEvent && preRequestEvent.script) {
      // Add script to set the balance_id variable
      const idSetupScript = `
// Set the balance_id path parameter to use balanceId
pm.variables.set("balance_id", pm.environment.get("balanceId"));
console.log("Set balance_id path parameter to use balanceId value: " + pm.environment.get("balanceId"));`;
      
      // Add the script to the end of the existing pre-request script
      if (Array.isArray(preRequestEvent.script.exec)) {
        // Remove any existing balance_id setup to avoid duplicates
        preRequestEvent.script.exec = preRequestEvent.script.exec.filter(
          line => !line.includes('pm.variables.set("balance_id"') &&
                  !line.includes('balanceId')
        );
        // Add the new script
        preRequestEvent.script.exec.push(...idSetupScript.split('\n'));
      }
    } else {
      // Create a new pre-request script if none exists
      request.event.push({
        listen: "prerequest",
        script: {
          type: "text/javascript",
          exec: [
            "// Set the balance_id path parameter to use balanceId",
            "pm.variables.set(\"balance_id\", pm.environment.get(\"balanceId\"));",
            "console.log(\"Set balance_id path parameter to use balanceId value: \" + pm.environment.get(\"balanceId\"));"
          ]
        }
      });
    }
    console.log('Added pre-request script to set balance_id parameter');
  }
}

/**
 * Customizes "Get Account by Alias" endpoints in the Postman collection
 * 
 * Updates the request to properly use the accountAlias environment variable:
 * 1. Replaces any {alias} or {{alias}} references with {{accountAlias}}
 * 2. Updates URL path components to use the correct variable
 * 3. Adds a pre-request script to set a default alias if none exists
 * 
 * @param {object} request - The Postman request object to customize
 */
function customizeGetAccountByAliasEndpoint(request) {
  if (!request || !request.url) return;
  
  console.log('Customizing: Updating Get Account by Alias to use accountAlias variable');
  
  // Replace any occurrence of {alias} or {{alias}} with {{accountAlias}}
  if (request.url.raw) {
    request.url.raw = request.url.raw.replace(/\{\{alias\}\}|\{alias\}/g, "{{accountAlias}}");
    console.log('Updated raw URL to use accountAlias');
  }
  
  // Update path components
  if (request.url.path) {
    for (let i = 0; i < request.url.path.length; i++) {
      if (request.url.path[i] === "{alias}" || 
          request.url.path[i] === "{{alias}}" ||
          request.url.path[i] === "alias" ||
          request.url.path[i] === "{accountAlias}") {
        request.url.path[i] = "{{accountAlias}}";
        console.log('Updated URL path to use accountAlias');
      }
    }
  }
  
  // Update variable definitions
  if (request.url.variable) {
    for (let i = 0; i < request.url.variable.length; i++) {
      if (request.url.variable[i].key === "alias" ||
          request.url.variable[i].key === "accountAlias") {
        request.url.variable[i].value = "{{accountAlias}}";
        console.log('Updated URL variable to use accountAlias');
      }
    }
  }
  
  // Add a pre-request script to ensure accountAlias is set
  if (request.event && request.event.length > 0) {
    // Find the pre-request script event
    const preRequestEvent = request.event.find(e => e.listen === 'prerequest');
    if (preRequestEvent && preRequestEvent.script) {
      // Add script to check for accountAlias
      const accountAliasCheckScript = `
// Check if accountAlias is set, otherwise use a default value
if (!pm.environment.get("accountAlias")) {
  console.log("Warning: accountAlias is not set, using a default value");
  pm.environment.set("accountAlias", "default-account-alias");
}`;
      
      // Add the script to the end of the existing pre-request script
      if (Array.isArray(preRequestEvent.script.exec)) {
        // Remove any existing accountAlias check to avoid duplicates
        preRequestEvent.script.exec = preRequestEvent.script.exec.filter(
          line => !line.includes('accountAlias') && !line.includes('account alias')
        );
        // Add the new script
        preRequestEvent.script.exec.push(...accountAliasCheckScript.split('\n'));
      }
    }
  }
}

/**
 * Recursively processes all items in a collection and applies endpoint-specific customizations
 * 
 * This function walks through the entire collection structure and applies the appropriate
 * customization function to each request based on its type and name.
 * 
 * @param {Array} items - The collection items to process
 */
function customizeEndpoints(items) {
  if (!items) return;
  
  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    
    // If this is a folder with subitems, process them
    if (item.item) {
      customizeEndpoints(item.item);
    }
    
    // If this is a request, process it
    if (item.request) {
      // Fix specific endpoints
      
      // Post-process DSL endpoints
      if (item.name === 'Create a Transaction using DSL' || 
          (item.name.includes('DSL') && item.request.method === 'POST')) {
        customizeDslEndpoint(item.request);
      }
      
      // Post-process Transaction JSON endpoint
      if (item.name === 'Create a Transaction using JSON' || 
          item.name === '27. Create Transaction using JSON' ||
          (item.name.includes('Transaction') && item.name.includes('JSON') && item.request.method === 'POST')) {
        customizeTransactionJsonEndpoint(item.request);
      }

      // Post-process Account creation endpoint
      if (item.name === "14. Create Account" || 
          (item.name.includes('Account') && item.request.method === 'POST')) {
        customizeAccountEndpoint(item.request);
      }
      
      // Post-process Account update endpoint
      if (item.name === "17. Update Account" || 
          (item.name.includes('Update Account') && item.request.method === 'PATCH')) {
        customizeAccountUpdateEndpoint(item.request);
      }
      
      // Post-process Portfolio creation endpoint
      if (item.name === "19. Create Portfolio" ||
          (item.name.includes('Create Portfolio') && item.request.method === 'POST')) {
        customizePortfolioCreateEndpoint(item.request);
      }
      
      // Post-process Portfolio update endpoint
      if (item.name === "21. Update Portfolio" || 
          (item.name.includes('Update Portfolio') && item.request.method === 'PATCH')) {
        customizePortfolioUpdateEndpoint(item.request);
      }
      
      // Post-process Segment creation endpoint
      if (item.name === "23. Create Segment" ||
          (item.name.includes('Create Segment') && item.request.method === 'POST')) {
        customizeSegmentCreateEndpoint(item.request);
      }
      
      // Post-process Segment update endpoint
      if (item.name === "25. Update Segment" || 
          (item.name.includes('Update Segment') && item.request.method === 'PATCH')) {
        customizeSegmentUpdateEndpoint(item.request);
      }
      
      // Post-process Portfolio get endpoint
      if (item.name === "20. Get Portfolio" || 
          (item.name.includes('Get Portfolio') && item.request.method === 'GET')) {
        customizePortfolioUpdateEndpoint(item.request);
      }
      
      // Post-process Segment get endpoint
      if (item.name === "24. Get Segment" || 
          (item.name.includes('Get Segment') && item.request.method === 'GET')) {
        customizeSegmentUpdateEndpoint(item.request);
      }
      
      // Post-process Transaction update endpoint
      if (item.name === "29. Update Transaction" || 
          (item.name.includes('Update Transaction') && item.request.method === 'PATCH')) {
        customizeTransactionUpdateEndpoint(item.request);
      }
      
      // Post-process Account Balances endpoint
      if (item.name === "30. Get Account Balances" ||
          (item.name.includes('Account Balances') && item.request.method === 'GET')) {
        customizeBalanceEndpoint(item.request);
      }
      
      // Post-process List All Balances endpoint
      if (item.name === "31. List All Balances" ||
          (item.name.includes('List All Balances') && item.request.method === 'GET')) {
        customizeBalanceEndpoint(item.request);
      }
      
      // Post-process Get Balance by ID endpoint
      if (item.name === "32. Get Balance by ID" ||
          (item.name.includes('Get Balance') && item.request.method === 'GET')) {
        customizeBalanceEndpoint(item.request);
      }
      
      // Post-process Balance update endpoint
      if (item.name === "33. Update Balance" || 
          (item.name.includes('Update Balance') && item.request.method === 'PATCH')) {
        customizeBalanceUpdateEndpoint(item.request);
      }
      
      // Post-process List Account Operations endpoint
      if (item.name === "34. List Account Operations" ||
          (item.name.includes('List Account Operations') && item.request.method === 'GET')) {
        customizeOperationEndpoint(item.request);
      }
      
      // Post-process Get Account by Alias endpoint
      if (item.name === "16. Get Account by Alias" ||
          (item.name.includes('Account') && item.name.includes('Alias'))) {
        customizeGetAccountByAliasEndpoint(item.request);
      }
      
      // Special case for creating AssetRate with USD
      if (item.name === "13. Create AssetRate") {
        if (item.request.body && item.request.body.mode === 'raw') {
          try {
            item.request.body.raw = JSON.stringify(endpointExamples.assetRateExample, null, 2);
          } catch (e) {
            console.log("Could not set body for AssetRate");
          }
        }
      }
    }
  }
}

/**
 * Finds a request in the collection that matches a specific path and HTTP method
 * 
 * This function searches through all folders and requests in the collection to find
 * a request that matches the specified path pattern and HTTP method. It handles
 * special cases for different endpoint types and uses regex pattern matching
 * to accommodate variations in path parameter names.
 * 
 * @param {object} collection - The Postman collection to search
 * @param {string} path - The API path to match
 * @param {string} method - The HTTP method to match
 * @returns {object|null} The matching request or null if not found
 */
function findRequestByPathAndMethod(collection, path, method) {
  let result = null;
  console.log(`Searching for endpoint with method=${method}, path=${path}`);
  
  // Convert path to regex pattern by replacing path parameters with regex pattern
  // This makes matching more flexible with different parameter names
  const pathPattern = path
    .replace(/\{[^}]+\}/g, '\\{[^}]+\\}')  // Replace {param} with regex that matches any parameter
    .replace(/\//g, '\\/');                // Escape forward slashes
  
  const pathRegex = new RegExp(pathPattern);
  
  // Special cases
  const isAccountAliasEndpoint = path.includes('/accounts/alias/');
  const isUpdateEndpoint = method === 'PATCH';
  const isUpdateOrganizationEndpoint = isUpdateEndpoint && path.includes('/organizations/{id}');
  const isUpdateLedgerEndpoint = isUpdateEndpoint && path.includes('/ledgers/{id}');
  const isUpdateAccountEndpoint = isUpdateEndpoint && path.includes('/accounts/{id}') && !path.includes('/balances');
  const isUpdatePortfolioEndpoint = isUpdateEndpoint && path.includes('/portfolios/{id}');
  const isUpdateSegmentEndpoint = isUpdateEndpoint && path.includes('/segments/{id}');
  const isUpdateTransactionEndpoint = isUpdateEndpoint && path.includes('/transactions/{transaction_id}');
  const isUpdateBalanceEndpoint = isUpdateEndpoint && path.includes('/balances/{balance_id}');
  
  // Search through all folders
  if (collection.item) {
    for (const folder of collection.item) {
      if (folder.item) {
        for (const request of folder.item) {
          if (request.request && 
              request.request.method === method &&
              request.request.url && 
              request.request.url.raw) {
                
            // Extract the raw URL for easier comparisons
            const rawUrl = request.request.url.raw;
            
            // Check if URL matches the path pattern
            const urlMatches = pathRegex.test(rawUrl);
            
            // Special handling for specific endpoints
            const isAliasEndpoint = isAccountAliasEndpoint && rawUrl.includes('/accounts/alias/');
            
            // Ensure we don't match asset-rates for update endpoints
            let isCorrectUpdateEndpoint = true;
            if (isUpdateEndpoint) {
              if ((isUpdateOrganizationEndpoint && rawUrl.includes('asset-rates')) ||
                  (isUpdateLedgerEndpoint && rawUrl.includes('asset-rates')) ||
                  (isUpdateAccountEndpoint && rawUrl.includes('asset-rates')) ||
                  (isUpdatePortfolioEndpoint && rawUrl.includes('asset-rates')) ||
                  (isUpdateSegmentEndpoint && rawUrl.includes('asset-rates')) ||
                  (isUpdateTransactionEndpoint && rawUrl.includes('asset-rates')) ||
                  (isUpdateBalanceEndpoint && rawUrl.includes('asset-rates'))) {
                isCorrectUpdateEndpoint = false;
              }
            }
            
            // For update endpoints, ensure we have the correct entity pattern
            // For example, a PATCH with /organizations/{id} should not match /organizations/{organization_id}/asset-rates
            if (isUpdateEndpoint) {
              if (isUpdateOrganizationEndpoint && !rawUrl.match(/organizations\/\{[^\/]*\}$/)) {
                isCorrectUpdateEndpoint = false;
              } else if (isUpdateLedgerEndpoint && !rawUrl.match(/ledgers\/\{[^\/]*\}$/)) {
                isCorrectUpdateEndpoint = false;
              } else if (isUpdateAccountEndpoint && !rawUrl.match(/accounts\/\{[^\/]*\}$/)) {
                isCorrectUpdateEndpoint = false;
              } else if (isUpdatePortfolioEndpoint && !rawUrl.match(/portfolios\/\{[^\/]*\}$/)) {
                isCorrectUpdateEndpoint = false;
              } else if (isUpdateSegmentEndpoint && !rawUrl.match(/segments\/\{[^\/]*\}$/)) {
                isCorrectUpdateEndpoint = false;
              } else if (isUpdateTransactionEndpoint && !rawUrl.match(/transactions\/\{[^\/]*\}$/)) {
                isCorrectUpdateEndpoint = false;
              } else if (isUpdateBalanceEndpoint && !rawUrl.match(/balances\/\{[^\/]*\}$/)) {
                isCorrectUpdateEndpoint = false;
              }
            }
            
            if ((urlMatches || isAliasEndpoint) && isCorrectUpdateEndpoint) {
              console.log(`Found matching endpoint: ${request.name}`);
              return request;
            }
          }
        }
      }
    }
  }
  
  console.log(`No matching endpoint found for ${method} ${path}`);
  return result;
}

/**
 * Creates a complete E2E workflow folder from existing requests in the collection
 * 
 * This function:
 * 1. Creates a new "E2E Flow" folder in the collection
 * 2. Finds matching requests for each step in the workflow
 * 3. Clones and renames the requests according to the workflow sequence
 * 4. Adds all requests to the workflow folder in the proper order
 * 
 * @param {object} collection - The Postman collection to process
 * @returns {object} The updated collection with the E2E workflow folder
 */
function createE2EWorkflow(collection) {
  // Safety check
  if (!collection) {
    console.warn("Collection is undefined, skipping E2E workflow creation");
    return {};
  }
  
  // Ensure collection has an item array
  if (!collection.item) {
    collection.item = [];
  }

  // Create a copy of the workflow folder to avoid modifying the original
  let workflowFolderCopy = JSON.parse(JSON.stringify(workflowFolder));
  workflowFolderCopy.item = [];
  
  // Do the actual processing to add requests
  for (const step of workflowSteps) {
    // Find matching request and add to workflow folder
    let matchingRequest;
    
    // For update endpoints with a specified pathPattern, use a more targeted approach
    if (step.pathPattern) {
      // Get all requests that match the operation (e.g., PUT)
      const possibleRequests = [];
      
      // Search through all folders to find requests with matching method
      if (collection.item) {
        for (const folder of collection.item) {
          if (folder.item) {
            for (const request of folder.item) {
              if (request.request && 
                  request.request.method === step.operation &&
                  request.request.url && 
                  request.request.url.raw) {
                possibleRequests.push(request);
              }
            }
          }
        }
      }
      
      // Now find the one that matches the pathPattern
      const pathRegex = new RegExp(step.pathPattern);
      matchingRequest = possibleRequests.find(req => {
        const matches = pathRegex.test(req.request.url.raw);
        if (matches) {
          console.log(`Found match for ${step.pathPattern} in request: ${req.name}`);
        }
        return matches;
      });
      
      if (matchingRequest) {
        console.log(`Found matching request for: ${step.name} using pathPattern: ${step.pathPattern}`);
      }
    } else {
      // Use the standard path-based matching
      matchingRequest = findRequestByPathAndMethod(collection, step.path, step.operation);
    }
    
    if (matchingRequest) {
      console.log(`Found matching request for: ${step.name}`);
      
      // Clone the request to avoid modifying the original
      const clonedRequest = JSON.parse(JSON.stringify(matchingRequest));
      
      // Update name to include step name
      clonedRequest.name = step.name;
      
      // Add to the workflow
      workflowFolderCopy.item.push(clonedRequest);
    } else {
      console.warn(`Warning: Could not find request for workflow step: ${step.name}`);
    }
  }
  
  // Add the workflow folder to the collection
  collection.item.push(workflowFolderCopy);
  
  return collection;
}

// Export functions for use in other modules
module.exports = {
  createE2EWorkflow,
  customizeEndpoints,
  endpointExamples
};

// When this file is run directly, it processes a collection file
if (require.main === module) {
  try {
    // Create the workflow
    collection = createE2EWorkflow(collection);
    
    // Customize the endpoints
    customizeEndpoints(collection.item);

    // Write the updated collection with the workflow folder
    fs.writeFileSync(collectionFile, JSON.stringify(collection, null, 2), 'utf8');
    console.log(`E2E Flow folder added to collection at ${collectionFile}`);
  } catch (error) {
    console.error(`Error processing collection file: ${error.message}`);
    process.exit(1);
  }
}