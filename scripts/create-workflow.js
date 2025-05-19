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
    // Match Output section
    else if (currentStep && (line.includes('**Output:**') || line.includes('**Outputs:**'))) {
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

// Add or update the test script to extract variables
function addOrUpdateTestScript(workflowItem, outputs) {
    if (!outputs || outputs.length === 0) return; // No outputs to extract
    
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
    
    // Add variable extraction script
    let extractScript = "\n// Extract variables from response\ntry {\n";
    extractScript += "    const jsonData = pm.response.json();\n";
    
    for (const output of outputs) {
        if (typeof output === 'string' && output.trim() !== '') {
            extractScript += "    // Attempting to save " + output + "\n";
            extractScript += "    let foundVal = null;\n";
            extractScript += "    if (jsonData && jsonData." + output + ") { foundVal = jsonData." + output + "; }\n";
            extractScript += "    else if (jsonData && jsonData.data && jsonData.data." + output + ") { foundVal = jsonData.data." + output + "; }\n";
            // Add more specific checks if needed, e.g., iterating arrays
            extractScript += "    if (foundVal) {\n";
            extractScript += "        pm.environment.set(\"" + output + "\", foundVal);\n";
            extractScript += "        console.log(\"Saved " + output + ":\", foundVal);\n";
            extractScript += "    } else { /* console.log(\"Could not find " + output + " directly in response\"); */ }\n";
        } else {
            console.warn(`  Warning: Don't know how to extract output variable '${output}'. Skipping.`);
        }
    }
    extractScript += '} catch (e) { console.error("Error parsing response JSON or extracting variables:", e); }\n';

    // Append extraction script to existing test exec array
    testEvent.script.exec.push(extractScript);
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
            if (workflowItem.request.url && workflowItem.request.url.path) {
                workflowItem.request.url.path = workflowItem.request.url.path.map(segment => {
                    // Replace path parameters with environment variables
                    if (segment === 'organizationId' || segment === '{organizationId}') {
                        return '{organizationId}';
                    } else if (segment === 'ledgerId' || segment === '{ledgerId}') {
                        return '{ledgerId}';
                    } else if (segment === 'accountId' || segment === '{accountId}') {
                        return '{accountId}';
                    } else if (segment === 'assetId' || segment === '{assetId}') {
                        return '{assetId}';
                    } else if (segment === 'balanceId' || segment === '{balanceId}') {
                        return '{balanceId}';
                    } else if (segment === 'portfolioId' || segment === '{portfolioId}') {
                        return '{portfolioId}';
                    } else if (segment === 'segmentId' || segment === '{segmentId}') {
                        return '{segmentId}';
                    } else if (segment === 'transactionId' || segment === '{transactionId}') {
                        return '{transactionId}';
                    } else if (segment === 'operationId' || segment === '{operationId}') {
                        return '{operationId}';
                    }
                    return segment;
                });
            }

            // Add/Update test script for variable extraction
            addOrUpdateTestScript(workflowItem, step.outputs);

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
                
                // Set the custom transaction body for zeroing out the balance
                const zeroOutTransactionBody = {
                  "chartOfAccountsGroupName": "Example chartOfAccountsGroupName",
                  "code": "Example code",
                  "description": "Example description",
                  "metadata": {
                    "key": "value"
                  },
                  "send": {
                    "asset": "USD",
                    "distribute": {
                      "to": [
                        {
                          "account": "@external/USD",
                          "amount": {
                            "asset": "USD",
                            "scale": 2,
                            "value": 100
                          },
                          "chartOfAccounts": "Example chartOfAccounts",
                          "description": "Example description",
                          "metadata": {
                            "key": "value"
                          }
                        }
                      ]
                    },
                    "scale": 2,
                    "source": {
                      "from": [
                        {
                          "account": "{{accountAlias}}",
                          "amount": {
                            "asset": "USD",
                            "scale": 2,
                            "value": 100
                          },
                          "chartOfAccounts": "Example chartOfAccounts",
                          "description": "Example description",
                          "metadata": {
                            "key": "value"
                          }
                        }
                      ]
                    },
                    "value": 100
                  }
                };
                
                // Update the request body
                if (workflowItem.request.body) {
                    workflowItem.request.body.raw = JSON.stringify(zeroOutTransactionBody, null, 2);
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
            workflowFolder.item.push({
                name: `[NOT FOUND] ${step.title}`, // Indicate it wasn't found
                request: {
                    method: step.method,
                    url: { raw: step.path }, // Use path as raw URL placeholder
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
