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
        '    let requestData = {};\n' +
        '    try {\n' +
        '        requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    } catch (e) {\n' +
        '        console.warn(\"⚠️ Could not parse request body as JSON; skipping request/response field equality checks\");\n' +
        '    }\n' +
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
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    let requestData = {};\n' +
        '    try {\n' +
        '        requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    } catch (e) {\n' +
        '        console.warn(\"⚠️ Could not parse request body as JSON; skipping request/response field equality checks\");\n' +
        '    }\n' +
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
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    let requestData = {};\n' +
        '    try {\n' +
        '        requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    } catch (e) {\n' +
        '        console.warn(\"⚠️ Could not parse request body as JSON; skipping request/response field equality checks\");\n' +
        '    }\n' +
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
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    let requestData = {};\n' +
        '    try {\n' +
        '        requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    } catch (e) {\n' +
        '        console.warn(\"⚠️ Could not parse request body as JSON; skipping request/response field equality checks\");\n' +
        '    }\n' +
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
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    let requestData = {};\n' +
        '    try {\n' +
        '        requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    } catch (e) {\n' +
        '        console.warn(\"⚠️ Could not parse request body as JSON; skipping request/response field equality checks\");\n' +
        '    }\n' +
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
        '        return;\n' +
        '    }\n' +
        '    \n' +
        '    const jsonData = pm.response.json();\n' +
        '    let requestData = {};\n' +
        '    try {\n' +
        '        requestData = JSON.parse(pm.request.body.raw || \'{}\');\n' +
        '    } catch (e) {\n' +
        '        console.warn(\"⚠️ Could not parse request body as JSON; skipping request/response field equality checks\");\n' +
        '    }\n' +
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
        '    const accountIdVar = varPrefix ? varPrefix + "AccountId" : "accountId";\n' +
        '    \n' +
        '    pm.environment.set(transactionIdVar, jsonData.id);\n' +
        '    console.log("💾 Stored " + transactionIdVar + ":", jsonData.id);\n' +
        '    \n' +
        '    // Extract operation and balance IDs if available\n' +
        '    if (jsonData.operations && jsonData.operations.length > 0) {\n' +
        '        // Find the user operation (non-external account) first\n' +
        '        const userOperation = jsonData.operations.find(op => \n' +
        '            op.accountAlias && !op.accountAlias.startsWith("@external/")  \n' +
        '        ) || jsonData.operations[0]; // fallback to first operation if no user account found\n' +
        '        \n' +
        '        // Extract operation ID from the user operation (not external account)\n' +
        '        if (userOperation && userOperation.id) {\n' +
        '            pm.environment.set(operationIdVar, userOperation.id);\n' +
        '            console.log("💾 Stored " + operationIdVar + " from user account:", userOperation.id);\n' +
        '            console.log("    Operation belongs to account:", userOperation.accountAlias || userOperation.accountId);\n' +
        '        } else {\n' +
        '            // Fallback to first operation if no user operation found\n' +
        '            pm.environment.set(operationIdVar, jsonData.operations[0].id);\n' +
        '            console.log("⚠️ Stored " + operationIdVar + " from first operation (might be external):", jsonData.operations[0].id);\n' +
        '        }\n' +
        '        \n' +
        '        // Extract and store accountId - prefer non-external accounts\n' +
        '        if (userOperation && userOperation.accountId) {\n' +
        '            // Only store if we do not already have an accountId for this variable (preserve treasury account ID)\n' +
        '            const existingAccountId = pm.environment.get(accountIdVar);\n' +
        '            if (!existingAccountId) {\n' +
        '                pm.environment.set(accountIdVar, userOperation.accountId);\n' +
        '                console.log("💾 Stored " + accountIdVar + ":", userOperation.accountId);\n' +
        '            } else {\n' +
        '                console.log("⚠️ Preserving existing " + accountIdVar + ":", existingAccountId, "(not overwriting with:", userOperation.accountId + ")");\n' +
        '            }\n' +
        '        }\n' +
        '        \n' +
        '        // Extract balance ID from user operation as well\n' +
        '        if (userOperation && userOperation.balanceId) {\n' +
        '            pm.environment.set(balanceIdVar, userOperation.balanceId);\n' +
        '            console.log("💾 Stored " + balanceIdVar + " from user account:", userOperation.balanceId);\n' +
        '        } else if (jsonData.operations[0].balanceId) {\n' +
        '            pm.environment.set(balanceIdVar, jsonData.operations[0].balanceId);\n' +
        '            console.log("⚠️ Stored " + balanceIdVar + " from first operation (might be external):", jsonData.operations[0].balanceId);\n' +
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

