#!/usr/bin/env node

/**
 * @file Midaz API Workflow Generator v2
 * @description
 * This script is a modernized, modular version of the workflow generator that
 * creates a Postman collection workflow from a Markdown definition. It maintains
 * 100% compatibility with the original implementation while providing a more
 * maintainable and configuration-driven architecture.
 *
 * @usage node create-workflow.js <input-collection> <workflow-md> <output-collection>
 * @example node create-workflow.js ./postman/MIDAZ.postman_collection.json ./postman/WORKFLOW.md ./postman/MIDAZ.postman_collection.json
 */

const fs = require('fs');
const path = require('path');
const config = require('./config/workflow.config');
const { WorkflowProcessor } = require('./lib/workflow-processor');
const { ValidationError, ParseError } = require('./lib/markdown-parser');

// --- Utility Functions ---

/**
 * Reads and parses a JSON file.
 * @param {string} filePath - The path to the JSON file.
 * @returns {Object} The parsed JSON object.
 */
function readJsonFile(filePath) {
  try {
    const data = fs.readFileSync(filePath, 'utf8');
    return JSON.parse(data);
  } catch (error) {
    console.error(`❌ Error reading JSON file ${filePath}: ${error.message}`);
    process.exit(1);
  }
}

/**
 * Reads a text file.
 * @param {string} filePath - The path to the text file.
 * @returns {string} The content of the file.
 */
function readFile(filePath) {
  try {
    return fs.readFileSync(filePath, 'utf8');
  } catch (error) {
    console.error(`❌ Error reading file ${filePath}: ${error.message}`);
    process.exit(1);
  }
}

/**
 * Writes data to a JSON file.
 * @param {string} filePath - The path to the output file.
 * @param {Object} data - The data to write.
 */
function writeJsonFile(filePath, data) {
  try {
    fs.writeFileSync(filePath, JSON.stringify(data, null, 2));
    console.log(`✅ Successfully wrote JSON to ${filePath}`);
  } catch (error) {
    console.error(`❌ Error writing JSON file ${filePath}: ${error.message}`);
    process.exit(1);
  }
}

/**
 * Validates the command-line arguments.
 * @returns {Object} An object containing the file paths.
 */
function validateArguments() {
  if (process.argv.length < 5) {
    console.error(`❌ Usage: node create-workflow-v2.js <collection-file> <workflow-markdown-file> <output-file>`);
    console.error(`   Example: node create-workflow-v2.js ./postman/MIDAZ.postman_collection.json ./postman/WORKFLOW.md ./postman/MIDAZ.postman_collection.json`);
    process.exit(1);
  }

  const [collectionFile, workflowFile, outputFile] = process.argv.slice(2);
  
  // Check if input files exist
  if (!fs.existsSync(collectionFile)) {
    console.error(`❌ Collection file not found: ${collectionFile}`);
    process.exit(1);
  }
  
  if (!fs.existsSync(workflowFile)) {
    console.error(`❌ Workflow markdown file not found: ${workflowFile}`);
    process.exit(1);
  }
  
  return { collectionFile, workflowFile, outputFile };
}

/**
 * Prints the script header to the console.
 */
function printHeader() {
  console.log(`
╔════════════════════════════════════════════════╗
║        Midaz Workflow Generator v2.0           ║
║     🚀 Simplified Modular Architecture         ║
╚════════════════════════════════════════════════╝
`);
}

/**
 * Prints a summary of the workflow generation process.
 * @param {Object} stats - The statistics collected during processing.
 * @param {number} processingTime - The total processing time in milliseconds.
 */
function printSummary(stats, processingTime) {
  console.log(`
╔════════════════════════════════════════════════╗
║                   SUMMARY                      ║
╚════════════════════════════════════════════════╝
⏱️  Processing Time: ${processingTime}ms
📊 Request Matching: ${stats.matcherStats.successRate} success rate
📋 Total Attempts: ${stats.matcherStats.totalAttempts}
✅ Successful: ${stats.matcherStats.successful}
❌ Failed: ${stats.matcherStats.failed}
🔧 Variable Mappings: ${stats.variableMapperStats.totalMappingRules} rules configured
`);

  if (stats.matcherStats.failed > 0) {
    console.log(`❌ Failed Requests:`);
    stats.matcherStats.failedRequests.forEach(req => {
      console.log(`   - ${req.method} ${req.path}`);
    });
  }
}

