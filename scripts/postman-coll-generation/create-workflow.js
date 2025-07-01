#!/usr/bin/env node

/**
 * Midaz API Workflow Generator (Simplified)
 *
 * This script creates a Postman collection folder with the workflow sequence
 * from the WORKFLOW.md file. It finds requests in the main collection
 * based on method and path, and copies them directly.
 *
 * Usage: node create-workflow.js <input-collection> <workflow-md> <output-collection>
 *
 * Example: node create-workflow.js ./postman/MIDAZ.postman_collection.json ./postman/WORKFLOW.md ./postman/MIDAZ.postman_collection.json
 */

const fs = require('fs');
const path = require('path');
const { v4: uuidv4 } = require('uuid');

// Import enhanced workflow features for CI regression testing
const {
  generateEnhancedTestScript,
  generateEnhancedPreRequestScript,
  generateWorkflowSummaryScript
} = require('./enhance-tests.js');

// --- Utility Functions ---

// Read JSON file
function readJsonFile(filePath) {
  try {
    const data = fs.readFileSync(filePath, 'utf8');
    return JSON.parse(data);
  } catch (error) {
    console.error(`Error reading JSON file ${filePath}: ${error.message}`);
    process.exit(1);
  }
}

// Read file content
function readFile(filePath) {
  try {
    return fs.readFileSync(filePath, 'utf8');
  } catch (error) {
    console.error(`Error reading file ${filePath}: ${error.message}`);
    process.exit(1);
  }
}

// Write JSON file
function writeJsonFile(filePath, data) {
  try {
    fs.writeFileSync(filePath, JSON.stringify(data, null, 2));
    console.log(`Successfully wrote JSON to ${filePath}`);
  } catch (error) {
    console.error(`Error writing JSON file ${filePath}: ${error.message}`);
    process.exit(1);
  }
}

// Deep clone an object
function deepClone(obj) {
  return JSON.parse(JSON.stringify(obj));
}

// Extract path from URL object or raw string
function getPathFromUrl(urlObject) {
    if (!urlObject || !urlObject.path) {
        return '';
    }
    if (!Array.isArray(urlObject.path)) {
         return '';
    }
    const joinedPath = '/' + urlObject.path.join('/');
    return joinedPath;
}

// Utility function to normalize a path string
function normalizePath(pathStr) {
    if (!pathStr) return '';
    let normalized = pathStr.trim();
    if (normalized.endsWith('/')) {
        normalized = normalized.slice(0, -1);
    }
    if (!normalized.startsWith('/')) {
        normalized = '/' + normalized;
    }
    // Replace Postman-style {{variables}} first
    normalized = normalized.replace(/\{\{[^}]+\}\}/g, '{}');
    // Then replace standard {parameters}
    normalized = normalized.replace(/\{[^}]+\}/g, '{}');

    return normalized;
}

// --- Workflow Parsing ---

