#!/usr/bin/env node

/**
 * @file Workflow Configuration for Postman Collection Generation
 *
 * @description
 * This file centralizes all configuration for the workflow generation process,
 * including API routing patterns, path corrections, variable mappings, and
 * transaction templates. It provides a structured approach to handling API
 * evolution and special cases, replacing hardcoded values from the original
 * implementation.
 */

module.exports = {
  // API Evolution Management: Defines rules for routing and correcting API paths.
  apiPatterns: {
    // serviceRouting: Maps URL path prefixes to their corresponding services.
    serviceRouting: {
      onboarding: [
        "/organizations",
        "/ledgers", 
        "/accounts",
        "/assets",
        "/portfolios",
        "/segments",
      ],
      transaction: [
        "/transactions",
        "/operations",
        "/balances",
        "/asset-rates",
      ],
    },

    // pathCorrections: Provides rules for rewriting incorrect or outdated API paths.
    pathCorrections: [
      {
        name: "Missing ledger segment in accounts path",
        detect: /^\/v1\/organizations\/[^/]+\/accounts/,
        correct: (path) =>
          path.replace(
            /\/organizations\/([^/]+)\/accounts/,
            "/organizations/$1/ledgers/{{ledgerId}}/accounts"
          ),
      },
      {
        name: "Missing ledger segment in balances path", 
        detect: /^\/v1\/organizations\/[^/]+\/balances/,
        correct: (path) =>
          path.replace(
            /\/organizations\/([^/]+)\/balances/,
            "/organizations/$1/ledgers/{{ledgerId}}/balances"
          ),
      },
      {
        name: "Missing ledger segment in operations path",
        detect: /^\/v1\/organizations\/[^/]+\/operations/,
        correct: (path) =>
          path.replace(
            /\/organizations\/([^/]+)\/operations/,
            "/organizations/$1/ledgers/{{ledgerId}}/operations"
          ),
      },
      {
        name: "Missing account segment in operations path",
        detect: /^\/v1\/organizations\/[^/]+\/ledgers\/[^/]+\/operations\/[^/]+$/,
        correct: (path) =>
          path.replace(
            /\/organizations\/([^/]+)\/ledgers\/([^/]+)\/operations/,
            "/organizations/$1/ledgers/$2/accounts/{{accountId}}/operations"
          ),
      },
      {
        name: "Asset rates path normalization",
        detect: /^\/v1\/organizations\/[^/]+\/asset-rates/,
        correct: (path) =>
          path.replace(
            /\/organizations\/([^/]+)\/asset-rates/,
            "/organizations/$1/ledgers/{{ledgerId}}/asset-rates"
          ),
      },
    ],
  },

  // Variable Management: Configures how variables are extracted and mapped in requests.
  variables: {
    // Variable extraction configuration for special steps.
    extraction: {
      "Check Account Balance Before Zeroing": {
        from: "response.items[0].available",
        transform: "absolute", // Math.abs()
        fallback: 0,
        store: "currentBalanceAmount",
        validation: {
          type: "number",
          min: 0,
        },
      },
    },

    // Variable mapping for parameter substitution.
    mapping: {
      // Context-dependent {id} parameter mapping based on URL path.
      contextual: {
        "{id}": [
          {
            pattern: /\/organizations\/\{id\}/,
            replacement: "{{organizationId}}",
          },
          {
            pattern: /\/ledgers\/\{id\}/,
            replacement: "{{ledgerId}}",
          },
          {
            pattern: /\/accounts\/\{id\}/,
            replacement: "{{accountId}}",
          },
          {
            pattern: /\/assets\/\{id\}/,
            replacement: "{{assetId}}",
          },
          {
            pattern: /\/portfolios\/\{id\}/,
            replacement: "{{portfolioId}}",
          },
          {
            pattern: /\/segments\/\{id\}/,
            replacement: "{{segmentId}}",
          },
          {
            pattern: /\/transactions\/\{id\}/,
            replacement: "{{transactionId}}",
          },
          {
            pattern: /\/operations\/\{id\}/,
            replacement: "{{operationId}}",
          },
          {
            pattern: /\/balances\/\{id\}/,
            replacement: "{{balanceId}}",
          },
          {
            pattern: /\/asset-rates\/\{id\}/,
            replacement: "{{assetRateId}}",
          },
        ],
      },

      // Direct parameter mapping for common variable names.
      direct: {
        "{organizationId}": "{{organizationId}}",
        "{organization_id}": "{{organizationId}}",
        "{ledgerId}": "{{ledgerId}}",
        "{ledger_id}": "{{ledgerId}}",
        "{accountId}": "{{accountId}}",
        "{account_id}": "{{accountId}}",
        "{assetId}": "{{assetId}}",
        "{asset_id}": "{{assetId}}",
        "{portfolioId}": "{{portfolioId}}",
        "{portfolio_id}": "{{portfolioId}}",
        "{segmentId}": "{{segmentId}}",
        "{segment_id}": "{{segmentId}}",
        "{transactionId}": "{{transactionId}}",
        "{transaction_id}": "{{transactionId}}",
        "{operationId}": "{{operationId}}",
        "{operation_id}": "{{operationId}}",
        "{balanceId}": "{{balanceId}}",
        "{balance_id}": "{{balanceId}}",
        "{assetRateId}": "{{assetRateId}}",
        "{asset_rate_id}": "{{assetRateId}}",
        "{alias}": "{{accountAlias}}",
        "{code}": "{{externalCode}}",
        "{assetCode}": "{{assetCode}}",
        "{asset_code}": "{{assetCode}}",
        "{externalId}": "{{assetRateId}}",
        "{external_id}": "{{assetRateId}}",
      },
    },
  },

  // Transaction Templates: Provides predefined request bodies for different transaction types.
  transactions: {
    templates: {
      // JSON Transaction Template (explicit source and destination).
      json: {
        chartOfAccountsGroupName: "Example chartOfAccountsGroupName",
        code: "Example code",
        description: "Example description",
        metadata: { key: "value" },
        send: {
          asset: "USD",
          distribute: {
            to: [
              {
                accountAlias: "{{accountAlias}}",
                amount: { asset: "USD", value: "100.00" },
                chartOfAccounts: "Example chartOfAccounts",
                description: "Example description",
                metadata: { key: "value" },
              },
            ],
          },
          source: {
            from: [
              {
                accountAlias: "@external/USD",
                amount: { asset: "USD", value: "100.00" },
                chartOfAccounts: "Example chartOfAccounts",
                description: "Example description",
                metadata: { key: "value" },
              },
            ],
          },
          value: "100.00",
        },
      },

      // Inflow Transaction Template (money coming IN, no explicit source).
      inflow: {
        chartOfAccountsGroupName: "Example chartOfAccountsGroupName",
        code: "Example code",
        description: "Example description",
        metadata: { key: "value" },
        send: {
          asset: "USD",
          distribute: {
            to: [
              {
                accountAlias: "{{accountAlias}}",
                amount: { asset: "USD", value: "100.00" },
                chartOfAccounts: "Example chartOfAccounts",
                description: "Example description",
                metadata: { key: "value" },
              },
            ],
          },
          value: "100.00",
        },
      },

      // Outflow Transaction Template (money going OUT, no explicit destination).
      outflow: {
        chartOfAccountsGroupName: "Example chartOfAccountsGroupName",
        code: "Example code",
        description: "Example description",
        metadata: { key: "value" },
        send: {
          asset: "USD",
          source: {
            from: [
              {
                accountAlias: "{{accountAlias}}",
                amount: { asset: "USD", value: "100.00" },
                chartOfAccounts: "Example chartOfAccounts",
                description: "Example description",
                metadata: { key: "value" },
              },
            ],
          },
          value: "100.00",
        },
      },

      // Zero Out Transaction Template (CRITICAL - do not modify).
      zeroOut: {
        chartOfAccountsGroupName: "Example chartOfAccountsGroupName",
        code: "Zero Out Balance Transaction",
        description:
          "Reverse transaction to zero out the account balance using actual current balance",
        metadata: {
          purpose: "balance_zeroing",
          reference_step: "48",
        },
        send: {
          asset: "USD",
          distribute: {
            to: [
              {
                accountAlias: "@external/USD",
                amount: { asset: "USD", value: "{{currentBalanceAmount}}" },
                chartOfAccounts: "Example chartOfAccounts",
                description: "External account for balance zeroing",
                metadata: { operation_type: "credit" },
              },
            ],
          },
          source: {
            from: [
              {
                accountAlias: "{{accountAlias}}",
                amount: { asset: "USD", value: "{{currentBalanceAmount}}" },
                chartOfAccounts: "Example chartOfAccounts",
                description: "Source account for balance zeroing",
                metadata: { operation_type: "debit" },
              },
            ],
          },
          value: "{{currentBalanceAmount}}",
        },
      },
    },
  },

  // Step Classifications and Test Strategies: Defines how each workflow step is categorized and tested.
  stepTypes: {
    CREATE: {
      pattern: /^Create /,
      testStrategy: "extractId",
      validateStatus: [200, 201],
      extractPattern: /^(\w+)Id$/,
    },
    READ: {
      pattern: /^Get |^List /,
      testStrategy: "validate",
      validateStatus: [200],
    },
    UPDATE: {
      pattern: /^Update /,
      testStrategy: "validate",
      validateStatus: [200, 204],
    },
    DELETE: {
      pattern: /^Delete /,
      testStrategy: "validate", 
      validateStatus: [200, 204],
    },
    COUNT: {
      pattern: /^Count /,
      testStrategy: "header",
      validateStatus: [204],
      extractHeader: "X-Total-Count",
    },
    TRANSACTION: {
      pattern: /Transaction/,
      testStrategy: "extractMultiple",
      validateStatus: [200, 201],
      extractFields: ["transactionId", "balanceId", "operationId"],
    },
    SPECIAL: {
      titles: ["Check Account Balance Before Zeroing", "Zero Out Balance"],
      testStrategy: "custom",
    },
  },

  // Base URL Configuration: Defines the base URLs for each service.
  baseUrls: {
    onboarding: "{{onboardingUrl}}",
    transaction: "{{transactionUrl}}",
  },

  // Performance and Validation Settings: Configures performance tracking and validation rules.
  performance: {
    maxResponseTime: 5000, // milliseconds
    enablePerformanceTracking: true,
    enableRegressionDetection: true,
    performanceIncreaseThreshold: 50, // percentage
  },

  // Validation Rules: Defines regex patterns and required fields for response validation.
  validation: {
    uuidRegex: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
    isoTimestampRegex: /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/,
    
    requiredFields: {
      organization: ["id", "legalName", "status", "createdAt", "updatedAt"],
      ledger: ["id", "name", "status", "createdAt", "updatedAt"],
      account: ["id", "name", "status", "createdAt", "updatedAt"],
      asset: ["id", "name", "code", "status", "createdAt", "updatedAt"],
      portfolio: ["id", "name", "status", "createdAt", "updatedAt"],
      segment: ["id", "name", "status", "createdAt", "updatedAt"],
      transaction: ["id", "status", "createdAt", "updatedAt"],
    },
  },

  // Error Handling Configuration: Configures how errors and missing requests are handled.
  errors: {
    handleMissingRequests: true,
    createPlaceholders: true,
    logLevel: "warn", // error, warn, info, debug
    maxMissingRequestsWarning: 5,
  },

  // Feature Flags: Enables or disables specific features in the generation process.
  features: {
    enhancedTestScripts: true,
    dependencyValidation: true,
    performanceTracking: true,
    businessLogicValidation: true,
    balanceExtractionValidation: true,
  },
};