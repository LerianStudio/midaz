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
} catch (error) {
  console.error(`Error reading/parsing collection file: ${error.message}`);
  process.exit(1);
}

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
  
  // Account flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "13. List Accounts" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts", name: "14. Create Account" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "15. Get Account" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}", name: "16. Get Account by Alias" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "17. Update Account" },
  
  // Portfolio flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "18. List Portfolios" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", name: "19. Create Portfolio" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "20. Get Portfolio" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "21. Update Portfolio" },
  
  // Segment flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "22. List Segments" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments", name: "23. Create Segment" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "24. Get Segment" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "25. Update Segment" },
  
  // Transaction flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions", name: "26. List Transactions" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json", name: "27. Create Transaction using JSON" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "28. Get Transaction" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{id}", name: "29. Update Transaction" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit", name: "30. Commit Transaction" },
  
  // Balance flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances", name: "31. Get Account Balances" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances", name: "32. List All Balances" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "33. Get Balance by ID" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "34. Update Balance" },
  
  // Operation flow
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/operations", name: "35. List Operations" },
  { operation: "GET", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/operations/{id}", name: "36. Get Operation" },
  { operation: "PATCH", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/operations/{id}", name: "37. Update Operation" },
  
  // Additional transaction types
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/templates", name: "38. Create Transaction Template" },
  { operation: "POST", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert", name: "39. Revert Transaction" },
  
  // Delete flow (reverse order of creation to handle dependencies properly)
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{id}", name: "40. Delete Balance" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}", name: "41. Delete Account" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}", name: "42. Delete Segment" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", name: "43. Delete Portfolio" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}", name: "44. Delete Asset" },
  { operation: "DELETE", path: "/v1/organizations/{organization_id}/ledgers/{id}", name: "45. Delete Ledger" },
  { operation: "DELETE", path: "/v1/organizations/{id}", name: "46. Delete Organization" }
];

const workflowFolder = {
  name: "E2E Flow",
  description: "Complete workflow that demonstrates the entire API flow from creating an organization to funding an account and cleaning up resources",
  item: []
};

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

// Find and clone the requests for each step in the workflow
workflowSequence.forEach((step, index) => {
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
    
    // Special case for creating a transaction to fund account
    if (step.name === "27. Create Transaction using JSON") {
      if (clonedRequest.request && clonedRequest.request.body) {
        // Set transaction body for funding from external source using the correct schema
        const fundingTxBody = {
          "chartOfAccountsGroupName": "PIX_TRANSACTIONS",
          "description": "Initial funding from external source",
          "metadata": {
            "reference": "FUNDING-001",
            "source": "e2e-test"
          },
          "send": {
            "asset": "USD",
            "value": 1000,
            "scale": 2,
            "source": {
              "from": [
                {
                  "account": "@external/USD",
                  "amount": {
                    "asset": "USD",
                    "value": 1000,
                    "scale": 2
                  },
                  "description": "Debit Operation - External Funding",
                  "chartOfAccounts": "EXTERNAL_DEBIT",
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
                    "value": 1000,
                    "scale": 2
                  },
                  "description": "Credit Operation - Account Funding",
                  "chartOfAccounts": "ACCOUNT_CREDIT",
                  "metadata": {
                    "operation": "funding",
                    "type": "account"
                  }
                }
              ]
            }
          }
        };
        clonedRequest.request.body.raw = JSON.stringify(fundingTxBody, null, 2);
        
        // Add a timeout settings to the request
        if (!clonedRequest.request.timeout) {
          clonedRequest.request.timeout = 60000; // 60 seconds timeout
        }
      }
      
      // Add pre-request script to validate input values before sending
      if (clonedRequest.event) {
        let hasPrerequest = false;
        for (const event of clonedRequest.event) {
          if (event.listen === "prerequest") {
            hasPrerequest = true;
            let script = event.script.exec.join("\n");
            
            // Add validation to ensure accountId is available
            script += `
// Validate that accountId is set before sending
if (!pm.environment.get("accountId")) {
  console.error("ERROR: accountId is not set in the environment. This request will fail.");
}

// Set a reasonable timeout for this request
pm.request.timeout = 60000; // 60 seconds
`;
            event.script.exec = script.split("\n");
            break;
          }
        }
        
        // If no prerequest script exists, create one
        if (!hasPrerequest) {
          clonedRequest.event.push({
            listen: "prerequest",
            script: {
              type: "text/javascript",
              exec: [
                "// Validate that accountId is set before sending",
                "if (!pm.environment.get(\"accountId\")) {",
                "  console.error(\"ERROR: accountId is not set in the environment. This request will fail.\");",
                "}",
                "",
                "// Set a reasonable timeout for this request",
                "pm.request.timeout = 60000; // 60 seconds"
              ]
            }
          });
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
        } else if (step.name === "33. Get Balance by ID") {
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
        } else if (step.name === "36. Get Operation") {
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

// Write the modified collection back to the file
try {
  fs.writeFileSync(collectionFile, JSON.stringify(collection, null, 2));
  console.log(`E2E Flow folder added to collection at ${collectionFile}`);
} catch (error) {
  console.error(`Error writing collection file: ${error.message}`);
  process.exit(1);
}