function parseWorkflowStepsFromMarkdown(markdown) {
  const lines = markdown.split('\n');
  const steps = [];
  let currentStep = null;
  let stepNumber = 0;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();

    // Match step title: "1. **Create Organization**"
    const titleMatch = line.match(/^(\d+)\.\s+\*\*(.+)\*\*/);
    if (titleMatch) {
      stepNumber = parseInt(titleMatch[1], 10);
      currentStep = {
        number: stepNumber,
        title: titleMatch[2].trim(),
        method: '',
        path: '',
        description: '',
        uses: [],
        outputs: []
      };
      
      // Debug for Zero Out Balance step
      if (titleMatch[2].trim() === "Zero Out Balance") {
        console.log(`DEBUG: Found Zero Out Balance step in markdown: number=${stepNumber}, title="${titleMatch[2].trim()}"`);
      }
      
      steps.push(currentStep);
    }
    // Match method and path: "- `POST /v1/organizations`"
    else if (currentStep && line.startsWith('- `')) {
        const endpointMatch = line.match(/- `([A-Z]+)\s+([^`]+)`/);
        if (endpointMatch) {
            currentStep.method = endpointMatch[1];
            currentStep.path = endpointMatch[2];
        }
    }
    // Match description: "- Creates a new organization..." (ensure it's not Uses/Output)
    else if (currentStep && line.startsWith('- ') && !line.includes('**Uses:**') && !line.includes('**Output')) {
        currentStep.description = line.substring(2).trim();
    }
    // Match Uses section
    else if (currentStep && line.includes('**Uses:**')) {
        // Check if uses is on the same line - handle multiple uses separated by commas
        const inlineUsesMatch = line.match(/\*\*Uses:\*\*\s*(.+)/);
        if (inlineUsesMatch) {
            const usesText = inlineUsesMatch[1];
            // Match all `variable` from step X patterns and extract them
            const useMatches = usesText.match(/`([^`]+)`\s+from\s+step\s+(\d+)/g);
            if (useMatches) {
                useMatches.forEach(match => {
                    const useMatch = match.match(/`([^`]+)`\s+from\s+step\s+(\d+)/);
                    if (useMatch) {
                        currentStep.uses.push({ variable: useMatch[1], step: parseInt(useMatch[2]) });
                    }
                });
            }
        } else {
            // Check following lines for uses
            let j = i + 1;
            while (j < lines.length && lines[j].trim().startsWith('- `')) {
                const useMatch = lines[j].match(/- `([^`]+)` from step (\d+)/);
                if (useMatch) {
                    currentStep.uses.push({ variable: useMatch[1], step: parseInt(useMatch[2]) });
                }
                j++;
            }
            i = j - 1; // Adjust outer loop index
        }
    }
    // Match Output section
    else if (currentStep && (line.includes('**Output:**') || line.includes('**Outputs:**'))) {
        // Check if output is on the same line - handle multiple outputs separated by commas
        const inlineOutputMatch = line.match(/\*\*Outputs?:\*\*\s*(.+)/);
        if (inlineOutputMatch) {
            const outputsText = inlineOutputMatch[1];
            // Match all `variable` patterns and extract them
            const outputMatches = outputsText.match(/`([^`]+)`/g);
            if (outputMatches) {
                outputMatches.forEach(match => {
                    const variable = match.replace(/`/g, '');
                    currentStep.outputs.push(variable);
                });
            }
        } else {
            // Check following lines for outputs
            let j = i + 1;
            while (j < lines.length && lines[j].trim().startsWith('- `')) {
                const outputMatch = lines[j].match(/- `([^`]+)`/);
                if (outputMatch) {
                    currentStep.outputs.push(outputMatch[1]);
                }
                j++;
            }
            i = j - 1; // Adjust outer loop index
        }
    }
  }
  console.log(`Parsed ${steps.length} workflow steps from Markdown.`);
  return steps;
}

// --- Request Finding ---

// Recursive function to find a request matching method and path
function findRequestRecursive(items, targetMethod, targetPath) {
    // Normalize target path once before the loop
    const normalizedTargetPath = normalizePath(targetPath);
    
    // Create alternative paths for common discrepancies
    const alternativePaths = [];
    
    // Handle /organizations/{}/accounts/{} vs /organizations/{}/ledgers/{}/accounts/{}
    if (normalizedTargetPath.includes('/organizations/{}/accounts/')) {
        const altPath = normalizedTargetPath.replace('/organizations/{}/accounts/', '/organizations/{}/ledgers/{}/accounts/');
        alternativePaths.push(altPath);
    }
    
    // Handle /organizations/{}/balances vs /organizations/{}/ledgers/{}/balances
    if (normalizedTargetPath.includes('/organizations/{}/balances')) {
        const altPath = normalizedTargetPath.replace('/organizations/{}/balances', '/organizations/{}/ledgers/{}/balances');
        alternativePaths.push(altPath);
    }
    
    // Handle /organizations/{}/asset-rates vs /organizations/{}/ledgers/{}/asset-rates
    if (normalizedTargetPath.includes('/organizations/{}/ledgers/{}/asset-rates')) {
        const altPath = normalizedTargetPath.replace('/organizations/{}/ledgers/{}/asset-rates', '/organizations/{}/ledgers/{}/asset-rates');
        alternativePaths.push(altPath);
    }
    
    // Handle operations path variations
    if (normalizedTargetPath.includes('/operations/')) {
        // Try with ledgers in the path
        const altPath = normalizedTargetPath.replace('/organizations/{}/operations/', '/organizations/{}/ledgers/{}/operations/');
        alternativePaths.push(altPath);
        
        // Try with accounts in the path
        const altPath2 = normalizedTargetPath.replace('/organizations/{}/operations/', '/organizations/{}/ledgers/{}/accounts/{}/operations/');
        alternativePaths.push(altPath2);
    }

    for (const item of items) {
        // If it's a request, check if it matches
        if (item.request) {
            const requestMethod = item.request.method;
            // Call getPathFromUrl 
            const requestPath = getPathFromUrl(item.request.url);
            // Call normalizePath 
            const normalizedRequestPath = normalizePath(requestPath);

            // Log the final comparison details
            console.log(`  Comparing: Target[${targetMethod} ${normalizedTargetPath}] vs Collection[${requestMethod} ${normalizedRequestPath}] (Name: ${item.name})`);

            // Check primary path
            if (requestMethod === targetMethod && normalizedRequestPath === normalizedTargetPath) {
                console.log(`    Match found for ${item.name}!`); // Log match success
                return item; // Return the found item (request holder)
            }
            
            // Check alternative paths
            for (const altPath of alternativePaths) {
                if (requestMethod === targetMethod && normalizedRequestPath === altPath) {
                    console.log(`    Match found for ${item.name} using alternative path!`);
                    console.log(`    Alternative path used: ${altPath}`);
                    return item;
                }
            }
        }
        // If it's a folder, search recursively within its items
        if (item.item && item.item.length > 0) {
            const found = findRequestRecursive(item.item, targetMethod, targetPath);
            if (found) {
                return found; // Return the found item from recursion
            }
        }
    }
    return null; // Not found in this branch
}


// --- Test Script Generation ---

// Enhanced test script generation for CI regression testing
function addOrUpdateTestScript(workflowItem, outputs, stepNumber, stepTitle, operation, path, method, requires = []) {
    // Find or create test event
    let testEvent = null;
    if (!workflowItem.event) {
        workflowItem.event = [];
    }
    
    testEvent = workflowItem.event.find(e => e.listen === 'test');
    if (!testEvent) {
        testEvent = {
            listen: 'test',
            script: {
                id: uuidv4(),
                exec: [],
                type: 'text/javascript'
            }
        };
        workflowItem.event.push(testEvent);
    }
    
    // Ensure exec is an array
    if (!Array.isArray(testEvent.script.exec)) {
        testEvent.script.exec = [];
    }
    
    // Generate comprehensive test script for CI regression testing
    const enhancedScript = generateEnhancedTestScript(
        operation, 
        path, 
        method, 
        outputs, 
        stepNumber, 
        stepTitle
    );
    
    // Replace the exec array with enhanced script
    testEvent.script.exec = [enhancedScript];
    
    // Add or update pre-request script
    let preRequestEvent = workflowItem.event.find(e => e.listen === 'prerequest');
    if (!preRequestEvent) {
        preRequestEvent = {
            listen: 'prerequest',
            script: {
                id: uuidv4(),
                exec: [],
                type: 'text/javascript'
            }
        };
        workflowItem.event.push(preRequestEvent);
    }
    
    // Generate enhanced pre-request script
    const enhancedPreScript = generateEnhancedPreRequestScript(
        operation,
        path,
        method,
        stepNumber,
        requires
    );
    
    // Replace the exec array with enhanced pre-request script
    preRequestEvent.script.exec = [enhancedPreScript];
}

// --- Workflow Folder Creation ---

function createWorkflowFolder(collection, steps) {
    const workflowFolder = {
        name: "Complete API Workflow",
        description: "A sequence of API calls representing a typical workflow, generated from WORKFLOW.md.",
        item: [],
        _postman_id: uuidv4(),
        event: [] // Folder-level events if needed later
    };

    let notFoundCount = 0;
    const missingSteps = []; // Array to collect missing steps

    console.log("DEBUG: Starting to process steps for workflow folder creation");
    
    steps.forEach((step, index) => {
        console.log(`Processing Step ${index + 1}: ${step.title} (${step.method} ${step.path})`);
        
        // Debug logging for Zero Out Balance step
        if (step.title === "Zero Out Balance") {
            console.log(`DEBUG: Found Zero Out Balance step in workflow creation: number=${step.number}, title="${step.title}"`);
        }

        // Find the original request in the main collection
        // Exclude the workflow folder itself from the search
        const collectionItemsToSearch = collection.item.filter(item => item.name !== workflowFolder.name);
        const originalItem = findRequestRecursive(collectionItemsToSearch, step.method, step.path);

        if (originalItem) {
            // Clone the found item to avoid modifying the original
            const workflowItem = deepClone(originalItem);

            // Update the name for the workflow step
            workflowItem.name = `${step.number}. ${step.title}`;

            // Add/Update description from Markdown
            let markdownDesc = `**Workflow Step ${step.number}: ${step.title}**\n\n${step.description || 'No description provided in Markdown.'}`;
            
            // Add "Uses" section if applicable
            if (step.uses && step.uses.length > 0) {
                markdownDesc += '\n\n---\n\n**Uses:**\n';
                step.uses.forEach(use => {
                    markdownDesc += `- \`${use}\`\n`;
                });
            }
            
            // Set the description
            workflowItem.request.description = markdownDesc;

            // Update path parameters in the URL
            let newPath = null;
            if (workflowItem.request.url && workflowItem.request.url.path) {
                // Rebuild the path array from the workflow step path to ensure correct structure
                const expectedPath = step.path.replace(/^\//, '').split('/');
                newPath = expectedPath.map(segment => {
                    // Replace path parameters with Postman environment variables
                    if (segment === '{organizationId}' || segment === '{organization_id}') {
                        return '{{organizationId}}';
                    } else if (segment === '{ledgerId}' || segment === '{ledger_id}') {
                        return '{{ledgerId}}';
                    } else if (segment === '{accountId}' || segment === '{account_id}') {
                        return '{{accountId}}';
                    } else if (segment === '{assetId}' || segment === '{asset_id}') {
                        return '{{assetId}}';
                    } else if (segment === '{balanceId}' || segment === '{balance_id}') {
                        return '{{balanceId}}';
                    } else if (segment === '{portfolioId}' || segment === '{portfolio_id}') {
                        return '{{portfolioId}}';
                    } else if (segment === '{segmentId}' || segment === '{segment_id}') {
                        return '{{segmentId}}';
                    } else if (segment === '{transactionId}' || segment === '{transaction_id}') {
                        return '{{transactionId}}';
                    } else if (segment === '{operationId}' || segment === '{operation_id}') {
                        return '{{operationId}}';
                    } else if (segment === '{assetRateId}' || segment === '{asset_rate_id}') {
                        return '{{assetRateId}}';
                    } else if (segment === '{alias}') {
                        return '{{accountAlias}}';
                    } else if (segment === '{code}') {
                        return '{{externalCode}}';
                    } else if (segment === '{assetCode}' || segment === '{asset_code}') {
                        return '{{assetCode}}';
                    } else if (segment === '{externalId}' || segment === '{external_id}') {
                        return '{{assetRateId}}';
                    } else if (segment === '{id}') {
                        // Generic {id} parameter - map based on context
                        if (step.path.includes('/organizations/{id}')) {
                            return '{{organizationId}}';
                        } else if (step.path.includes('/ledgers/{id}')) {
                            return '{{ledgerId}}';
                        } else if (step.path.includes('/accounts/{id}')) {
                            return '{{accountId}}';
                        } else if (step.path.includes('/assets/{id}')) {
                            return '{{assetId}}';
                        } else if (step.path.includes('/portfolios/{id}')) {
                            return '{{portfolioId}}';
                        } else if (step.path.includes('/segments/{id}')) {
                            return '{{segmentId}}';
                        } else if (step.path.includes('/transactions/{id}')) {
                            return '{{transactionId}}';
                        } else if (step.path.includes('/operations/{id}')) {
                            return '{{operationId}}';
                        } else if (step.path.includes('/balances/{id}')) {
                            return '{{balanceId}}';
                        } else if (step.path.includes('/asset-rates/{id}')) {
                            return '{{assetRateId}}';
                        }
                        return '{{id}}'; // fallback
                    } else {
                        // Keep literal segments as they are
                        return segment;
                    }
                });
                
                // Replace the path array with the correctly structured one
                workflowItem.request.url.path = newPath;
            }
            
            // Also update the raw URL to match the corrected path
            if (workflowItem.request.url && newPath) {
                // Determine the correct base URL for this endpoint
                let baseUrl = '{{onboardingUrl}}'; // Default to onboarding
                if (step.path.includes('/transactions') || step.path.includes('/operations') || 
                    step.path.includes('/balances') || step.path.includes('/asset-rates')) {
                    baseUrl = '{{transactionUrl}}';
                }
                
                // Build raw URL from the corrected path
                const fullPath = '/' + newPath.join('/');
                workflowItem.request.url.raw = baseUrl + fullPath;
            }

            // Add/Update enhanced test script for CI regression testing
            const requires = (step.uses || []).map(use => use.variable || use);
            addOrUpdateTestScript(
                workflowItem, 
                step.outputs, 
                step.number, 
                step.title, 
                step.title, // Use step title as operation description
                step.path, 
                step.method, 
                requires
            );

            // Special handling for Check Account Balance Before Zeroing step
            if (step.title === "Check Account Balance Before Zeroing") {
                console.log(`  DEBUG: Adding balance extraction for step ${step.number}: Check Account Balance Before Zeroing`);
                
                // Add custom test script to extract balance amount and scale
                let testEvent = workflowItem.event.find(e => e.listen === 'test');
                if (testEvent && testEvent.script && testEvent.script.exec) {
                    // Add balance extraction logic to the existing test script
                    const balanceExtractionScript = `
// Extract balance information for zero-out transaction
if (pm.response.code === 200) {
    const responseJson = pm.response.json();
    console.log("🏦 Balance response structure:", JSON.stringify(responseJson, null, 2));
    
    if (responseJson.items && responseJson.items.length > 0) {
        // Get the first balance (USD balance for the account)
        const balance = responseJson.items[0];
        if (balance.available !== undefined && balance.scale !== undefined) {
            const balanceAmount = Math.abs(balance.available); // Use absolute value of available balance
            const balanceScale = balance.scale;
            
            pm.environment.set("currentBalanceAmount", balanceAmount);
            pm.environment.set("currentBalanceScale", balanceScale);
            
            console.log("💰 Extracted balance amount:", balanceAmount);
            console.log("📊 Extracted balance scale:", balanceScale);
            console.log("✅ Balance variables set for zero-out transaction");
        } else {
            console.warn("⚠️ No balance amount/scale found in response");
            console.warn("⚠️ Balance object structure:", JSON.stringify(balance, null, 2));
            // Set default values to prevent failures
            pm.environment.set("currentBalanceAmount", 0);
            pm.environment.set("currentBalanceScale", 2);
        }
    } else {
        console.warn("⚠️ No balance items found in response");
        // Set default values to prevent failures
        pm.environment.set("currentBalanceAmount", 0);
        pm.environment.set("currentBalanceScale", 2);
    }
}`;
                    
                    // Append the balance extraction script to the existing test script
                    testEvent.script.exec[0] += balanceExtractionScript;
                    console.log(`  DEBUG: Added balance extraction logic to test script`);
                }
            }
            
            // Special handling for Zero Out Balance step
            if (step.title === "Zero Out Balance") {
                console.log(`  DEBUG: Attempting to customize Zero Out Balance step (Step ${step.number})...`);
                console.log(`  DEBUG: Request body before customization:`, JSON.stringify(workflowItem.request.body || {}, null, 2));
                
                // Make sure the request has a body
                if (!workflowItem.request.body) {
                    console.log(`  DEBUG: Creating new request body for Zero Out Balance step`);
                    workflowItem.request.body = {
                        mode: 'raw',
                        raw: '',
                        options: {
                            raw: {
                                language: 'json'
                            }
                        }
                    };
                } else {
                    console.log(`  DEBUG: Request body already exists with mode: ${workflowItem.request.body.mode}`);
                }
                
                // Set the custom transaction body for zeroing out the balance (reverse transaction using dynamic amounts)
                const zeroOutTransactionBody = {
                  "chartOfAccountsGroupName": "Example chartOfAccountsGroupName",
                  "code": "Zero Out Balance Transaction",
                  "description": "Reverse transaction to zero out the account balance using actual current balance",
                  "metadata": {
                    "purpose": "balance_zeroing",
                    "reference_step": "48"
                  },
                  "send": {
                    "asset": "USD",
                    "distribute": {
                      "to": [
                        {
                          "account": "@external/USD",
                          "amount": {
                            "asset": "USD",
                            "scale": "{{currentBalanceScale}}",
                            "value": "{{currentBalanceAmount}}"
                          },
                          "chartOfAccounts": "Example chartOfAccounts",
                          "description": "External account for balance zeroing",
                          "metadata": {
                            "operation_type": "credit"
                          }
                        }
                      ]
                    },
                    "source": {
                      "from": [
                        {
                          "account": "{{accountAlias}}",
                          "amount": {
                            "asset": "USD",
                            "scale": "{{currentBalanceScale}}",
                            "value": "{{currentBalanceAmount}}"
                          },
                          "chartOfAccounts": "Example chartOfAccounts",
                          "description": "Source account for balance zeroing",
                          "metadata": {
                            "operation_type": "debit"
                          }
                        }
                      ]
                    },
                    "scale": "{{currentBalanceScale}}",
                    "value": "{{currentBalanceAmount}}"
                  }
                };
                
                // Update the request body with proper variable substitution
                if (workflowItem.request.body) {
                    let bodyString = JSON.stringify(zeroOutTransactionBody, null, 2);
                    // Replace numeric placeholders with Postman variables (without quotes) for all occurrences
                    bodyString = bodyString.replace(/\"{{currentBalanceScale}}\"/g, '{{currentBalanceScale}}');
                    bodyString = bodyString.replace(/\"{{currentBalanceAmount}}\"/g, '{{currentBalanceAmount}}');
                    
                    workflowItem.request.body.raw = bodyString;
                    console.log(`  DEBUG: Updated request body for Zero Out Balance step`);
                    console.log(`  DEBUG: New request body:`, workflowItem.request.body.raw);
                } else {
                    console.log(`  DEBUG: Failed to update request body - body object is null or undefined`);
                }
            }

            // Add the prepared item to the workflow folder
            workflowFolder.item.push(workflowItem);
        } else {
            console.warn(`  Warning: Could not find matching request for Step ${index + 1}: ${step.title} (${step.method} ${step.path})`);
            notFoundCount++;
            missingSteps.push({
                number: step.number,
                title: step.title,
                method: step.method,
                path: step.path
            });
            // Optionally, create a placeholder item
            let baseUrl = '{{onboardingUrl}}'; // Default to onboarding
            // Determine which service URL to use based on path patterns
            if (step.path.includes('/transactions') || step.path.includes('/operations') || 
                step.path.includes('/balances') || step.path.includes('/asset-rates')) {
                baseUrl = '{{transactionUrl}}';
            }
            
            workflowFolder.item.push({
                name: `[NOT FOUND] ${step.title}`, // Indicate it wasn't found
                request: {
                    method: step.method,
                    url: { raw: `${baseUrl}${step.path}` }, // Include proper base URL
                    description: `Placeholder for missing request: ${step.method} ${step.path}`
                },
                 // Add a simple test script indicating failure
                event: [
                    {
                        listen: "test",
                        script: {
                            id: uuidv4(),
                            exec: [
                                `// Request not found in collection for: ${step.method} ${step.path}`,
                                `// Variables to extract: ${JSON.stringify(step.outputs)}`
                            ],
                            type: "text/javascript"
                        }
                    }
                ]
            });
        }
    });

    if (notFoundCount > 0) {
        console.warn(`\nWARNING: ${notFoundCount} requests were not found in the collection and placeholders were added.\n`);
        console.warn('Missing steps:');
        missingSteps.forEach(step => {
            console.warn(`  - Step ${step.number}: ${step.title} (${step.method} ${step.path})`);
        });
        console.warn('');
    }

    return workflowFolder;
}

// --- Main Execution ---

function main() {
    // Check command line arguments
    if (process.argv.length < 5) {
        console.error('Usage: node create-workflow.js <collection-file> <workflow-markdown-file> <output-file>');
        process.exit(1);
    }

    const collectionFilePath = process.argv[2];
    const workflowMarkdownFilePath = process.argv[3];
    const outputFilePath = process.argv[4];

    try {
        // Read input files
        console.log(`Reading collection: ${collectionFilePath}`);
        const collection = readJsonFile(collectionFilePath);

        console.log(`Reading workflow markdown: ${workflowMarkdownFilePath}`);
        const workflowMarkdown = readFile(workflowMarkdownFilePath);

        // Parse workflow steps from markdown
        const workflowSteps = parseWorkflowStepsFromMarkdown(workflowMarkdown);
        console.log(`Parsed ${workflowSteps.length} workflow steps from Markdown.`);
        
        // Debug: Print all step titles to verify Zero Out Balance is included
        console.log("DEBUG: All parsed step titles:");
        workflowSteps.forEach(step => {
            console.log(`  Step ${step.number}: ${step.title}`);
            if (step.title === "Zero Out Balance") {
                console.log(`  DEBUG: Zero Out Balance step details: number=${step.number}, title="${step.title}", method=${step.method}, path=${step.path}`);
            }
        });

        console.log("\nCreating workflow folder by finding and copying requests...");
        const workflowFolder = createWorkflowFolder(collection, workflowSteps);

        // 5. Add/Replace Workflow Folder in Collection
        // Remove existing workflow folder if present
        const existingWorkflowIndex = collection.item.findIndex(item => item.name === "Complete API Workflow");
        if (existingWorkflowIndex !== -1) {
            console.log("Replacing existing 'Complete API Workflow' folder.");
            collection.item.splice(existingWorkflowIndex, 1);
        } else {
            console.log("Adding new 'Complete API Workflow' folder.");
        }
        // Add workflow summary step at the end for CI reporting
        const summaryStep = {
            name: "Workflow Summary & Report",
            request: {
                method: "GET",
                url: {
                    raw: "{{onboardingUrl}}/health",
                    host: ["{{onboardingUrl}}"],
                    path: ["health"]
                },
                description: "Final step that generates comprehensive test summary for CI reporting"
            },
            event: [
                {
                    listen: "test",
                    script: {
                        id: uuidv4(),
                        exec: [generateWorkflowSummaryScript()],
                        type: "text/javascript"
                    }
                }
            ]
        };
        
        workflowFolder.item.push(summaryStep);
        console.log("Added workflow summary and reporting step for CI");

        // Add the new folder at the beginning
        collection.item.unshift(workflowFolder);

        // 6. Write Updated Collection
        console.log(`\nWriting updated collection to: ${outputFilePath}`);
        writeJsonFile(outputFilePath, collection);

        console.log("\nWorkflow generation complete.");
    } catch (error) {
        console.error("An error occurred:", error);
    }
}

// Run main
main();
