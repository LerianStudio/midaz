#!/usr/bin/env node

/**
 * This script adds an E2E Flow folder to a Postman collection
 * It implements a comprehensive workflow that tests all API endpoints
 * 
 * Usage: node create-e2e-flow.js <collection-file>
 */

const fs = require('fs');

// Check arguments
const args = process.argv.slice(2);
if (args.length < 1) {
  console.error('Usage: node create-e2e-flow.js <collection-file>');
  process.exit(1);
}

const collectionFile = args[0];

// Check if collection file exists
if (!fs.existsSync(collectionFile)) {
  console.error(`Collection file not found: ${collectionFile}`);
  process.exit(1);
}

// Read the collection file
let collection;
try {
  const fileContent = fs.readFileSync(collectionFile, 'utf8');
  collection = JSON.parse(fileContent);
  
  // Remove any existing E2E Flow folders or duplicates
  if (collection.item) {
    // Filter out any E2E Flow folders
    collection.item = collection.item.filter(item => item.name !== "E2E Flow");
    
    // Also look for and remove duplicate entries with numeric prefixes in the folder names
    for (let i = 0; i < collection.item.length; i++) {
      if (collection.item[i].item) {
        // For each folder, filter out items with numbered names that might be E2E flow duplicates
        collection.item[i].item = collection.item[i].item.filter(item => {
          // Keep items that don't match the pattern "XX. Name" 
          return !(item.name && /^\d+\.\s+/.test(item.name));
        });
      }
    }
  }
} catch (error) {
  console.error(`Error reading/parsing collection file: ${error.message}`);
  process.exit(1);
}

// Defines the complete E2E flow of API requests in a specific order
const workflowSteps = [
  // Onboarding flow
  { operation: "GET", path: "/v1/organizations", name: "1. List Organizations" },
  { operation: "POST", path: "/v1/organizations", name: "2. Create Organization" },
  { operation: "GET", path: "/v1/organizations/{id}", name: "3. Get Organization" },
  { operation: "PUT", path: "/v1/organizations/{id}", name: "4. Update Organization" },
  
  // Ledger flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers", name: "5. List Ledgers" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers", name: "6. Create Ledger" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "7. Get Ledger" },
  { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "8. Update Ledger" },
  
  // Asset flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets", name: "9. List Assets" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets", name: "10. Create USD Asset" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "11. Get Asset" },
  { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "12. Update Asset" },
  
  // Account flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "13. List Accounts" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "14. Create Account" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "15. Get Account" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}", name: "16. Get Account by Alias" },
  { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "17. Update Account" },
  
  // Portfolio flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "18. List Portfolios" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "19. Create Portfolio" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "20. Get Portfolio" },
  { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "21. Update Portfolio" },
  
  // Segment flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "22. List Segments" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "23. Create Segment" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "24. Get Segment" },
  { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "25. Update Segment" },
  
  // Transaction flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions", name: "26. List Transactions" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json", name: "27. Create Transaction using JSON" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "28. Get Transaction" },
  { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "29. Update Transaction" },
  
  // Balance flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances", name: "30. Get Account Balances" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances", name: "31. List All Balances" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "32. Get Balance by ID" },
  { operation: "PUT", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "33. Update Balance" },
  
  // Account-scoped Operations flow (since global operations endpoints don't exist)
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations", name: "34. List Account Operations" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations/{operation_id}", name: "35. Get Account Operation" },
  
  // Additional transaction types
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert", name: "36. Revert Transaction" },
  
  // Delete flow (reverse order of creation to handle dependencies properly)
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}", name: "37. Delete Balance" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "38. Delete Account" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "39. Delete Segment" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "40. Delete Portfolio" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "41. Delete Asset" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "42. Delete Ledger" },
  { operation: "DELETE", path: "/v1/organizations/{id}", name: "43. Delete Organization" }
];

const workflowFolder = {
  name: "E2E Flow",
  description: "Complete workflow that demonstrates the entire API flow from creating an organization to funding an account and cleaning up resources",
  item: []
};

