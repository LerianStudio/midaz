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
function parseWorkflowSteps(markdownContent) {
  const steps = [];
  
  // Regular expression to match step entries
  const stepRegex = /(\d+)\. \*\*(.*?)\*\*\s*\n\s*- `(.*?)`\s*\n\s*-(.*?)(?:\s*\n\s*- \*\*Uses:\*\*(.*?))?(?:\s*\n\s*- \*\*Output(?:s)?:\*\*(.*?))?(?=\n\s*\d+\. \*\*|\n*$)/gs;
  
  let match;
  while ((match = stepRegex.exec(markdownContent)) !== null) {
    const number = parseInt(match[1], 10);
    const title = match[2].trim();
    const endpoint = match[3].trim();
    const description = match[4].trim();
    
    // Parse the uses
    const uses = [];
    if (match[5]) {
      const usesRegex = /`(.*?)`\s*from\s*step\s*(\d+)/g;
      let usesMatch;
      while ((usesMatch = usesRegex.exec(match[5])) !== null) {
        uses.push({
          variable: usesMatch[1].trim(),
          step: parseInt(usesMatch[2], 10)
        });
      }
    }
    
    // Parse the outputs
    const outputs = [];
    if (match[6]) {
      const outputsRegex = /`(.*?)`/g;
      let outputsMatch;
      while ((outputsMatch = outputsRegex.exec(match[6])) !== null) {
        outputs.push(outputsMatch[1].trim());
      }
    }
    
    // Split the endpoint into method and path
    const [method, path] = endpoint.split(' ');
    
    steps.push({
      number,
      title,
      method,
      path,
      description,
      uses,
      outputs
    });
  }
  
  console.log(`Found ${steps.length} workflow steps`);
  return steps;
}

// Parse a step from the workflow markdown
function parseStep(line, number) {
  const step = {
    number,
    title: '',
    method: '',
    path: '',
    description: '',
    uses: [],
    outputs: []
  };
  
  // Extract the title
  const titleMatch = line.match(/^\d+\.\s+\*\*([^*]+)\*\*/);
  if (titleMatch) {
    step.title = titleMatch[1].trim();
  }
  
  console.log(`Found step ${step.number}: ${step.title}`);
  
  return step;
}

