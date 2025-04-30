#!/usr/bin/env node

/**
 * Midaz API Workflow Generator
 * 
 * This script creates a Postman collection folder with the workflow sequence
 * from the WORKFLOW.md file. It should be called after convert-openapi.js.
 * 
 * Usage: node create-workflow.js <input-collection> <workflow-md> <output-collection>
 * 
 * Example: node create-workflow.js ./postman/MIDAZ.postman_collection.json ./postman/WORKFLOW.md ./postman/MIDAZ.postman_collection.json
 */

const fs = require('fs');
const path = require('path');
const { v4: uuidv4 } = require('uuid');

// Parse command line arguments
function parseCommandLineArgs() {
  const args = process.argv.slice(2);
  
  if (args.length < 3) {
    console.error('Usage: node create-workflow.js <input-collection> <workflow-md> <output-collection>');
    process.exit(1);
  }
  
  return {
    inputCollection: args[0],
    workflowMd: args[1],
    outputCollection: args[2]
  };
}

// Read the Postman collection
function readPostmanCollection(filePath) {
  try {
    const data = fs.readFileSync(filePath, 'utf8');
    return JSON.parse(data);
  } catch (error) {
    console.error(`Error reading Postman collection: ${error.message}`);
    process.exit(1);
  }
}

// Read the workflow markdown file
function readWorkflowMd(filePath) {
  try {
    return fs.readFileSync(filePath, 'utf8');
  } catch (error) {
    console.error(`Error reading workflow markdown: ${error.message}`);
    process.exit(1);
  }
}