/**
 * Main execution function for the workflow generator.
 */
async function main() {
  const startTime = Date.now();
  
  try {
    printHeader();
    
    // 1. Validate and parse arguments
    const { collectionFile, workflowFile, outputFile } = validateArguments();
    
    console.log(`📂 Input Collection: ${collectionFile}`);
    console.log(`📝 Workflow Markdown: ${workflowFile}`);
    console.log(`📤 Output File: ${outputFile}`);
    
    // 2. Read input files
    console.log(`\n🔄 Reading input files...`);
    const collection = readJsonFile(collectionFile);
    const workflowMarkdown = readFile(workflowFile);
    
    console.log(`✅ Collection loaded: ${collection.info?.name || 'Unknown'}`);
    console.log(`✅ Markdown loaded: ${Math.round(workflowMarkdown.length / 1024)}KB`);
    
    // 3. Initialize processor with configuration
    console.log(`\n⚙️  Initializing workflow processor...`);
    const processor = new WorkflowProcessor(config);
    
    console.log(`✅ Processor initialized with ${Object.keys(config.variables.mapping.direct).length} direct mappings`);
    console.log(`✅ API patterns: ${config.apiPatterns.pathCorrections.length} corrections configured`);
    console.log(`✅ Transaction templates: ${Object.keys(config.transactions.templates).length} types available`);
    
    // 4. Process workflow
    console.log(`\n🔄 Processing workflow...`);
    const workflowFolder = await processor.process(collection, workflowMarkdown);
    
    // 5. Integrate into collection
    console.log(`\n🔗 Integrating workflow into collection...`);
    
    // Remove existing workflow folder if present
    const existingWorkflowIndex = collection.item.findIndex(item => item.name === "Complete API Workflow");
    if (existingWorkflowIndex !== -1) {
      console.log(`🔄 Replacing existing 'Complete API Workflow' folder`);
      collection.item.splice(existingWorkflowIndex, 1);
    } else {
      console.log(`➕ Adding new 'Complete API Workflow' folder`);
    }
    
    // Add the new folder at the beginning
    collection.item.unshift(workflowFolder);
    
    // 6. Write updated collection
    console.log(`\n💾 Writing updated collection...`);
    writeJsonFile(outputFile, collection);
    
    // 7. Print statistics and summary
    const processingTime = Date.now() - startTime;
    const stats = processor.getStatistics();
    
    printSummary(stats, processingTime);
    
    console.log(`\n🎉 Workflow generation completed successfully!`);
    console.log(`📁 Generated workflow with ${workflowFolder.item.length} steps`);
    
    // Exit with success
    process.exit(0);
    
  } catch (error) {
    const processingTime = Date.now() - startTime;
    console.error(`\n💥 Workflow generation failed after ${processingTime}ms`);
    
    if (error instanceof ValidationError) {
      console.error(`❌ Markdown Validation Failed:`);
      error.issues.forEach((issue, index) => {
        console.error(`   ${index + 1}. ${issue.message}`);
        if (issue.type) {
          console.error(`      Type: ${issue.type}`);
        }
      });
    } else if (error instanceof ParseError) {
      console.error(`❌ Markdown Parse Error: ${error.message}`);
      if (error.context) {
        console.error(`   Context: ${JSON.stringify(error.context, null, 2)}`);
      }
    } else {
      console.error(`❌ Unexpected Error: ${error.message}`);
      if (process.env.DEBUG) {
        console.error(error.stack);
      }
    }
    
    console.error(`\n💡 Troubleshooting Tips:`);
    console.error(`   - Check that the markdown file follows the correct format`);
    console.error(`   - Verify that the collection file is valid JSON`);
    console.error(`   - Ensure all step dependencies are properly defined`);
    console.error(`   - Run with DEBUG=1 for detailed error information`);
    
    process.exit(1);
  }
}

// Handle graceful shutdown
process.on('SIGINT', () => {
  console.log(`\n⏹️  Workflow generation interrupted by user`);
  process.exit(1);
});

process.on('uncaughtException', (error) => {
  console.error(`💥 Uncaught Exception: ${error.message}`);
  if (process.env.DEBUG) {
    console.error(error.stack);
  }
  process.exit(1);
});

process.on('unhandledRejection', (reason) => {
  console.error(`💥 Unhandled Promise Rejection: ${reason}`);
  process.exit(1);
});

// Run main function
if (require.main === module) {
  main();
}

module.exports = { main };