// Parse the workflow steps from the markdown file
function parseWorkflowStepsFromMarkdown(markdown) {
  const lines = markdown.split('\n');
  const steps = [];
  let currentStep = null;
  let stepNumber = 0;
  
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    
    // Check if this is a step title line
    if (line.match(/^\d+\.\s+\*\*([^*]+)\*\*/)) {
      stepNumber++;
      currentStep = {
        number: stepNumber,
        title: '',
        method: '',
        path: '',
        description: '',
        uses: [],
        outputs: []
      };
      
      // Extract the title
      const titleMatch = line.match(/^\d+\.\s+\*\*([^*]+)\*\*/);
      if (titleMatch) {
        currentStep.title = titleMatch[1].trim();
      }
      
      steps.push(currentStep);
      console.log(`Found step ${currentStep.number}: ${currentStep.title}`);
    } 
    // Check if this is a path line for the current step
    else if (currentStep && line.match(/^\s*- `[A-Z]+ /)) {
      const pathMatch = line.match(/^\s*- `([A-Z]+) ([^`]+)`/);
      if (pathMatch) {
        currentStep.method = pathMatch[1];
        currentStep.path = pathMatch[2];
        console.log(`  Method: ${currentStep.method}, Path: ${currentStep.path}`);
      }
    }
    // Check if this is a description line for the current step
    else if (currentStep && line.match(/^\s*- [^`]/)) {
      const descriptionMatch = line.match(/^\s*- ([^`].+)$/);
      if (descriptionMatch && !line.includes('**Uses:**') && !line.includes('**Output')) {
        currentStep.description = descriptionMatch[1].trim();
      }
    }
    // Check if this is a "uses" line for the current step
    else if (currentStep && line.includes('**Uses:**')) {
      // Process the next lines until we hit a blank line or another section
      let j = i + 1;
      while (j < lines.length && lines[j].trim() !== '' && !lines[j].includes('**Output')) {
        const useMatch = lines[j].match(/^\s*- `([^`]+)` from step (\d+)/);
        if (useMatch) {
          currentStep.uses.push({
            variable: useMatch[1],
            step: parseInt(useMatch[2])
          });
        }
        j++;
      }
    }
    // Check if this is an "outputs" line for the current step
    else if (currentStep && (line.includes('**Output:**') || line.includes('**Outputs:**'))) {
      // Process the next lines until we hit a blank line or another section
      let j = i + 1;
      while (j < lines.length && lines[j].trim() !== '' && !lines[j].includes('**Uses:**')) {
        const outputMatch = lines[j].match(/^\s*- `([^`]+)`/);
        if (outputMatch) {
          currentStep.outputs.push(outputMatch[1]);
        }
        j++;
      }
    }
  }
  
  console.log(`Found ${steps.length} workflow steps`);
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
  // Special case for the transaction creation endpoint
  if (method === 'POST' && path.includes('transactions/json')) {
    console.log("Special handling for transaction creation endpoint");
    // Search for the transaction creation endpoint in the main collection
    for (const item of collection.item) {
      if (item.item) {
        for (const subItem of item.item) {
          if (subItem.request && 
              subItem.request.method === 'POST' && 
              subItem.request.url && 
              (typeof subItem.request.url === 'string' ? 
                subItem.request.url.includes('transactions/json') : 
                (subItem.request.url.path && subItem.request.url.path.includes('transactions') && 
                 subItem.request.url.path.includes('json')))) {
            
            console.log(`Found transaction creation endpoint: ${subItem.name}`);
            return subItem;
          }
        }
      }
    }
  }
  
  // Normalize the path by removing the leading slash if present
  const normalizedPath = path.startsWith('/') ? path.substring(1) : path;
  const resourceTypes = getResourceTypesFromPath(normalizedPath);
  const mainResourceType = resourceTypes.length > 0 ? resourceTypes[resourceTypes.length - 1] : null;
  
  // For debugging
  console.log(`Finding request for ${method} ${normalizedPath} (Resource types: ${resourceTypes.join(', ')})`);
  
  // First, try to find the request in the main resource folders
  const mainFolders = collection.item.filter(item => item.item && !item.name.includes('Workflow'));
  
  for (const folder of mainFolders) {
    console.log(`Searching in main folder: ${folder.name}`);
    
    // Check if this folder matches our resource type
    const folderResourceType = getResourceTypeFromName(folder.name);
    if (folderResourceType && resourceTypes.includes(folderResourceType)) {
      console.log(`Folder ${folder.name} matches resource type ${folderResourceType}`);
      
      // Search for the request in this folder
      for (const item of folder.item) {
        if (item.request) {
          const request = item.request;
          const requestName = item.name.toLowerCase();
          
          // Check if the method matches
          if (request.method === method.toUpperCase()) {
            // Check if this is the right operation type
            if (method === 'POST' && (requestName.includes('create') || requestName.includes('new'))) {
              if (mainResourceType && requestName.includes(mainResourceType.slice(0, -1))) {
                console.log(`Found match in main folder: ${item.name} in ${folder.name}`);
                return item;
              }
            } else if (method === 'GET' && normalizedPath.includes('{') && 
                      (requestName.includes('retrieve') || requestName.includes('get') || requestName.includes('specific'))) {
              if (mainResourceType && requestName.includes(mainResourceType.slice(0, -1))) {
                console.log(`Found match in main folder: ${item.name} in ${folder.name}`);
                return item;
              }
            } else if (method === 'GET' && !normalizedPath.includes('{') && 
                      (requestName.includes('list') || requestName.includes('all'))) {
              if (mainResourceType && requestName.includes(mainResourceType)) {
                console.log(`Found match in main folder: ${item.name} in ${folder.name}`);
                return item;
              }
            } else if (method === 'PATCH' && 
                      (requestName.includes('update') || requestName.includes('modify') || requestName.includes('existing'))) {
              if (mainResourceType && requestName.includes(mainResourceType.slice(0, -1))) {
                console.log(`Found match in main folder: ${item.name} in ${folder.name}`);
                return item;
              }
            } else if (method === 'DELETE' && 
                      (requestName.includes('delete') || requestName.includes('remove'))) {
              if (mainResourceType && requestName.includes(mainResourceType.slice(0, -1))) {
                console.log(`Found match in main folder: ${item.name} in ${folder.name}`);
                return item;
              }
            }
            
            // Also check the URL path for a match
            if (request.url) {
              let requestPath = '';
              
              // Extract path from url object or string
              if (request.url.path) {
                requestPath = request.url.path.join('/');
              } else if (typeof request.url === 'string') {
                // Extract path from URL string
                const urlParts = request.url.split('?')[0].split('/');
                // Skip the protocol and domain
                const pathParts = [];
                let foundPath = false;
                for (const part of urlParts) {
                  if (foundPath || part === 'v1') {
                    foundPath = true;
                    pathParts.push(part);
                  }
                }
                requestPath = pathParts.join('/');
              }
              
              // Check if the path contains all our resource types
              let matchesAllTypes = true;
              for (const type of resourceTypes) {
                if (!requestPath.includes(type)) {
                  matchesAllTypes = false;
                  break;
                }
              }
              
              if (matchesAllTypes) {
                console.log(`Found match by URL path in main folder: ${item.name} in ${folder.name}`);
                return item;
              }
            }
          }
        }
      }
    }
  }
  
  // If we didn't find a match in the main folders, fall back to searching the entire collection
  console.log("Falling back to searching the entire collection");
  
  // Function to search recursively through folders
  function searchInItems(items, folderName = '') {
    for (const item of items) {
      // If this is a folder, search its items
      if (item.item) {
        const folderResourceType = getResourceTypeFromName(item.name);
        // If the folder matches one of our resource types, prioritize searching here
        if (folderResourceType && resourceTypes.includes(folderResourceType)) {
          console.log(`Searching in folder: ${item.name} (matches resource type)`);
          const found = searchInItems(item.item, item.name);
          if (found) return found;
        } else {
          // Still search other folders, but with lower priority
          const found = searchInItems(item.item, item.name);
          if (found) return found;
        }
      } else if (item.request) {
        // This is a request, check if it matches
        const request = item.request;
        
        // Check if the method matches
        if (request.method === method.toUpperCase()) {
          // Get the request name and the folder it's in
          const requestName = item.name.toLowerCase();
          
          // Check if this request is for the right resource type based on its name and folder
          const requestResourceType = getResourceTypeFromName(requestName) || 
                                     getResourceTypeFromName(folderName);
          
          // If we have a main resource type and it matches the request resource type
          if (mainResourceType && requestResourceType === mainResourceType) {
            // Now check for the specific operation type
            if (method.toUpperCase() === 'POST' && (requestName.includes('create') || requestName.includes('new'))) {
              console.log(`Found match by resource type and operation: ${item.name} in ${folderName}`);
              return item;
            } else if (method.toUpperCase() === 'GET' && normalizedPath.includes('{') && 
                      (requestName.includes('retrieve') || requestName.includes('get'))) {
              console.log(`Found match by resource type and operation: ${item.name} in ${folderName}`);
              return item;
            } else if (method.toUpperCase() === 'GET' && !normalizedPath.includes('{') && 
                      (requestName.includes('list') || requestName.includes('all'))) {
              console.log(`Found match by resource type and operation: ${item.name} in ${folderName}`);
              return item;
            } else if (method.toUpperCase() === 'PATCH' && 
                      (requestName.includes('update') || requestName.includes('modify'))) {
              console.log(`Found match by resource type and operation: ${item.name} in ${folderName}`);
              return item;
            } else if (method.toUpperCase() === 'DELETE' && 
                      (requestName.includes('delete') || requestName.includes('remove'))) {
              console.log(`Found match by resource type and operation: ${item.name} in ${folderName}`);
              return item;
            }
          }
          
          // Also check the URL path for a more precise match
          if (request.url) {
            let requestPath = '';
            
            // Extract path from url object or string
            if (request.url.path) {
              requestPath = request.url.path.join('/');
            } else if (typeof request.url === 'string') {
              // Extract path from URL string
              const urlParts = request.url.split('?')[0].split('/');
              // Skip the protocol and domain
              const pathParts = [];
              let foundPath = false;
              for (const part of urlParts) {
                if (foundPath || part === 'v1') {
                  foundPath = true;
                  pathParts.push(part);
                }
              }
              requestPath = pathParts.join('/');
            }
            
            // Check if the path matches our resource types in the right order
            let matchesAllTypes = true;
            for (const type of resourceTypes) {
              if (!requestPath.includes(type)) {
                matchesAllTypes = false;
                break;
              }
            }
            
            if (matchesAllTypes) {
              // Check if the method and operation type match
              if (method.toUpperCase() === 'POST' && (requestName.includes('create') || requestName.includes('new'))) {
                console.log(`Found match by URL path and operation: ${item.name} in ${folderName}`);
                return item;
              } else if (method.toUpperCase() === 'GET' && normalizedPath.includes('{') && 
                        (requestName.includes('retrieve') || requestName.includes('get'))) {
                console.log(`Found match by URL path and operation: ${item.name} in ${folderName}`);
                return item;
              } else if (method.toUpperCase() === 'GET' && !normalizedPath.includes('{') && 
                        (requestName.includes('list') || requestName.includes('all'))) {
                console.log(`Found match by URL path and operation: ${item.name} in ${folderName}`);
                return item;
              } else if (method.toUpperCase() === 'PATCH' && 
                        (requestName.includes('update') || requestName.includes('modify'))) {
                console.log(`Found match by URL path and operation: ${item.name} in ${folderName}`);
                return item;
              } else if (method.toUpperCase() === 'DELETE' && 
                        (requestName.includes('delete') || requestName.includes('remove'))) {
                console.log(`Found match by URL path and operation: ${item.name} in ${folderName}`);
                return item;
              }
            }
          }
        }
      }
    }
    return null;
  }
  
  return searchInItems(collection.item);
}

