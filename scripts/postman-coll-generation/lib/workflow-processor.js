#!/usr/bin/env node

/**
 * WorkflowProcessor Class
 * 
 * Main orchestration class that coordinates all components to generate
 * the complete workflow from markdown and collection inputs.
 */

const { v4: uuidv4 } = require('uuid');
const { MarkdownParser } = require('./markdown-parser');
const { RequestMatcher } = require('./request-matcher');
const { VariableMapper } = require('./variable-mapper');
const { PathResolver } = require('./path-resolver');
const { RequestBodyGenerator } = require('./request-body-generator');
const { generateEnhancedTestScript, generateEnhancedPreRequestScript, generateWorkflowSummaryScript } = require('../enhance-tests');

class WorkflowProcessor {
  constructor(config) {
    this.config = config;
    this.markdownParser = new MarkdownParser(config);
    this.requestMatcher = new RequestMatcher(config);
    this.variableMapper = new VariableMapper(config);
    this.pathResolver = new PathResolver(config);
    this.bodyGenerator = new RequestBodyGenerator(config);
  }

  /**
   * Process collection and markdown to generate workflow
   * @param {Object} collection - Postman collection
   * @param {string} markdown - Workflow markdown content
   * @returns {Object} Complete workflow folder
   */
  async process(collection, markdown) {
    console.log('🚀 Starting workflow processing...');

    // Step 1: Parse workflow steps
    const steps = this.markdownParser.parse(markdown);
    console.log(`📋 Parsed ${steps.length} steps from markdown`);

    // Step 2: Process each step
    const workflowItems = [];
    let notFoundCount = 0;
    const missingSteps = [];

    for (const [index, step] of steps.entries()) {
      console.log(`\n🔄 Processing Step ${index + 1}: ${step.title} (${step.method} ${step.path})`);
      
      try {
        const item = await this.processStep(step, collection);
        if (item) {
          workflowItems.push(item);
        } else {
          notFoundCount++;
          missingSteps.push({
            number: step.number,
            title: step.title,
            method: step.method,
            path: step.path
          });
          
          // Create placeholder
          const placeholder = this.createPlaceholder(step);
          workflowItems.push(placeholder);
        }
      } catch (error) {
        console.error(`❌ Error processing step ${step.number}: ${error.message}`);
        const placeholder = this.createPlaceholder(step, error);
        workflowItems.push(placeholder);
        notFoundCount++;
      }
    }

    // Step 3: Create workflow folder
    const workflowFolder = this.createWorkflowFolder(workflowItems, steps);

    // Step 4: Add summary information
    if (notFoundCount > 0) {
      console.warn(`\n⚠️ WARNING: ${notFoundCount} requests were not found in the collection`);
      missingSteps.forEach(step => {
        console.warn(`  - Step ${step.number}: ${step.title} (${step.method} ${step.path})`);
      });
    }

    console.log(`\n✅ Workflow processing complete! Generated ${workflowItems.length} workflow items`);
    return workflowFolder;
  }

  /**
   * Process a single workflow step
   * @param {Object} step - Step object
   * @param {Object} collection - Postman collection
   * @returns {Object|null} Processed workflow item or null
   */
  async processStep(step, collection) {
    // Find the original request in the collection
    const collectionItemsToSearch = collection.item.filter(item => item.name !== "Complete API Workflow");
    const originalRequest = await this.requestMatcher.find(step.method, step.path, { item: collectionItemsToSearch });

    if (!originalRequest) {
      console.warn(`❌ No matching request found for: ${step.method} ${step.path}`);
      return null;
    }

    // Clone and transform the request
    const workflowItem = this.cloneRequest(originalRequest);
    
    // Update metadata
    workflowItem.name = `${step.number}. ${step.title}`;
    workflowItem.request.description = this.generateDescription(step);

    // Transform URL
    this.transformUrl(workflowItem, step);

    // Transform body if needed
    if (this.needsBodyTransformation(step)) {
      this.transformBody(workflowItem, step);
    }

    // Add enhanced test scripts
    this.addTestScripts(workflowItem, step);

    // Handle special cases
    this.handleSpecialCases(workflowItem, step);

    return workflowItem;
  }

  /**
   * Deep clone a request object
   * @param {Object} original - Original request
   * @returns {Object} Cloned request
   */
  cloneRequest(original) {
    return JSON.parse(JSON.stringify(original));
  }

  /**
   * Generate description for workflow step
   * @param {Object} step - Step object
   * @returns {string} Generated description
   */
  generateDescription(step) {
    let description = `**Workflow Step ${step.number}: ${step.title}**\n\n${step.description || 'No description provided in Markdown.'}`;
    
    if (step.uses && step.uses.length > 0) {
      description += '\n\n---\n\n**Uses:**\n';
      step.uses.forEach(use => {
        const varName = typeof use === 'string' ? use : use.variable;
        description += `- \`${varName}\`\n`;
      });
    }
    
    return description;
  }

  /**
   * Transform URL in workflow item
   * @param {Object} workflowItem - Workflow item to transform
   * @param {Object} step - Step object
   */
  transformUrl(workflowItem, step) {
    if (!workflowItem.request.url || !workflowItem.request.url.path) {
      return;
    }

    // Map variables in path
    const mappedPath = this.variableMapper.mapPath(step.path, workflowItem.request.url.path);
    workflowItem.request.url.path = mappedPath;

    // Update raw URL
    const baseUrl = this.pathResolver.determineBaseUrl(step.path);
    const fullPath = '/' + mappedPath.join('/');
    workflowItem.request.url.raw = baseUrl + fullPath;
  }

