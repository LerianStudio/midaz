#!/usr/bin/env node

/**
 * Enhanced Workflow Generator for CI Regression Testing
 * Simplified version without template literal issues
 */

const fs = require('fs');
const path = require('path');
const { v4: uuidv4 } = require('uuid');

/**
 * Generate comprehensive validation script for CI regression testing
 */
function generateEnhancedTestScript(operation, path, method, outputs, stepNumber, stepTitle) {
  const scripts = [];
  
  // 1. Basic status code validation with detailed error reporting
  scripts.push('\n// ===== STEP ' + stepNumber + ': ' + stepTitle + ' =====\n' +
    'console.log("🔍 Executing Step ' + stepNumber + ': ' + stepTitle + '");\n\n' +
    '// Record start time for performance tracking\n' +
    'pm.globals.set("step_' + stepNumber + '_start", Date.now());');

  // 2. Enhanced status code validation
  if (method === 'POST') {
    scripts.push('\npm.test("✅ Status: POST request successful (200/201)", function () {\n' +
      '    pm.expect(pm.response.code).to.be.oneOf([200, 201]);\n' +
      '    if (pm.response.code === 201) {\n' +
      '        console.log("✅ Resource created successfully");\n' +
      '    } else {\n' +
      '        console.log("✅ POST operation completed successfully");\n' +
      '    }\n' +
      '});');
  } else if (method === 'DELETE') {
    scripts.push('\npm.test("✅ Status: DELETE request successful (204)", function () {\n' +
      '    pm.expect(pm.response.code).to.equal(204);\n' +
      '    console.log("✅ Resource deleted successfully");\n' +
      '});');
  } else if (method === 'HEAD') {
    scripts.push('\npm.test("✅ Status: HEAD request successful (204)", function () {\n' +
      '    pm.expect(pm.response.code).to.equal(204);\n' +
      '    console.log("✅ HEAD operation completed successfully - Count: " + pm.response.headers.get("X-Total-Count"));\n' +
      '});');
  } else {
    scripts.push('\npm.test("✅ Status: ' + method + ' request successful (200)", function () {\n' +
      '    pm.expect(pm.response.code).to.equal(200);\n' +
      '    console.log("✅ ' + method + ' operation completed successfully");\n' +
      '});');
  }

  // 3. Response time performance tracking
  scripts.push('\npm.test("⚡ Performance: Response time acceptable", function () {\n' +
    '    const responseTime = pm.response.responseTime;\n' +
    '    const maxTime = pm.environment.get("max_response_time") || 5000;\n' +
    '    \n' +
    '    pm.expect(responseTime).to.be.below(maxTime);\n' +
    '    console.log("⚡ Response time: " + responseTime + "ms (max: " + maxTime + "ms)");\n' +
    '    \n' +
    '    // Track performance for regression detection\n' +
    '    const perfKey = "perf_step_' + stepNumber + '";\n' +
    '    const previousTime = pm.environment.get(perfKey);\n' +
    '    pm.environment.set(perfKey, responseTime);\n' +
    '    \n' +
    '    if (previousTime) {\n' +
    '        const increase = ((responseTime - previousTime) / previousTime) * 100;\n' +
    '        if (increase > 50) {\n' +
    '            console.warn("⚠️ Performance regression: " + increase.toFixed(1) + "% slower than previous run");\n' +
    '        }\n' +
    '    }\n' +
    '});');

  // 4. Response structure validation (only for non-DELETE and non-HEAD)
  if (method !== 'DELETE' && method !== 'HEAD') {
    scripts.push('\npm.test("📋 Structure: Response has valid JSON structure", function() {\n' +
      '    pm.response.to.be.json;\n' +
      '    \n' +
      '    const jsonData = pm.response.json();\n' +
      '    pm.expect(jsonData).to.be.an(\'object\');\n' +
      '    \n' +
      '    // Log response structure for debugging\n' +
      '    console.log("📋 Response structure keys:", Object.keys(jsonData));\n' +
      '});');

    // 5. Business logic validation for specific endpoints
    if (path.includes('/organizations') && method === 'POST' && !path.includes('/ledgers')) {
      scripts.push('\npm.test("🏢 Business Logic: Organization has required fields", function() {\n' +
        '    const jsonData = pm.response.json();\n' +
        '    const requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    \n' +
        '    // Required fields validation\n' +
        '    pm.expect(jsonData).to.have.property(\'id\');\n' +
        '    pm.expect(jsonData).to.have.property(\'legalName\');\n' +
        '    pm.expect(jsonData).to.have.property(\'status\');\n' +
        '    pm.expect(jsonData).to.have.property(\'createdAt\');\n' +
        '    pm.expect(jsonData).to.have.property(\'updatedAt\');\n' +
        '    \n' +
        '    // UUID format validation\n' +
        '    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;\n' +
        '    pm.expect(jsonData.id).to.match(uuidRegex, "Organization ID should be a valid UUID");\n' +
        '    \n' +
        '    // ISO timestamp format validation\n' +
        '    const isoTimestampRegex = /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d+)?Z$/;\n' +
        '    pm.expect(jsonData.createdAt).to.match(isoTimestampRegex, "Organization createdAt should be ISO timestamp");\n' +
        '    pm.expect(jsonData.updatedAt).to.match(isoTimestampRegex, "Organization updatedAt should be ISO timestamp");\n' +
        '    \n' +
        '    // Data consistency validation\n' +
        '    if (requestData.legalName) {\n' +
        '        pm.expect(jsonData.legalName).to.equal(requestData.legalName, "Response legalName should match request");\n' +
        '    }\n' +
        '    \n' +
        '    // Store organization ID for subsequent requests\n' +
        '    pm.environment.set("organizationId", jsonData.id);\n' +
        '    \n' +
        '    console.log("🏢 Organization creation validation passed:", jsonData.legalName);\n' +
        '    console.log("✅ Required fields verified, UUID and timestamp validation completed");\n' +
        '    console.log("💾 Stored organizationId:", jsonData.id);\n' +
        '});');
    }

    // Ledger creation validation and ID extraction (only for direct ledger creation, not sub-resources)
    if (path.includes('/ledgers') && method === 'POST' && !path.includes('/accounts') && !path.includes('/assets') && !path.includes('/portfolios') && !path.includes('/segments') && !path.includes('/transactions')) {
      scripts.push('\npm.test("📒 Business Logic: Ledger has required fields", function() {\n' +
        '    // Only validate if response was successful\n' +
        '    if (pm.response.code !== 200 && pm.response.code !== 201) {\n' +
        '        console.log("⚠️ Skipping ledger validation - response code:", pm.response.code);\n' +
        '        pm.test.skip();\n' +
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    const requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    \n' +
        '    // Required fields validation\n' +
        '    pm.expect(jsonData).to.have.property(\'id\');\n' +
        '    pm.expect(jsonData).to.have.property(\'name\');\n' +
        '    pm.expect(jsonData).to.have.property(\'status\');\n' +
        '    pm.expect(jsonData).to.have.property(\'createdAt\');\n' +
        '    pm.expect(jsonData).to.have.property(\'updatedAt\');\n' +
        '    \n' +
        '    // UUID format validation\n' +
        '    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;\n' +
        '    pm.expect(jsonData.id).to.match(uuidRegex, "Ledger ID should be a valid UUID");\n' +
        '    \n' +
        '    // ISO timestamp format validation\n' +
        '    const isoTimestampRegex = /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d+)?Z$/;\n' +
        '    pm.expect(jsonData.createdAt).to.match(isoTimestampRegex, "Ledger createdAt should be ISO timestamp");\n' +
        '    pm.expect(jsonData.updatedAt).to.match(isoTimestampRegex, "Ledger updatedAt should be ISO timestamp");\n' +
        '    \n' +
        '    // Store ledger ID for subsequent requests\n' +
        '    pm.environment.set("ledgerId", jsonData.id);\n' +
        '    \n' +
        '    console.log("📒 Ledger creation validation passed:", jsonData.name);\n' +
        '    console.log("✅ Required fields verified, UUID and timestamp validation completed");\n' +
        '    console.log("💾 Stored ledgerId:", jsonData.id);\n' +
        '});');
    }

    // Portfolio creation validation and ID extraction
    if (path.includes('/portfolios') && method === 'POST') {
      scripts.push('\npm.test("📁 Business Logic: Portfolio has required fields", function() {\n' +
        '    // Only validate if response was successful\n' +
        '    if (pm.response.code !== 200 && pm.response.code !== 201) {\n' +
        '        console.log("⚠️ Skipping portfolio validation - response code:", pm.response.code);\n' +
        '        pm.test.skip();\n' +
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    const requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    \n' +
        '    // Required fields validation\n' +
        '    pm.expect(jsonData).to.have.property(\'id\');\n' +
        '    pm.expect(jsonData).to.have.property(\'name\');\n' +
        '    pm.expect(jsonData).to.have.property(\'status\');\n' +
        '    pm.expect(jsonData).to.have.property(\'createdAt\');\n' +
        '    pm.expect(jsonData).to.have.property(\'updatedAt\');\n' +
        '    \n' +
        '    // UUID format validation\n' +
        '    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;\n' +
        '    pm.expect(jsonData.id).to.match(uuidRegex, "Portfolio ID should be a valid UUID");\n' +
        '    \n' +
        '    // ISO timestamp format validation\n' +
        '    const isoTimestampRegex = /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d+)?Z$/;\n' +
        '    pm.expect(jsonData.createdAt).to.match(isoTimestampRegex, "Portfolio createdAt should be ISO timestamp");\n' +
        '    pm.expect(jsonData.updatedAt).to.match(isoTimestampRegex, "Portfolio updatedAt should be ISO timestamp");\n' +
        '    \n' +
        '    // Store portfolio ID for subsequent requests\n' +
        '    pm.environment.set("portfolioId", jsonData.id);\n' +
        '    \n' +
        '    console.log("📁 Portfolio creation validation passed:", jsonData.name);\n' +
        '    console.log("✅ Required fields verified, UUID and timestamp validation completed");\n' +
        '    console.log("💾 Stored portfolioId:", jsonData.id);\n' +
        '});');
    }

    // Account creation validation and ID extraction
    if (path.includes('/accounts') && method === 'POST') {
      scripts.push('\npm.test("👤 Business Logic: Account has required fields", function() {\n' +
        '    // Only validate if response was successful\n' +
        '    if (pm.response.code !== 200 && pm.response.code !== 201) {\n' +
        '        console.log("⚠️ Skipping account validation - response code:", pm.response.code);\n' +
        '        pm.test.skip();\n' +
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    const requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    \n' +
        '    // Required fields validation\n' +
        '    pm.expect(jsonData).to.have.property(\'id\');\n' +
        '    pm.expect(jsonData).to.have.property(\'name\');\n' +
        '    pm.expect(jsonData).to.have.property(\'status\');\n' +
        '    pm.expect(jsonData).to.have.property(\'createdAt\');\n' +
        '    pm.expect(jsonData).to.have.property(\'updatedAt\');\n' +
        '    \n' +
        '    // UUID format validation\n' +
        '    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;\n' +
        '    pm.expect(jsonData.id).to.match(uuidRegex, "Account ID should be a valid UUID");\n' +
        '    \n' +
        '    // ISO timestamp format validation\n' +
        '    const isoTimestampRegex = /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d+)?Z$/;\n' +
        '    pm.expect(jsonData.createdAt).to.match(isoTimestampRegex, "Account createdAt should be ISO timestamp");\n' +
        '    pm.expect(jsonData.updatedAt).to.match(isoTimestampRegex, "Account updatedAt should be ISO timestamp");\n' +
        '    \n' +
        '    // Store account ID and alias for subsequent requests\n' +
        '    pm.environment.set("accountId", jsonData.id);\n' +
        '    if (jsonData.alias) {\n' +
        '        pm.environment.set("accountAlias", jsonData.alias);\n' +
        '        console.log("💾 Stored accountAlias:", jsonData.alias);\n' +
        '    }\n' +
        '    \n' +
        '    console.log("👤 Account creation validation passed:", jsonData.name);\n' +
        '    console.log("✅ Required fields verified, UUID and timestamp validation completed");\n' +
        '    console.log("💾 Stored accountId:", jsonData.id);\n' +
        '});');
    }

    // Asset creation validation and ID extraction
    if (path.includes('/assets') && method === 'POST') {
      scripts.push('\npm.test("💰 Business Logic: Asset has required fields", function() {\n' +
        '    // Only validate if response was successful\n' +
        '    if (pm.response.code !== 200 && pm.response.code !== 201) {\n' +
        '        console.log("⚠️ Skipping asset validation - response code:", pm.response.code);\n' +
        '        pm.test.skip();\n' +
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    const requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    \n' +
        '    // Required fields validation\n' +
        '    pm.expect(jsonData).to.have.property(\'id\');\n' +
        '    pm.expect(jsonData).to.have.property(\'name\');\n' +
        '    pm.expect(jsonData).to.have.property(\'code\');\n' +
        '    pm.expect(jsonData).to.have.property(\'status\');\n' +
        '    pm.expect(jsonData).to.have.property(\'createdAt\');\n' +
        '    pm.expect(jsonData).to.have.property(\'updatedAt\');\n' +
        '    \n' +
        '    // UUID format validation\n' +
        '    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;\n' +
        '    pm.expect(jsonData.id).to.match(uuidRegex, "Asset ID should be a valid UUID");\n' +
        '    \n' +
        '    // ISO timestamp format validation\n' +
        '    const isoTimestampRegex = /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d+)?Z$/;\n' +
        '    pm.expect(jsonData.createdAt).to.match(isoTimestampRegex, "Asset createdAt should be ISO timestamp");\n' +
        '    pm.expect(jsonData.updatedAt).to.match(isoTimestampRegex, "Asset updatedAt should be ISO timestamp");\n' +
        '    \n' +
        '    // Store asset ID for subsequent requests\n' +
        '    pm.environment.set("assetId", jsonData.id);\n' +
        '    \n' +
        '    console.log("💰 Asset creation validation passed:", jsonData.name);\n' +
        '    console.log("✅ Required fields verified, UUID and timestamp validation completed");\n' +
        '    console.log("💾 Stored assetId:", jsonData.id);\n' +
        '});');
    }

    // Segment creation validation and ID extraction
    if (path.includes('/segments') && method === 'POST') {
      scripts.push('\npm.test("🏷️ Business Logic: Segment has required fields", function() {\n' +
        '    // Only validate if response was successful\n' +
        '    if (pm.response.code !== 200 && pm.response.code !== 201) {\n' +
        '        console.log("⚠️ Skipping segment validation - response code:", pm.response.code);\n' +
        '        pm.test.skip();\n' +
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    const requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    \n' +
        '    // Required fields validation\n' +
        '    pm.expect(jsonData).to.have.property(\'id\');\n' +
        '    pm.expect(jsonData).to.have.property(\'name\');\n' +
        '    pm.expect(jsonData).to.have.property(\'status\');\n' +
        '    pm.expect(jsonData).to.have.property(\'createdAt\');\n' +
        '    pm.expect(jsonData).to.have.property(\'updatedAt\');\n' +
        '    \n' +
        '    // UUID format validation\n' +
        '    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;\n' +
        '    pm.expect(jsonData.id).to.match(uuidRegex, "Segment ID should be a valid UUID");\n' +
        '    \n' +
        '    // ISO timestamp format validation\n' +
        '    const isoTimestampRegex = /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d+)?Z$/;\n' +
        '    pm.expect(jsonData.createdAt).to.match(isoTimestampRegex, "Segment createdAt should be ISO timestamp");\n' +
        '    pm.expect(jsonData.updatedAt).to.match(isoTimestampRegex, "Segment updatedAt should be ISO timestamp");\n' +
        '    \n' +
        '    // Store segment ID for subsequent requests\n' +
        '    pm.environment.set("segmentId", jsonData.id);\n' +
        '    \n' +
        '    console.log("🏷️ Segment creation validation passed:", jsonData.name);\n' +
        '    console.log("✅ Required fields verified, UUID and timestamp validation completed");\n' +
        '    console.log("💾 Stored segmentId:", jsonData.id);\n' +
        '});');
    }

    // Transaction creation validation and ID extraction (only for successful 200/201 responses)
    if (path.includes('/transactions') && method === 'POST') {
      scripts.push('\npm.test("💸 Business Logic: Transaction has required fields", function() {\n' +
        '    // Only validate if response was successful\n' +
        '    if (pm.response.code !== 200 && pm.response.code !== 201) {\n' +
        '        console.log("⚠️ Skipping transaction validation - response code:", pm.response.code);\n' +
        '        pm.test.skip();\n' +
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    \n' +
        '    // Required fields validation\n' +
        '    pm.expect(jsonData).to.have.property(\'id\');\n' +
        '    pm.expect(jsonData).to.have.property(\'status\');\n' +
        '    pm.expect(jsonData).to.have.property(\'createdAt\');\n' +
        '    pm.expect(jsonData).to.have.property(\'updatedAt\');\n' +
        '    \n' +
        '    // UUID format validation\n' +
        '    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;\n' +
        '    pm.expect(jsonData.id).to.match(uuidRegex, "Transaction ID should be a valid UUID");\n' +
        '    \n' +
        '    // ISO timestamp format validation\n' +
        '    const isoTimestampRegex = /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d+)?Z$/;\n' +
        '    pm.expect(jsonData.createdAt).to.match(isoTimestampRegex, "Transaction createdAt should be ISO timestamp");\n' +
        '    pm.expect(jsonData.updatedAt).to.match(isoTimestampRegex, "Transaction updatedAt should be ISO timestamp");\n' +
        '    \n' +
        '    // Determine transaction type and set appropriate variable names\n' +
        '    const currentUrl = pm.request.url.toString();\n' +
        '    let varPrefix = "";\n' +
        '    \n' +
        '    if (currentUrl.includes("/transactions/json")) {\n' +
        '        varPrefix = ""; // Standard transaction - use original variable names\n' +
        '    } else if (currentUrl.includes("/transactions/dsl")) {\n' +
        '        varPrefix = "dsl"; // DSL transaction\n' +
        '    } else if (currentUrl.includes("/transactions/inflow")) {\n' +
        '        varPrefix = "inflow"; // Inflow transaction\n' +
        '    } else if (currentUrl.includes("/transactions/outflow")) {\n' +
        '        varPrefix = "outflow"; // Outflow transaction\n' +
        '    }\n' +
        '    \n' +
        '    // Store transaction ID with appropriate prefix\n' +
        '    const transactionIdVar = varPrefix ? varPrefix + "TransactionId" : "transactionId";\n' +
        '    const operationIdVar = varPrefix ? varPrefix + "OperationId" : "operationId";\n' +
        '    const balanceIdVar = varPrefix ? varPrefix + "BalanceId" : "balanceId";\n' +
        '    \n' +
        '    pm.environment.set(transactionIdVar, jsonData.id);\n' +
        '    console.log("💾 Stored " + transactionIdVar + ":", jsonData.id);\n' +
        '    \n' +
        '    // Extract operation and balance IDs if available\n' +
        '    if (jsonData.operations && jsonData.operations.length > 0) {\n' +
        '        pm.environment.set(operationIdVar, jsonData.operations[0].id);\n' +
        '        console.log("💾 Stored " + operationIdVar + ":", jsonData.operations[0].id);\n' +
        '        \n' +
        '        if (jsonData.operations[0].balanceId) {\n' +
        '            pm.environment.set(balanceIdVar, jsonData.operations[0].balanceId);\n' +
        '            console.log("💾 Stored " + balanceIdVar + ":", jsonData.operations[0].balanceId);\n' +
        '        }\n' +
        '    }\n' +
        '    \n' +
        '    console.log("💸 Transaction creation validation passed:", jsonData.id);\n' +
        '    console.log("✅ Required fields verified, UUID and timestamp validation completed");\n' +
        '});');
    }
  }

  // Return combined scripts
  return scripts.join('\n');
}