// Extract the resource types from a path (e.g., ["organizations", "ledgers"] from "v1/organizations/{organizationId}/ledgers/{ledgerId}")
function getResourceTypesFromPath(path) {
  const parts = path.split('/').filter(p => p);
  const resourceTypes = [];
  
  // Look for known resource types
  const knownTypes = [
    'organizations', 'ledgers', 'accounts', 'assets', 'asset-rates',
    'portfolios', 'segments', 'transactions', 'operations', 'balances'
  ];
  
  for (const part of parts) {
    // Check if this part is a known resource type
    for (const type of knownTypes) {
      if (part === type) {
        resourceTypes.push(type);
        break;
      }
    }
  }
  
  return resourceTypes;
}

// Extract a resource type from a name (e.g., "organizations" from "List all organizations")
function getResourceTypeFromName(name) {
  if (!name) return null;
  
  name = name.toLowerCase();
  
  // Look for known resource types in the name
  const knownTypes = [
    'organizations', 'ledgers', 'accounts', 'assets', 'asset-rates',
    'portfolios', 'segments', 'transactions', 'operations', 'balances'
  ];
  
  for (const type of knownTypes) {
    // Check for plural form
    if (name.includes(type)) {
      return type;
    }
    // Check for singular form (remove trailing 's')
    if (name.includes(type.slice(0, -1))) {
      return type;
    }
  }
  
  return null;
}