  /**
   * Check if step needs body transformation
   * @param {Object} step - Step object
   * @returns {boolean} True if body transformation is needed
   */
  needsBodyTransformation(step) {
    return this.bodyGenerator.needsBody(step);
  }

  /**
   * Transform request body
   * @param {Object} workflowItem - Workflow item to transform
   * @param {Object} step - Step object
   */
  transformBody(workflowItem, step) {
    const body = this.bodyGenerator.generate(step);
    
    if (body) {
      workflowItem.request.body = {
        mode: 'raw',
        raw: JSON.stringify(body, null, 2),
        options: {
          raw: {
            language: 'json'
          }
        }
      };
    }
  }

  /**
   * Add enhanced test scripts
   * @param {Object} workflowItem - Workflow item
   * @param {Object} step - Step object
   */
  addTestScripts(workflowItem, step) {
    const requires = (step.uses || []).map(use => typeof use === 'string' ? use : use.variable);
    
    // Generate enhanced test script
    const testScript = generateEnhancedTestScript(
      step.title,
      step.path,
      step.method,
      step.outputs,
      step.number,
      step.title
    );

    // Generate pre-request script
    const preRequestScript = generateEnhancedPreRequestScript(step.number, step.title);

    // Add test event
    if (!workflowItem.event) {
      workflowItem.event = [];
    }

    // Remove existing test events
    workflowItem.event = workflowItem.event.filter(e => e.listen !== 'test' && e.listen !== 'prerequest');

    // Add new events
    workflowItem.event.push({
      listen: 'test',
      script: {
        id: uuidv4(),
        exec: [testScript],
        type: 'text/javascript'
      }
    });

    workflowItem.event.push({
      listen: 'prerequest',
      script: {
        id: uuidv4(),
        exec: [preRequestScript],
        type: 'text/javascript'
      }
    });
  }

  /**
   * Handle special cases
   * @param {Object} workflowItem - Workflow item
   * @param {Object} step - Step object
   */
  handleSpecialCases(workflowItem, step) {
    if (step.title === "Check Account Balance Before Zeroing") {
      this.handleBalanceExtraction(workflowItem);
    }
  }

  /**
   * Handle balance extraction for zero-out step
   * @param {Object} workflowItem - Workflow item
   */
  handleBalanceExtraction(workflowItem) {
    // Find test event and add balance extraction logic
    const testEvent = workflowItem.event.find(e => e.listen === 'test');
    if (testEvent && testEvent.script && testEvent.script.exec) {
      const balanceExtractionScript = `
// Extract balance information for zero-out transaction
if (pm.response.code === 200) {
    const responseJson = pm.response.json();
    const debug = pm.environment.get("debug_logs") === "true";

    // Log only item count to avoid leaking sensitive financial data in CI logs
    console.log("🏦 Balance items found:", (responseJson.items || []).length);
    if (debug) {
        console.log("🔍 [DEBUG] Balance response keys:", Object.keys(responseJson));
    }

    if (responseJson.items && responseJson.items.length > 0) {
        const balance = responseJson.items[0];
        if (balance.available !== undefined) {
            const balanceAmount = Math.abs(balance.available);
            pm.environment.set("currentBalanceAmount", balanceAmount);
            console.log("💰 Extracted balance amount:", balanceAmount);
            console.log("✅ Balance amount variable set for zero-out transaction");
        } else {
            console.warn("⚠️ No balance amount found in response");
            pm.environment.set("currentBalanceAmount", 0);
        }
    } else {
        console.warn("⚠️ No balance items found in response");
        pm.environment.set("currentBalanceAmount", 0);
    }
}`;
      
      testEvent.script.exec[0] += balanceExtractionScript;
    }
  }

  /**
   * Create placeholder for missing request
   * @param {Object} step - Step object
   * @param {Error} [error] - Optional error object
   * @returns {Object} Placeholder item
   */
  createPlaceholder(step, error = null) {
    const baseUrl = this.pathResolver.determineBaseUrl(step.path);
    
    return {
      name: `[NOT FOUND] ${step.title}`,
      request: {
        method: step.method,
        url: { raw: `${baseUrl}${step.path}` },
        description: `Placeholder for missing request: ${step.method} ${step.path}${error ? '\nError: ' + error.message : ''}`
      },
      event: [
        {
          listen: "test",
          script: {
            id: uuidv4(),
            exec: [
              `// Request not found in collection for: ${step.method} ${step.path}`,
              `// Variables to extract: ${JSON.stringify(step.outputs)}`,
              error ? `// Error: ${error.message}` : ''
            ].filter(Boolean),
            type: "text/javascript"
          }
        }
      ]
    };
  }

  /**
   * Create workflow folder with all items
   * @param {Array} workflowItems - Array of workflow items
   * @param {Array} steps - Original steps for summary
   * @returns {Object} Workflow folder object
   */
  createWorkflowFolder(workflowItems, steps) {
    const workflowFolder = {
      name: "Complete API Workflow",
      description: "A sequence of API calls representing a typical workflow, generated from WORKFLOW.md.",
      item: workflowItems,
      _postman_id: uuidv4(),
      event: []
    };

    // Add workflow summary step
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
            exec: [generateWorkflowSummaryScript(steps.length)],
            type: "text/javascript"
          }
        }
      ]
    };

    workflowFolder.item.push(summaryStep);
    return workflowFolder;
  }

  /**
   * Get processing statistics
   * @returns {Object} Processing statistics
   */
  getStatistics() {
    return {
      matcherStats: this.requestMatcher.getStatistics(),
      variableMapperStats: this.variableMapper.getStatistics()
    };
  }
}

module.exports = { WorkflowProcessor };