/**
 * Generate enhanced pre-request script for environment setup and validation
 */
function generateEnhancedPreRequestScript(stepNumber, stepTitle) {
  return `
// ===== PRE-REQUEST STEP ${stepNumber}: ${stepTitle} =====
console.log("⚙️ Setting up Step ${stepNumber}: ${stepTitle}");

// Set step start timestamp for performance tracking
pm.globals.set("step_${stepNumber}_start", Date.now());

// Generate unique idempotency key for each POST/PUT request
if (pm.request.method === 'POST' || pm.request.method === 'PUT') {
    // Always generate a new unique idempotency key for each transaction
    const newIdempotencyKey = pm.variables.replaceIn('{{$guid}}');
    pm.environment.set('idempotencyKey', newIdempotencyKey);
    console.log('🔑 Generated new idempotency key:', newIdempotencyKey);
}

// Check for required variables based on operation type
const requestUrl = pm.request.url.toString();
const method = pm.request.method;

// Base required variables
const requiredVars = ['organizationId', 'ledgerId'];

// Add specific variables based on the operation
if (requestUrl.includes('/transactions/') && (method === 'PATCH' || method === 'GET')) {
    if (requestUrl.includes('{{transactionId}}') || requestUrl.includes('{transaction_id}')) {
        requiredVars.push('transactionId');
    }
}

if (requestUrl.includes('/operations/') && (method === 'PATCH' || method === 'GET')) {
    if (requestUrl.includes('{{operationId}}') || requestUrl.includes('{operation_id}')) {
        requiredVars.push('operationId');
    }
}

if (requestUrl.includes('/balances/') && (method === 'PATCH' || method === 'DELETE' || method === 'GET')) {
    if (requestUrl.includes('{{balanceId}}') || requestUrl.includes('{balance_id}')) {
        requiredVars.push('balanceId');
    }
}

if (requestUrl.includes('/accounts/') && (method === 'PATCH' || method === 'DELETE' || method === 'GET')) {
    if (requestUrl.includes('{{accountId}}') || requestUrl.includes('{account_id}')) {
        requiredVars.push('accountId');
    }
}

// Check all required variables
let hasAllRequiredVars = true;
requiredVars.forEach(varName => {
    const value = pm.environment.get(varName);
    if (!value || value === '') {
        console.error("❌ CRITICAL: Required environment variable '" + varName + "' is not set for " + method + " operation");
        hasAllRequiredVars = false;
    } else {
        console.log("✅ " + varName + ": " + value);
    }
});

if (!hasAllRequiredVars) {
    console.error("❌ Missing required variables - this request will likely fail with 404/405 error");
    console.log("💡 Suggestion: Ensure previous steps completed successfully and extracted required IDs");
}

console.log("✅ Pre-request setup completed for Step ${stepNumber}");
`;
}

/**
 * Generate workflow summary script for final validation
 */
function generateWorkflowSummaryScript(totalSteps) {
  return `
// ===== WORKFLOW SUMMARY =====
console.log("📊 Workflow Execution Summary");
console.log("Total Steps: ${totalSteps}");

// Calculate total execution time
const startTime = pm.globals.get("workflow_start_time");
if (startTime) {
    const totalTime = Date.now() - startTime;
    console.log("⏱️ Total Execution Time: " + totalTime + "ms");
    pm.globals.set("workflow_total_time", totalTime);
}

// Summary of step performance
console.log("📈 Performance Summary:");
for (let i = 1; i <= ${totalSteps}; i++) {
    const stepTime = pm.environment.get("perf_step_" + i);
    if (stepTime) {
        console.log("  Step " + i + ": " + stepTime + "ms");
    }
}

console.log("✅ Workflow completed successfully");
`;
}

module.exports = {
  generateEnhancedTestScript,
  generateEnhancedPreRequestScript,
  generateWorkflowSummaryScript
};