// For backward compatibility
function getResourceTypeFromPath(path) {
  return getResourceTypesFromPath(path)[0] || null;
}

// Update the URL in the request to include the correct path with variables
function updateRequestUrl(request, step) {
  if (!request || !request.url) return;
  
  // Extract path parameters from the step path
  const pathParams = extractPathParams(step.path);
  
  // Get the base URL and path segments
  let baseUrl = '';
  if (step.path.includes('/v1/organizations')) {
    baseUrl = '{{onboardingUrl}}';
  } else {
    baseUrl = '{{transactionUrl}}';
  }
  
  // Build the path segments with variables
  const pathSegments = [];
  const pathParts = step.path.split('/').filter(p => p);
  
  for (const part of pathParts) {
    if (part.startsWith('{') && part.endsWith('}')) {
      // This is a path parameter
      const paramName = part.substring(1, part.length - 1);
      let variableName = '';
      
      // Map parameter names to variable names
      if (paramName === 'organizationId') {
        variableName = 'organizationId';
      } else if (paramName === 'ledgerId') {
        variableName = 'ledgerId';
      } else if (paramName === 'assetId') {
        variableName = 'assetId';
      } else if (paramName === 'accountId') {
        variableName = 'accountId';
      } else if (paramName === 'portfolioId') {
        variableName = 'portfolioId';
      } else if (paramName === 'segmentId') {
        variableName = 'segmentId';
      } else if (paramName === 'transactionId') {
        variableName = 'transactionId';
      } else if (paramName === 'operationId') {
        variableName = 'operationId';
      } else if (paramName === 'balanceId') {
        variableName = 'balanceId';
      } else {
        variableName = paramName;
      }
      
      pathSegments.push(`{{${variableName}}}`);
    } else {
      pathSegments.push(part);
    }
  }
  
  // Update the URL in the request
  request.url.raw = `${baseUrl}/${pathSegments.join('/')}`;
  request.url.host = [baseUrl];
  request.url.path = pathSegments;
  
  // Add path variables if needed
  if (pathParams.length > 0 && !request.url.variable) {
    request.url.variable = [];
  }
  
  // Update or add path variables
  for (const param of pathParams) {
    const paramName = param.substring(1, param.length - 1);
    let variableName = '';
    
    // Map parameter names to variable names
    if (paramName === 'organizationId') {
      variableName = 'organizationId';
    } else if (paramName === 'ledgerId') {
      variableName = 'ledgerId';
    } else if (paramName === 'assetId') {
      variableName = 'assetId';
    } else if (paramName === 'accountId') {
      variableName = 'accountId';
    } else if (paramName === 'portfolioId') {
      variableName = 'portfolioId';
    } else if (paramName === 'segmentId') {
      variableName = 'segmentId';
    } else if (paramName === 'transactionId') {
      variableName = 'transactionId';
    } else if (paramName === 'operationId') {
      variableName = 'operationId';
    } else if (paramName === 'balanceId') {
      variableName = 'balanceId';
    } else {
      variableName = paramName;
    }
    
    // Check if the variable already exists
    const existingVar = request.url.variable ? request.url.variable.find(v => v.key === paramName) : null;
    
    if (existingVar) {
      existingVar.value = `{{${variableName}}}`;
    } else if (request.url.variable) {
      request.url.variable.push({
        key: paramName,
        value: `{{${variableName}}}`,
        description: `${paramName} in UUID format`
      });
    }
  }
}