// Generate unique idempotency key for each POST/PUT/PATCH request
if (['POST', 'PUT', 'PATCH'].includes(pm.request.method)) {
    // Always generate a new unique idempotency key for each transaction
    const newIdempotencyKey = pm.variables.replaceIn('{{$guid}}');
    pm.environment.set('idempotencyKey', newIdempotencyKey);
    console.log('🔑 Generated new idempotency key:', newIdempotencyKey);
}

// Check for required variables based on operation type
const requestUrl = pm.request.url.toString();
const method = pm.request.method;

// Parse URL path segments for more robust variable detection
// pm.request.url.path is an array like ["v1", "organizations", "{{organizationId}}", "ledgers"]
const pathSegments = pm.request.url.path || [];

// Helper: Check if a path segment is a template variable for a specific ID
function hasTemplateVar(segments, varPatterns) {
    return segments.some(seg => varPatterns.some(pattern => seg.includes(pattern)));
}

// Helper: Check if resource ID is in a path position (after the resource name, not just listed)
// e.g., /organizations/{{id}}/ledgers requires organizationId, but /organizations does not
function requiresResourceId(segments, resourceName, varPatterns) {
    const resourceIndex = segments.findIndex(seg => seg === resourceName);
    if (resourceIndex === -1) return false;
    // Check if next segment is a variable pattern
    const nextSegment = segments[resourceIndex + 1];
    if (!nextSegment) return false;
    return varPatterns.some(pattern => nextSegment.includes(pattern));
}

// Base required variables - start empty and add based on URL patterns
const requiredVars = [];

// Require organizationId when URL has it as a path parameter (not for list/create endpoints)
const orgIdPatterns = ['{{organizationId}}', '{organization_id}', ':organization_id'];
if (hasTemplateVar(pathSegments, orgIdPatterns) || requiresResourceId(pathSegments, 'organizations', orgIdPatterns)) {
    requiredVars.push('organizationId');
}

// Require ledgerId when URL has it as a path parameter (not for list/create ledger endpoints)
const ledgerIdPatterns = ['{{ledgerId}}', '{ledger_id}', ':ledger_id'];
if (hasTemplateVar(pathSegments, ledgerIdPatterns) || requiresResourceId(pathSegments, 'ledgers', ledgerIdPatterns)) {
    requiredVars.push('ledgerId');
}

// Add specific variables based on the operation and URL path
const transactionIdPatterns = ['{{transactionId}}', '{transaction_id}', ':transaction_id'];
if (hasTemplateVar(pathSegments, transactionIdPatterns) || requiresResourceId(pathSegments, 'transactions', transactionIdPatterns)) {
    requiredVars.push('transactionId');
}

const operationIdPatterns = ['{{operationId}}', '{operation_id}', ':operation_id'];
if (hasTemplateVar(pathSegments, operationIdPatterns) || requiresResourceId(pathSegments, 'operations', operationIdPatterns)) {
    requiredVars.push('operationId');
}

const balanceIdPatterns = ['{{balanceId}}', '{balance_id}', ':balance_id'];
if (hasTemplateVar(pathSegments, balanceIdPatterns) || requiresResourceId(pathSegments, 'balances', balanceIdPatterns)) {
    requiredVars.push('balanceId');
}

const accountIdPatterns = ['{{accountId}}', '{account_id}', ':account_id'];
if (hasTemplateVar(pathSegments, accountIdPatterns) || requiresResourceId(pathSegments, 'accounts', accountIdPatterns)) {
    requiredVars.push('accountId');
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