// Parse the workflow markdown to extract steps
function parseWorkflowSteps(markdown) {
  const steps = [];
  // Updated regex to handle steps across different sections
  const stepRegex = /^\d+\.\s+\*\*([^*]+)\*\*\s*\n\s*\n\s*-\s+`([^`]+)`\s*\n\s*-\s+([^\n]+)(?:\s*\n\s*-\s+\*\*Uses:\*\*\s+([^\n]+))?(?:\s*\n\s*-\s+\*\*Output:\*\*\s+([^\n]+))?/gm;
  
  let match;
  while ((match = stepRegex.exec(markdown)) !== null) {
    const step = {
      number: parseInt(match[0].match(/^\d+/)[0], 10), // Extract the actual step number from the markdown
      name: match[1].trim(),
      endpoint: match[2].trim(),
      description: match[3].trim(),
      uses: match[4] ? parseUses(match[4]) : [],
      outputs: match[5] ? parseOutputs(match[5]) : []
    };
    steps.push(step);
  }
  
  // Sort steps by number to ensure they're in the correct order
  steps.sort((a, b) => a.number - b.number);
  
  return steps;
}

// Parse the "Uses" field to extract dependencies
function parseUses(usesText) {
  const uses = [];
  const usesRegex = /`([^`]+)`\s+from\s+step\s+(\d+)/g;
  
  let match;
  while ((match = usesRegex.exec(usesText)) !== null) {
    uses.push({
      variable: match[1].trim(),
      step: parseInt(match[2], 10)
    });
  }
  
  return uses;
}

// Parse the "Output" field to extract outputs
function parseOutputs(outputText) {
  return outputText.split(',').map(output => output.trim().replace(/`/g, ''));
}

// Find a request in the collection by its path and method
function findRequestInCollection(collection, path, method) {
  // Normalize the path by removing the leading slash if present
  const normalizedPath = path.startsWith('/') ? path.substring(1) : path;
  
  // Function to search recursively through folders
  function searchInItems(items) {
    for (const item of items) {
      // If this is a folder, search its items
      if (item.item) {
        const found = searchInItems(item.item);
        if (found) return found;
      } else if (item.request) {
        // This is a request, check if it matches
        const request = item.request;
        if (request.method === method.toUpperCase()) {
          // Get the path from the URL
          let requestPath = '';
          if (request.url.path) {
            requestPath = request.url.path.join('/');
          } else if (request.url.raw) {
            // Extract path from raw URL
            const urlParts = request.url.raw.split('?')[0].split('/');
            // Remove host parts (those with {{variables}})
            const pathParts = urlParts.filter(part => !part.includes('{{') || part.includes('organizationId') || part.includes('ledgerId'));
            requestPath = pathParts.join('/');
          }
          
          // Normalize the request path
          requestPath = requestPath.replace(/\{\{/g, '{').replace(/\}\}/g, '}');
          
          // For DELETE endpoints, we need to be more flexible with matching
          if (method.toUpperCase() === 'DELETE') {
            // Extract the resource type from the path (e.g., 'organizations', 'ledgers', etc.)
            const pathParts = normalizedPath.split('/');
            const resourceType = pathParts[pathParts.length - 2]; // The resource type is usually the second-to-last part
            
            // Check if the request path contains the resource type and the DELETE method matches
            if (requestPath.includes(resourceType)) {
              return item;
            }
          } else {
            // For non-DELETE endpoints, use the existing matching logic
            if (requestPath.includes(normalizedPath) || normalizedPath.includes(requestPath)) {
              return item;
            }
          }
        }
      }
    }
    return null;
  }
  
  return searchInItems(collection.item);
}

// Create a workflow folder with all the steps
function createWorkflowFolder(collection, steps) {
  const workflowFolder = {
    name: "Complete API Workflow",
    description: "A complete linear test sequence for all the main endpoints of the Midaz API, including creation, retrieval, updates, and deletion.",
    item: [],
    _postman_id: uuidv4()
  };
  
  // Process each step and add it to the workflow folder
  for (const step of steps) {
    // Extract HTTP method and path from the endpoint
    const [method, path] = step.endpoint.split(' ');
    
    // Find the corresponding request in the collection
    const request = findRequestInCollection(collection, path, method);
    
    if (request) {
      // Clone the request
      const clonedRequest = JSON.parse(JSON.stringify(request));
      
      // Update the name to include the step number
      clonedRequest.name = `${step.number}. ${step.name}`;
      
      // Add description with uses and outputs
      let description = step.description;
      
      if (step.uses.length > 0) {
        description += "\n\n**Uses:**\n";
        for (const use of step.uses) {
          description += `- \`${use.variable}\` from step ${use.step}\n`;
        }
      }
      
      if (step.outputs.length > 0) {
        description += "\n\n**Outputs:**\n";
        for (const output of step.outputs) {
          description += `- \`${output}\`\n`;
        }
      }
      
      clonedRequest.description = description;
      
      // Add to workflow folder
      workflowFolder.item.push(clonedRequest);
    } else {
      console.warn(`Warning: Could not find request for step ${step.number}: ${method} ${path}`);
    }
  }
  
  return workflowFolder;
}

// Add the workflow folder to the collection
function addWorkflowToCollection(collection, workflowFolder) {
  // Add the workflow folder at the beginning of the collection
  collection.item.unshift(workflowFolder);
  return collection;
}

// Write the updated collection to file
function writeCollection(collection, outputPath) {
  try {
    fs.writeFileSync(outputPath, JSON.stringify(collection, null, 2));
    console.log(`Updated collection written to ${outputPath}`);
  } catch (error) {
    console.error(`Error writing collection: ${error.message}`);
    process.exit(1);
  }
}

// Main function
function main() {
  // Parse command line arguments
  const args = parseCommandLineArgs();
  
  // Read the input collection
  const collection = readPostmanCollection(args.inputCollection);
  
  // Read the workflow markdown
  const workflowMd = readWorkflowMd(args.workflowMd);
  
  // Parse the workflow steps
  const steps = parseWorkflowSteps(workflowMd);
  console.log(`Found ${steps.length} workflow steps`);
  
  // Create the workflow folder
  const workflowFolder = createWorkflowFolder(collection, steps);
  
  // Add the workflow folder to the collection
  const updatedCollection = addWorkflowToCollection(collection, workflowFolder);
  
  // Write the updated collection to file
  writeCollection(updatedCollection, args.outputCollection);
}

// Run the main function
main();