// Endpoint-specific examples and customizations
const endpointExamples = {
  // Transaction JSON example
  transactionJsonExample: {
    "chartOfAccountsGroupName": "PIX_TRANSACTIONS",
    "description": "New Transaction",
    "metadata": {
      "reference": "TRANSACTION-001", 
      "source": "api"
    },
    "send": {
      "asset": "BRL",
      "value": 100,
      "scale": 2,
      "source": {
        "from": [
          {
            "account": "@external/BRL",
            "amount": {
              "asset": "BRL",
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
              "asset": "BRL",
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
  
  // AssetRate example
  assetRateExample: {
    "externalId": "USD-{{$timestamp}}",
    "sourceAssetCode": "USD",
    "rate": 1.0,
    "effectiveDate": new Date().toISOString()
  },
  
  // Simple funding transaction example
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
 * Customize the DSL transaction endpoint
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
 * Customize the Transaction JSON endpoint
 * @param {object} request - The Postman request object to customize
 */
function customizeTransactionJsonEndpoint(request) {
  if (!request || !request.body) return;
  
  console.log('Customizing: Adding complete send example to Transaction JSON endpoint');
  
  // Update the request body with the full example
  if (request.body.mode === 'raw') {
    request.body.raw = JSON.stringify(endpointExamples.transactionJsonExample, null, 2);
  }
}

/**
 * Process all items in a collection recursively and apply endpoint-specific customizations
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
          (item.name.includes('Transaction') && item.name.includes('JSON') && item.request.method === 'POST')) {
        customizeTransactionJsonEndpoint(item.request);
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

// Create E2E workflow from existing requests in the collection
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
  
  // Use the existing function but make it handle null/undefined collection
  let workflowCollection = Object.assign({}, collection);
  
  // Create a copy of the workflow folder to avoid modifying the original
  let workflowFolderCopy = JSON.parse(JSON.stringify(workflowFolder));
  workflowFolderCopy.item = [];
  
  // Do the actual processing to add requests
  for (const step of workflowSteps) {
    // Find matching request and add to workflow folder
    const matchingRequest = findRequestByPathAndMethod(collection, step.path, step.operation);
    if (matchingRequest) {
      console.log(`Found matching request for: ${step.name}`);
      workflowFolderCopy.item.push(matchingRequest);
    } else {
      console.warn(`Warning: Could not find request for workflow step: ${step.name}`);
    }
  }
  
  // Add the workflow folder to the collection
  workflowCollection.item.push(workflowFolderCopy);
  
  return workflowCollection;
}

/**
 * Customize endpoints with specific examples
 */
function customizeEndpoints(items) {
  if (!items) return;
  
  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    
    // Process folders recursively
    if (item.item) {
      customizeEndpoints(item.item);
    }
    
    // Process requests
    if (item.request) {
      // JSON Transaction example
      if (item.name && item.name.includes('Transaction') && 
          item.name.includes('JSON') && 
          item.request.method === 'POST') {
        console.log('Customizing: Transaction JSON example');
        
        if (item.request.body && item.request.body.mode === 'raw') {
          item.request.body.raw = JSON.stringify(endpointExamples.transactionJsonExample, null, 2);
        }
      }
      
      // DSL example
      if (item.name && item.name.includes('DSL') && item.request.method === 'POST') {
        console.log('Customizing: DSL example');
        
        item.request.body = {
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
    }
  }
}

// Export functions for use in other modules
module.exports = {
  createE2EWorkflow,
  customizeEndpoints,
  endpointExamples
};

// When this file is run directly, it processes a collection file
if (require.main === module) {
  // Read the collection file
  let collection;
  try {
    const fileContent = fs.readFileSync(collectionFile, 'utf8');
    collection = JSON.parse(fileContent);
  } catch (error) {
    console.error(`Error reading/parsing collection file: ${error.message}`);
    process.exit(1);
  }

  // Create the workflow using the directly defined function
  collection = createE2EWorkflow(collection);

  // Write the updated collection with the workflow folder
  try {
    if (collection) {
      fs.writeFileSync(collectionFile, JSON.stringify(collection, null, 2), 'utf8');
      console.log(`E2E Flow folder added to collection at ${collectionFile}`);
    } else {
      console.error("Failed to create E2E workflow: collection is undefined");
      process.exit(1);
    }
  } catch (error) {
    console.error(`Error writing collection file: ${error.message}`);
    process.exit(1);
  }
}

// Find and clone the requests for each step in the workflow
workflowSteps.forEach((step, index) => {
  const matchingRequest = findRequestByPathAndMethod(collection, step.path, step.operation);
  
  if (matchingRequest) {
    console.log(`Found matching request for: ${step.name}`);
    
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
    
    // Special case for Get Account by Alias to ensure it uses the accountAlias variable
    if (step.name === "16. Get Account by Alias") {
      if (clonedRequest.request && clonedRequest.request.url) {
        // Make sure we're using the accountAlias variable in the URL
        if (clonedRequest.request.url.path) {
          for (let i = 0; i < clonedRequest.request.url.path.length; i++) {
            if (clonedRequest.request.url.path[i] === "alias" && 
                i+1 < clonedRequest.request.url.path.length && 
                (clonedRequest.request.url.path[i+1] === "{alias}" || clonedRequest.request.url.path[i+1] === "{{alias}}")) {
              clonedRequest.request.url.path[i+1] = "{{accountAlias}}";
            }
          }
        }
        
        // Update the raw URL as well
        if (clonedRequest.request.url.raw) {
          clonedRequest.request.url.raw = clonedRequest.request.url.raw.replace(
            /\{alias\}|\{\{alias\}\}/g, 
            "{{accountAlias}}"
          );
        }
      }
    }
    
    // Fix the Get Portfolio request to use portfolioId instead of ledgerId
    if (step.name === "20. Get Portfolio" || step.name === "21. Update Portfolio") {
      if (clonedRequest.request && clonedRequest.request.url) {
        console.log(`Processing portfolio endpoint: ${step.name}`);
        
        // Fix URL path parameters
        if (clonedRequest.request.url.path) {
          for (let i = 0; i < clonedRequest.request.url.path.length; i++) {
            // Replace the last parameter with portfolioId
            if (i === clonedRequest.request.url.path.length - 1 && 
                (clonedRequest.request.url.path[i] === "{id}" || 
                 clonedRequest.request.url.path[i] === "{{ledgerId}}")) {
              clonedRequest.request.url.path[i] = "{{portfolioId}}";
            }
          }
        }
        
        // Update the raw URL as well
        if (clonedRequest.request.url.raw) {
          clonedRequest.request.url.raw = clonedRequest.request.url.raw.replace(
            /\/portfolios\/\{id\}$|\/portfolios\/\{\{ledgerId\}\}$/,
            "/portfolios/{{portfolioId}}"
          );
        }
        
        // Update variables if they exist
        if (clonedRequest.request.url.variable) {
          for (let i = 0; i < clonedRequest.request.url.variable.length; i++) {
            if (clonedRequest.request.url.variable[i].key === "id") {
              clonedRequest.request.url.variable[i].value = "{{portfolioId}}";
            }
          }
        }
      }
    }
    
    // Fix the Get Segment request to use segmentId instead of ledgerId
    if (step.name === "24. Get Segment" || step.name === "25. Update Segment") {
      if (clonedRequest.request && clonedRequest.request.url) {
        console.log(`Processing segment endpoint: ${step.name}`);
        
        // Fix URL path parameters
        if (clonedRequest.request.url.path) {
          for (let i = 0; i < clonedRequest.request.url.path.length; i++) {
            // Replace the last parameter with segmentId
            if (i === clonedRequest.request.url.path.length - 1 && 
                (clonedRequest.request.url.path[i] === "{id}" || 
                 clonedRequest.request.url.path[i] === "{{ledgerId}}")) {
              clonedRequest.request.url.path[i] = "{{segmentId}}";
            }
          }
        }
        
        // Update the raw URL as well
        if (clonedRequest.request.url.raw) {
          clonedRequest.request.url.raw = clonedRequest.request.url.raw.replace(
            /\/segments\/\{id\}$|\/segments\/\{\{ledgerId\}\}$/,
            "/segments/{{segmentId}}"
          );
        }
        
        // Update variables if they exist
        if (clonedRequest.request.url.variable) {
          for (let i = 0; i < clonedRequest.request.url.variable.length; i++) {
            if (clonedRequest.request.url.variable[i].key === "id") {
              clonedRequest.request.url.variable[i].value = "{{segmentId}}";
            }
          }
        }
      }
    }
    
    // Special case for transaction endpoints using transaction_id
    if (step.name === "28. Get Transaction" || step.name === "29. Update Transaction" || 
        step.name === "36. Revert Transaction") {
      if (clonedRequest.request && clonedRequest.request.url) {
        console.log(`Processing transaction endpoint: ${step.name}`);
        
        // Fix URL path parameters
        if (clonedRequest.request.url.path) {
          for (let i = 0; i < clonedRequest.request.url.path.length; i++) {
            // Look for the transaction_id in the path
            if (clonedRequest.request.url.path[i] === "{transaction_id}") {
              clonedRequest.request.url.path[i] = "{{transactionId}}";
              console.log("  Fixed transaction_id param in URL path");
            }
          }
          
          // Special handling for Update Transaction endpoint to ensure it uses the correct URL
          if (step.name === "29. Update Transaction") {
            // Check if the URL incorrectly includes operations/
            const operationsIndex = clonedRequest.request.url.path.indexOf("operations");
            if (operationsIndex !== -1) {
              // Remove "operations" and any path elements after it
              clonedRequest.request.url.path.splice(operationsIndex);
              console.log("  Fixed Update Transaction URL by removing operations/ path");
              
              // Update the raw URL as well
              if (clonedRequest.request.url.raw) {
                clonedRequest.request.url.raw = clonedRequest.request.url.raw.replace(
                  /\/operations\/.*$/,
                  ""
                );
              }
            }
          }
        }
        
        // Fix URL variables
        if (clonedRequest.request.url.variable) {
          for (let i = 0; i < clonedRequest.request.url.variable.length; i++) {
            if (clonedRequest.request.url.variable[i].key === "transaction_id") {
              clonedRequest.request.url.variable[i].value = "{{transactionId}}";
              console.log("  Fixed transaction_id in URL variables");
            }
          }
        }
      }
    }
    
    // Special case for balance endpoints using balance_id
    if (step.name === "32. Get Balance by ID" || step.name === "33. Update Balance" || 
        step.name === "37. Delete Balance") {
      if (clonedRequest.request && clonedRequest.request.url) {
        console.log(`Processing balance endpoint: ${step.name}`);
        
        // Ensure URL actually includes balance_id parameter when needed
        if (clonedRequest.request.url.path) {
          // Get the path array
          const pathParts = clonedRequest.request.url.path;
          
          // Check if the URL ends with "balances/" without a balance_id
          if (pathParts.length > 0 && pathParts[pathParts.length - 1] === "balances") {
            // Add the balanceId parameter to the path
            pathParts.push("{{balanceId}}");
            console.log("  Added missing balanceId parameter to URL path");
            
            // Update the raw URL as well
            if (clonedRequest.request.url.raw) {
              clonedRequest.request.url.raw = clonedRequest.request.url.raw.replace(
                /\/balances\/$/,
                "/balances/{{balanceId}}"
              );
              
              // If the URL doesn't end with a slash, add the parameter
              if (!clonedRequest.request.url.raw.endsWith("/")) {
                clonedRequest.request.url.raw = clonedRequest.request.url.raw + "/{{balanceId}}";
              }
            }
            
            // Add the variable definition if it doesn't exist
            if (!clonedRequest.request.url.variable) {
              clonedRequest.request.url.variable = [];
            }
            
            // Add the balance_id variable if it doesn't exist
            let hasBalanceIdVar = false;
            for (const v of clonedRequest.request.url.variable) {
              if (v.key === "balance_id") {
                v.value = "{{balanceId}}";
                hasBalanceIdVar = true;
                break;
              }
            }
            
            if (!hasBalanceIdVar) {
              clonedRequest.request.url.variable.push({
                key: "balance_id",
                value: "{{balanceId}}",
                description: "Balance ID"
              });
            }
          }
          
          // Fix any existing balance_id parameters
          for (let i = 0; i < pathParts.length; i++) {
            // Look for the balance_id in the path
            if (pathParts[i] === "{balance_id}") {
              pathParts[i] = "{{balanceId}}";
              console.log("  Fixed balance_id param in URL path");
            }
          }
        }
        
        // Fix URL variables
        if (clonedRequest.request.url.variable) {
          for (let i = 0; i < clonedRequest.request.url.variable.length; i++) {
            if (clonedRequest.request.url.variable[i].key === "balance_id") {
              clonedRequest.request.url.variable[i].value = "{{balanceId}}";
              console.log("  Fixed balance_id in URL variables");
            }
          }
        }
      }
    }
    
    // Special case for account operations endpoints
    if (step.name === "34. List Account Operations" || step.name === "35. Get Account Operation") {
      if (clonedRequest.request && clonedRequest.request.url) {
        console.log(`Processing account operations endpoint: ${step.name}`);
        
        // Fix URL path parameters
        if (clonedRequest.request.url.path) {
          for (let i = 0; i < clonedRequest.request.url.path.length; i++) {
            // Look for the account_id in the path
            if (clonedRequest.request.url.path[i] === "{account_id}") {
              clonedRequest.request.url.path[i] = "{{accountId}}";
              console.log("  Fixed account_id param in URL path");
            }
            
            // If this is the specific operation endpoint, handle operation_id too
            if (step.name === "35. Get Account Operation" && 
                clonedRequest.request.url.path[i] === "{operation_id}") {
              clonedRequest.request.url.path[i] = "{{operationId}}";
              console.log("  Fixed operation_id param in URL path");
            }
          }
        }
        
        // Fix URL variables
        if (clonedRequest.request.url.variable) {
          for (let i = 0; i < clonedRequest.request.url.variable.length; i++) {
            if (clonedRequest.request.url.variable[i].key === "account_id") {
              clonedRequest.request.url.variable[i].value = "{{accountId}}";
              console.log("  Fixed account_id in URL variables");
            }
            
            if (step.name === "35. Get Account Operation" && 
                clonedRequest.request.url.variable[i].key === "operation_id") {
              clonedRequest.request.url.variable[i].value = "{{operationId}}";
              console.log("  Fixed operation_id in URL variables");
            }
          }
        }
      }
    }
    
    // Special case for portfolio and segment deletion endpoints
    if (step.name === "39. Delete Segment" || step.name === "40. Delete Portfolio") {
      if (clonedRequest.request && clonedRequest.request.url) {
        console.log(`Processing ${step.name === "39. Delete Segment" ? "segment" : "portfolio"} deletion endpoint: ${step.name}`);
        
        // Fix URL path parameters
        if (clonedRequest.request.url.path) {
          for (let i = 0; i < clonedRequest.request.url.path.length; i++) {
            // Look for the last id parameter in the path and replace with the correct ID
            if (i === clonedRequest.request.url.path.length - 1 && 
                (clonedRequest.request.url.path[i] === "{id}" || 
                 clonedRequest.request.url.path[i] === "{{ledgerId}}")) {
              
              if (step.name === "39. Delete Segment") {
                clonedRequest.request.url.path[i] = "{{segmentId}}";
                console.log("  Fixed segment_id param in URL path");
              } else if (step.name === "40. Delete Portfolio") {
                clonedRequest.request.url.path[i] = "{{portfolioId}}";
                console.log("  Fixed portfolio_id param in URL path");
              }
            }
          }
        }
        
        // Fix URL variables if they exist
        if (clonedRequest.request.url.variable) {
          for (let i = 0; i < clonedRequest.request.url.variable.length; i++) {
            if (clonedRequest.request.url.variable[i].key === "id") {
              if (step.name === "39. Delete Segment") {
                clonedRequest.request.url.variable[i].value = "{{segmentId}}";
                console.log("  Fixed segment_id in URL variables");
              } else if (step.name === "40. Delete Portfolio") {
                clonedRequest.request.url.variable[i].value = "{{portfolioId}}";
                console.log("  Fixed portfolio_id in URL variables");
              }
            }
          }
        }
      }
    }

    
    // Special case for create account to fix parent account ID issue
    if (step.name === "14. Create Account") {
      if (clonedRequest.request && clonedRequest.request.body) {
        try {
          const bodyObj = JSON.parse(clonedRequest.request.body.raw);
          // Remove parentAccountId if it's set to all zeros
          if (bodyObj.parentAccountId === "00000000-0000-0000-0000-000000000000") {
            delete bodyObj.parentAccountId;
          }
          // If portfolioId and segmentId are zeros, remove them too
          if (bodyObj.portfolioId === "00000000-0000-0000-0000-000000000000") {
            delete bodyObj.portfolioId;
          }
          if (bodyObj.segmentId === "00000000-0000-0000-0000-000000000000") {
            delete bodyObj.segmentId;
          }
          clonedRequest.request.body.raw = JSON.stringify(bodyObj, null, 2);
        } catch (e) {
          console.log("Could not parse body for Account");
        }
      }
    }
    
    // Special case for update account to fix portfolio ID issue
    if (step.name === "17. Update Account") {
      if (clonedRequest.request && clonedRequest.request.body) {
        try {
          const bodyObj = JSON.parse(clonedRequest.request.body.raw);
          // Remove portfolioId and segmentId to avoid validation errors
          if (bodyObj.portfolioId) {
            delete bodyObj.portfolioId;
          }
          if (bodyObj.segmentId) {
            delete bodyObj.segmentId;
          }
          // Make sure we only include valid fields for update
          const validUpdateFields = ["name", "alias", "status", "metadata"];
          const updatedBody = {};
          validUpdateFields.forEach(field => {
            if (bodyObj[field]) {
              updatedBody[field] = bodyObj[field];
            }
          });
          clonedRequest.request.body.raw = JSON.stringify(updatedBody, null, 2);
        } catch (e) {
          console.log("Could not parse body for Update Account");
        }
      }
    }
    
    // Special case for create portfolio with relevant values
    if (step.name === "19. Create Portfolio") {
      if (clonedRequest.request && clonedRequest.request.body) {
        try {
          const bodyObj = JSON.parse(clonedRequest.request.body.raw);
          // Keep only the fields that are expected by the API
          const validPortfolioFields = ["name", "metadata"];
          const updatedBody = {
            name: "Test Portfolio"
          };
          // Retain original metadata if present
          if (bodyObj.metadata) {
            updatedBody.metadata = bodyObj.metadata;
          }
          clonedRequest.request.body.raw = JSON.stringify(updatedBody, null, 2);
        } catch (e) {
          console.log("Could not parse body for Portfolio");
        }
      }
    }

    // Special case for create segment with relevant values
    if (step.name === "23. Create Segment") {
      if (clonedRequest.request && clonedRequest.request.body) {
        try {
          const bodyObj = JSON.parse(clonedRequest.request.body.raw);
          // Keep only the fields that are expected by the API
          const updatedBody = {
            name: "Test Segment"
          };
          // Retain original metadata if present
          if (bodyObj.metadata) {
            updatedBody.metadata = bodyObj.metadata;
          }
          clonedRequest.request.body.raw = JSON.stringify(updatedBody, null, 2);
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
        
        // Custom status code test based on operation type
        if (testScript.includes("pm.test(\"Status code is successful\"")) {
          // Replace the generic status code test with a more specific one
          const statusCodeTest = (() => {
            if (step.operation === "GET") {
              return `pm.test("Status code is 200 OK", function () {
  pm.expect(pm.response.code).to.equal(200);
});`;
            } else if (step.operation === "POST") {
              if (step.path.includes("/revert") || step.path.includes("/commit")) {
                return `pm.test("Status code is 200 or 201", function () {
  pm.expect(pm.response.code).to.be.oneOf([200, 201]);
});`;
              } else {
                return `pm.test("Status code is 200 or 201", function () {
  pm.expect(pm.response.code).to.be.oneOf([200, 201]);
});`;
              }
            } else if (step.operation === "PUT") {
              return `pm.test("Status code is 200 OK", function () {
  pm.expect(pm.response.code).to.equal(200);
});`;
            } else if (step.operation === "DELETE") {
              return `pm.test("Status code is 204 No Content", function () {
  pm.expect(pm.response.code).to.equal(204);
});`;
            } else {
              // Fallback to original test
              return `pm.test("Status code is successful", function () {
  pm.expect(pm.response.code).to.be.oneOf([200, 201, 202, 204]);
});`;
            }
          })();
          
          // Replace the generic status code test
          testScript = testScript.replace(/pm\.test\("Status code is successful"[^}]+}\);/, statusCodeTest);
        }
        
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
        } else if (step.name === "14. Create Account") {
          testScript += `
// Save the created account ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("accountId", jsonData.id);
    console.log("accountId set to: " + jsonData.id);
  }
  
  // Also save the alias for "Get Account by Alias" step
  if (jsonData && jsonData.alias) {
    pm.environment.set("accountAlias", jsonData.alias);
    console.log("accountAlias set to: " + jsonData.alias);
  }
} catch (error) {
  console.error("Failed to extract accountId: ", error);
}`;
        } else if (step.name === "19. Create Portfolio") {
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
        } else if (step.name === "23. Create Segment") {
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
        } else if (step.name === "27. Create Transaction using JSON") {
          testScript += `
// Save the created transaction ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("transactionId", jsonData.id);
    // Use only camelCase for consistency
    console.log("transactionId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract transactionId: ", error);
}`;
        } else if (step.name === "15. Get Account") {
          testScript += `
// Save the account alias to use in subsequent requests, particularly for Get Account by Alias
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.alias) {
    pm.environment.set("accountAlias", jsonData.alias);
    console.log("accountAlias set to: " + jsonData.alias);
  }
} catch (error) {
  console.error("Failed to extract account alias: ", error);
}`;
        } else if (step.name === "31. List All Balances") {
          testScript += `
// Save the first balance ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && Array.isArray(jsonData.items) && jsonData.items.length > 0 && jsonData.items[0].id) {
    pm.environment.set("balanceId", jsonData.items[0].id);
    console.log("balanceId set to: " + jsonData.items[0].id);
  }
} catch (error) {
  console.error("Failed to extract balanceId: ", error);
}`;
        } else if (step.name === "32. Get Balance by ID") {
          testScript += `
// Confirm that we have a balance with this ID
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("balanceId", jsonData.id);
    // Use only camelCase for consistency
    console.log("balanceId confirmed: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract balanceId: ", error);
}`;
        } else if (step.name === "34. List Account Operations") {
          testScript += `
// Save the first operation ID to use in subsequent requests, if needed
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.items && jsonData.items.length > 0 && jsonData.items[0].id) {
    pm.environment.set("operationId", jsonData.items[0].id);
    // Use only camelCase for consistency
    console.log("operationId set to: " + jsonData.items[0].id);
  }
} catch (error) {
  console.error("Failed to extract operationId: ", error);
}`;
        } else if (step.name === "35. Get Account Operation") {
          testScript += `
// Save the operation ID to use in subsequent requests
try {
  var jsonData = pm.response.json();
  if (jsonData && jsonData.id) {
    pm.environment.set("operationId", jsonData.id);
    // Use only camelCase for consistency
    console.log("operationId set to: " + jsonData.id);
  }
} catch (error) {
  console.error("Failed to extract operationId: ", error);
}`;
        }
        
        // Add execution control for workflow
        if (index < workflowSteps.length - 1) {
          // Set next request if not the last step
          testScript += `\n\n// Set next request in workflow\npm.execution.setNextRequest("${workflowSteps[index + 1].name}");`;
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
if (!collection.item) {
  collection.item = [];
}
collection.item.push(workflowFolder);