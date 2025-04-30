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
    if (!outputs || outputs.length === 0) {
        return; // No outputs to extract
    }

    let testEvent = workflowItem.event?.find(e => e.listen === 'test');

    // If no test event exists, create one
    if (!testEvent) {
        if (!workflowItem.event) {
            workflowItem.event = [];
        }
        testEvent = {
            listen: 'test',
            script: {
                id: uuidv4(),
                type: 'text/javascript',
                exec: []
            }
        };
        workflowItem.event.push(testEvent);
    }

    // Ensure exec array exists
    if (!testEvent.script.exec) {
        testEvent.script.exec = [];
    }

    // Basic status code test (add if missing or minimal)
    const hasStatusCodeTest = testEvent.script.exec.some(line => line.includes('pm.test("Status code is') || line.includes('pm.response.to.have.status'));
    if (!hasStatusCodeTest) {
       testEvent.script.exec.unshift(
         '// Basic status code check',
         'pm.test("Status code is 2xx", function () {',
         '    pm.expect(pm.response.code).to.be.within(200, 299);',
         '});',
         ''
       );
    }


    // Generate extraction logic
    let extractScript = '\n// Auto-generated: Extract variables from response\n';
    extractScript += 'try {\n';
    extractScript += '    const jsonData = pm.response.json();\n';

    for (const output of outputs) {
        // Simple direct property access for now (e.g., id, alias)
        // More complex extraction (like balanceId/operationId from arrays) might need refinement
        if (output === 'organizationId' || output === 'ledgerId' || output === 'assetId' ||
            output === 'accountId' || output === 'assetRateId' || output === 'portfolioId' ||
            output === 'segmentId' || output === 'transactionId') {
            extractScript += `    if (jsonData && jsonData.id) {\n`;
            extractScript += `        pm.environment.set("${output}", jsonData.id);\n`;
            extractScript += `        console.log("Saved ${output}:", jsonData.id);\n`;
            extractScript += `    }\n`;
        } else if (output === 'accountAlias') {
             extractScript += `    if (jsonData && jsonData.alias) {\n`;
             extractScript += `        pm.environment.set("${output}", jsonData.alias);\n`;
             extractScript += `        console.log("Saved ${output}:", jsonData.alias);\n`;
             extractScript += `    }\n`;
        } else if (output === 'balanceId' || output === 'operationId') {
             // Basic handling for balanceId/operationId - assumes they might be in 'data' or top-level
             // This might need adjustment based on actual response structures
             extractScript += `    // Attempting to save ${output}\n`;
             extractScript += `    let foundVal = null;\n`;
             extractScript += `    if (jsonData && jsonData.${output}) { foundVal = jsonData.${output}; }\n`;
             extractScript += `    else if (jsonData && jsonData.data && jsonData.data.${output}) { foundVal = jsonData.data.${output}; }\n`;
             // Add more specific checks if needed, e.g., iterating arrays
             extractScript += `    if (foundVal) {\n`;
             extractScript += `        pm.environment.set("${output}", foundVal);\n`;
             extractScript += `        console.log("Saved ${output}:", foundVal);\n`;
             extractScript += `    } else { /* console.log("Could not find ${output} directly in response"); */ }\n`;
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

    steps.forEach((step, index) => {
        console.log(`Processing Step ${index + 1}: ${step.title} (${step.method} ${step.path})`);

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
            if (step.uses && step.uses.length > 0) {
                markdownDesc += '\n\n**Uses:**';
                step.uses.forEach(use => { markdownDesc += `\n- \`${use.variable}\` from step ${use.step}`; });
            }
             if (step.outputs && step.outputs.length > 0) {
                markdownDesc += '\n\n**Outputs:**';
                step.outputs.forEach(output => { markdownDesc += `\n- \`${output}\``; });
            }

            // Prepend Markdown description to existing request description (if any)
            if (workflowItem.request?.description) {
                 if (typeof workflowItem.request.description === 'string') {
                    workflowItem.request.description = markdownDesc + '\n\n---\n\n' + workflowItem.request.description;
                 } else if (workflowItem.request.description.content) {
                    workflowItem.request.description.content = markdownDesc + '\n\n---\n\n' + workflowItem.request.description.content;
                 } else {
                     workflowItem.request.description = markdownDesc; // Overwrite if format is unexpected
                 }
            } else {
                 if (!workflowItem.request) workflowItem.request = {}; // Ensure request object exists
                 workflowItem.request.description = markdownDesc;
            }


            // Add/Update test script for variable extraction
            addOrUpdateTestScript(workflowItem, step.outputs);

            // Add the prepared item to the workflow folder
            workflowFolder.item.push(workflowItem);
        } else {
            console.warn(`  WARNING: Could not find matching request for Step ${index + 1}: ${step.title} (${step.method} ${step.path})`);
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
    // 1. Parse Args
    const args = process.argv.slice(2);
    if (args.length < 3) {
        console.error('Usage: node create-workflow.js <input-collection.json> <workflow.md> <output-collection.json>');
        process.exit(1);
    }
    const [inputCollectionPath, workflowMdPath, outputCollectionPath] = args;

    // 2. Read Files
    console.log(`Reading collection: ${inputCollectionPath}`);
    const collection = readJsonFile(inputCollectionPath);
    console.log(`Reading workflow markdown: ${workflowMdPath}`);
    const workflowMd = readFile(workflowMdPath);

    // 3. Parse Workflow Steps
    const steps = parseWorkflowStepsFromMarkdown(workflowMd);

    // 4. Create Workflow Folder (find and copy requests)
    console.log("\nCreating workflow folder by finding and copying requests...");
    const workflowFolder = createWorkflowFolder(collection, steps);

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
    console.log(`\nWriting updated collection to: ${outputCollectionPath}`);
    writeJsonFile(outputCollectionPath, collection);

    console.log("\nWorkflow generation complete.");
}

// Run main
main();