// Extract path parameters from a path
function extractPathParams(path) {
  const params = [];
  const parts = path.split('/');
  
  for (const part of parts) {
    if (part.startsWith('{') && part.endsWith('}')) {
      params.push(part);
    }
  }
  
  return params;
}

// Add test script to extract variables from response
function addTestScript(request, outputs) {
  let testScript = findEventScript(request.event, 'test');
  if (!testScript) {
    testScript = {
      listen: 'test',
      script: {
        type: 'text/javascript',
        exec: ['']
      }
    };
    request.event.push(testScript);
  }
  
  let extractScript = '\n// Extract variables from response\n';
  for (const output of outputs) {
    if (output === 'organizationId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("organizationId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved organizationId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'ledgerId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("ledgerId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved ledgerId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'assetId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("assetId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved assetId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'accountId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("accountId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved accountId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'accountAlias') {
      extractScript += 'if (pm.response.json().alias) {\n';
      extractScript += '  pm.environment.set("accountAlias", pm.response.json().alias);\n';
      extractScript += '  console.log("Saved accountAlias:", pm.response.json().alias);\n';
      extractScript += '}\n';
    } else if (output === 'assetRateId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("assetRateId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved assetRateId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'portfolioId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("portfolioId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved portfolioId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'segmentId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("segmentId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved segmentId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'transactionId') {
      extractScript += 'if (pm.response.json().id) {\n';
      extractScript += '  pm.environment.set("transactionId", pm.response.json().id);\n';
      extractScript += '  console.log("Saved transactionId:", pm.response.json().id);\n';
      extractScript += '}\n';
    } else if (output === 'balanceId') {
      extractScript += 'if (pm.response.json().operations && pm.response.json().operations.length > 0) {\n';
      extractScript += '  // Find the destination operation\n';
      extractScript += '  var destinationOp = null;\n';
      extractScript += '  if (pm.response.json().destination && pm.response.json().destination.length > 0) {\n';
      extractScript += '    const destAccount = pm.response.json().destination[0];\n';
      extractScript += '    for (let i = 0; i < pm.response.json().operations.length; i++) {\n';
      extractScript += '      if (pm.response.json().operations[i].accountId === destAccount) {\n';
      extractScript += '        destinationOp = pm.response.json().operations[i];\n';
      extractScript += '        break;\n';
      extractScript += '      }\n';
      extractScript += '    }\n';
      extractScript += '  }\n';
      extractScript += '  \n';
      extractScript += '  if (destinationOp && destinationOp.balanceId) {\n';
      extractScript += '    pm.environment.set("balanceId", destinationOp.balanceId);\n';
      extractScript += '    console.log("Saved balanceId:", destinationOp.balanceId);\n';
      extractScript += '  }\n';
      extractScript += '}\n';
    } else if (output === 'operationId') {
      extractScript += 'if (pm.response.json().operations && pm.response.json().operations.length > 0) {\n';
      extractScript += '  // Find the destination operation\n';
      extractScript += '  var destinationOp = null;\n';
      extractScript += '  if (pm.response.json().destination && pm.response.json().destination.length > 0) {\n';
      extractScript += '    const destAccount = pm.response.json().destination[0];\n';
      extractScript += '    for (let i = 0; i < pm.response.json().operations.length; i++) {\n';
      extractScript += '      if (pm.response.json().operations[i].accountId === destAccount) {\n';
      extractScript += '        destinationOp = pm.response.json().operations[i];\n';
      extractScript += '        break;\n';
      extractScript += '      }\n';
      extractScript += '    }\n';
      extractScript += '  }\n';
      extractScript += '  \n';
      extractScript += '  if (destinationOp && destinationOp.id) {\n';
      extractScript += '    pm.environment.set("operationId", destinationOp.id);\n';
      extractScript += '    console.log("Saved operationId:", destinationOp.id);\n';
      extractScript += '  }\n';
      extractScript += '}\n';
    }
  }
  
  // Add the extract script to the test script
  testScript.script.exec.push(extractScript);
}

// Find a script in the event array by its listen type
function findEventScript(events, listenType) {
  if (!events) return null;
  
  for (const event of events) {
    if (event.listen === listenType) {
      return event;
    }
  }
  
  return null;
}

// Function to add a request to the workflow folder
function addRequestToWorkflow(collection, workflowFolder, step) {
  console.log(`Processing step ${step.number}: ${step.title}`);
  
  // Special case for the transaction creation endpoint
  if (step.method === 'POST' && step.path.includes('transactions/json')) {
    console.log("Special handling for transaction creation endpoint");
    console.log(`Step path: ${step.path}`);
    
    // Add a hardcoded request body for transactions
    const workflowRequest = {
      name: `${step.number}. ${step.title}`,
      request: {
        method: 'POST',
        header: [
          {
            key: 'Authorization',
            value: 'Bearer {{authToken}}',
            description: 'Authorization Bearer Token',
            disabled: false
          },
          {
            key: 'X-Request-Id',
            value: '{{$guid}}',
            description: 'Request ID',
            disabled: true
          }
        ],
        url: {
          raw: '{{onboardingUrl}}/v1/organizations/{{organizationId}}/ledgers/{{ledgerId}}/transactions/json',
          host: ['{{onboardingUrl}}'],
          path: ['v1', 'organizations', '{{organizationId}}', 'ledgers', '{{ledgerId}}', 'transactions', 'json'],
          variable: [
            {
              key: 'organizationId',
              value: '{{organizationId}}',
              description: 'organizationId in UUID format'
            },
            {
              key: 'ledgerId',
              value: '{{ledgerId}}',
              description: 'ledgerId in UUID format'
            }
          ]
        },
        body: {
          mode: 'raw',
          raw: '{\n  "chartOfAccountsGroupName": "Example chartOfAccountsGroupName",\n  "code": "Example code",\n  "description": "Example description",\n  "metadata": {\n    "key": "value"\n  },\n  "send": {\n    "asset": "USD",\n    "distribute": {\n      "to": [\n        {\n          "account": "{{accountAlias}}",\n          "amount": {\n            "asset": "USD",\n            "scale": 2,\n            "value": 100\n          },\n          "chartOfAccounts": "Example chartOfAccounts",\n          "description": "Example description",\n          "metadata": {\n            "key": "value"\n          }\n        }\n      ]\n    },\n    "scale": 2,\n    "source": {\n      "from": [\n        {\n          "account": "@external/USD",\n          "amount": {\n            "asset": "USD",\n            "scale": 2,\n            "value": 100\n          },\n          "chartOfAccounts": "Example chartOfAccounts",\n          "description": "Example description",\n          "metadata": {\n            "key": "value"\n          }\n        }\n      ]\n    },\n    "value": 100\n  }\n}',
          options: {
            raw: {
              language: 'json'
            }
          }
        },
        description: 'Create a Transaction with the input DSL file'
      },
      response: [],
      event: [
        {
          listen: 'prerequest',
          script: {
            type: 'text/javascript',
            exec: [
              '',
              '// Check for auth token',
              'if (!pm.environment.get("authToken")) {',
              '  console.log("Warning: authToken is not set in the environment");',
              '}',
              '',
              '// Set authorization header if it exists',
              'if (pm.environment.get("authToken")) {',
              '  pm.request.headers.upsert({',
              '    key: "Authorization",',
              '    value: "Bearer " + pm.environment.get("authToken")',
              '  });',
              '}',
              '',
              '// Set request ID for tracing',
              'pm.request.headers.upsert({',
              '  key: "X-Request-Id",',
              '  value: pm.variables.replaceIn("{{$guid}}")',
              '});',
              ''
            ]
          }
        },
        {
          listen: 'test',
          script: {
            type: 'text/javascript',
            exec: [
              '',
              '// Test for successful response status',
              'if (pm.request.method === "POST") {',
              '  pm.test("Status code is 200 or 201", function () {',
              '    pm.expect(pm.response.code).to.be.oneOf([200, 201]);',
              '  });',
              '} else if (pm.request.method === "DELETE") {',
              '  pm.test("Status code is 204 No Content", function () {',
              '    pm.expect(pm.response.code).to.equal(204);',
              '  });',
              '} else {',
              '  pm.test("Status code is 200 OK", function () {',
              '    pm.expect(pm.response.code).to.equal(200);',
              '  });',
              '}',
              '',
              '// Validate response has the expected format',
              'pm.test("Response has the correct structure", function() {',
              '  // For DELETE operations that return 204 No Content, the body is empty by design',
              '  if (pm.response.code === 204) {',
              '    pm.expect(true).to.be.true; // Always pass for 204 responses',
              '    return;',
              '  }',
              '  ',
              '  // For responses with content, validate JSON structure',
              '  pm.response.to.be.json;',
              '  ',
              '  // Add specific validation based on response schema here',
              '});',
              ''
            ]
          }
        }
      ]
    };
    
    // Add description with uses and outputs
    let description = step.description || '';
    
    if (step.uses && step.uses.length > 0) {
      description += '\n\n**Uses:**';
      step.uses.forEach(use => {
        description += `\n- \`${use.variable}\` from step ${use.step}`;
      });
    }
    
    if (step.outputs && step.outputs.length > 0) {
      description += '\n\n\n**Outputs:**';
      step.outputs.forEach(output => {
        description += `\n- \`${output}\``;
      });
    }
    
    workflowRequest.description = description;
    
    // Add test script to extract variables from response if there are outputs
    if (step.outputs && step.outputs.length > 0) {
      addTestScript(workflowRequest, step.outputs);
    }
    
    // Add the request to the workflow folder
    workflowFolder.item.push(workflowRequest);
    return;
  }
  
  // Find the request in the collection
  const request = findRequestInCollection(collection, step.path, step.method);
  
  if (!request) {
    console.warn(`Warning: Could not find request for ${step.method} ${step.path}`);
    return;
  }
  
  // Create a new request object with the correct structure
  const workflowRequest = {
    name: `${step.number}. ${step.title}`,
    request: JSON.parse(JSON.stringify(request.request)),
    response: [],
    event: request.event ? JSON.parse(JSON.stringify(request.event)) : []
  };
  
  // Add description with uses and outputs
  let description = step.description || '';
  
  if (step.uses && step.uses.length > 0) {
    description += '\n\n**Uses:**';
    step.uses.forEach(use => {
      description += `\n- \`${use.variable}\` from step ${use.step}`;
    });
  }
  
  if (step.outputs && step.outputs.length > 0) {
    description += '\n\n\n**Outputs:**';
    step.outputs.forEach(output => {
      description += `\n- \`${output}\``;
    });
  }
  
  workflowRequest.description = description;
  
  // Update the URL to use the correct path with variables
  if (workflowRequest.request && workflowRequest.request.url) {
    // Fix the URL to include the full path with variables
    updateRequestUrl(workflowRequest.request, step);
  }
  
  // Add test script to extract variables from response if there are outputs
  if (step.outputs && step.outputs.length > 0) {
    addTestScript(workflowRequest, step.outputs);
  }
  
  // Add the request to the workflow folder
  workflowFolder.item.push(workflowRequest);
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
    addRequestToWorkflow(collection, workflowFolder, step);
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
  const steps = parseWorkflowStepsFromMarkdown(workflowMd);
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
