# Creating Financial Structures

**Navigation:** [Home](../) > [Tutorials](./README.md) > Creating Financial Structures

This tutorial provides a step-by-step guide for creating a complete financial structure in Midaz, including organizations, ledgers, assets, portfolios, segments, and accounts.

## Table of Contents

- [Introduction](#introduction)
- [Financial Entity Hierarchy](#financial-entity-hierarchy)
- [Step-by-Step Guide](#step-by-step-guide)
- [Advanced Configurations](#advanced-configurations)
- [Complete Example](#complete-example)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)
- [References](#references)

## Introduction

Setting up a proper financial structure is the first step in using the Midaz platform for financial operations. This tutorial will guide you through creating a complete structure using the MDZ CLI, explaining each entity's purpose and how they relate to each other.

## Financial Entity Hierarchy

Midaz uses a hierarchical structure for organizing financial entities:

```
Organization
    └── Ledger
         ├── Asset
         ├── Segment
         └── Portfolio
              └── Account
```

Each entity has specific relationships and dependencies:

| Entity | Parent | Description |
|--------|--------|-------------|
| Organization | None | Top-level entity representing a company |
| Ledger | Organization | Financial record-keeping system |
| Asset | Ledger | Currencies or financial instruments (e.g., USD, BTC) |
| Segment | Ledger | Logical divisions (e.g., business areas, customer categories) |
| Portfolio | Ledger | Account grouping for specific purposes |
| Account | Portfolio | Individual financial account tied to a specific asset |

## Step-by-Step Guide

### 1. Create an Organization

The Organization is the top-level entity representing a company or financial institution.

```bash
# Create an organization
mdz organization create --legal-name "Demo Company Inc." \
                        --doing-business-as "Demo Company" \
                        --legal-document "123456789" \
                        --code "ACTIVE" \
                        --country "US"
```

Key fields:
- `legal-name`: The official registered name
- `doing-business-as`: The operational name (optional)
- `legal-document`: Tax ID or registration number
- `code`: Status code (usually "ACTIVE")

Store the organization ID for subsequent commands:
```bash
# Store organization ID in a variable
ORGANIZATION_ID=$(mdz organization list --output json | jq -r '.items[0].id')
echo "Organization ID: $ORGANIZATION_ID"
```

### 2. Create a Ledger

Ledgers belong to an organization and contain all financial records.

```bash
# Create a ledger within the organization
mdz ledger create --organization-id "$ORGANIZATION_ID" \
                  --name "Main Ledger" \
                  --status-code "ACTIVE"
```

Key fields:
- `organization-id`: The parent organization's ID
- `name`: Descriptive name for the ledger
- `status-code`: Status of the ledger (usually "ACTIVE")

Store the ledger ID:
```bash
# Store ledger ID in a variable
LEDGER_ID=$(mdz ledger list --organization-id "$ORGANIZATION_ID" --output json | jq -r '.items[0].id')
echo "Ledger ID: $LEDGER_ID"
```

### 3. Create Assets

Assets represent currencies, cryptocurrencies, or other financial instruments within a ledger.

```bash
# Create a USD asset
mdz asset create --organization-id "$ORGANIZATION_ID" \
                 --ledger-id "$LEDGER_ID" \
                 --name "US Dollar" \
                 --code "USD" \
                 --type "currency" \
                 --status-code "ACTIVE"

# Create a EUR asset
mdz asset create --organization-id "$ORGANIZATION_ID" \
                 --ledger-id "$LEDGER_ID" \
                 --name "Euro" \
                 --code "EUR" \
                 --type "currency" \
                 --status-code "ACTIVE"
```

Key fields:
- `organization-id` and `ledger-id`: Parent entity IDs
- `name`: Descriptive name of the asset
- `code`: Unique identifier code (e.g., USD, EUR, BTC)
- `type`: Asset category (e.g., currency, crypto, commodity)

Store asset IDs:
```bash
# Store asset IDs in variables
USD_ASSET_ID=$(mdz asset list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --output json | jq -r '.items[] | select(.code=="USD") | .id')
EUR_ASSET_ID=$(mdz asset list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --output json | jq -r '.items[] | select(.code=="EUR") | .id')
echo "USD Asset ID: $USD_ASSET_ID"
echo "EUR Asset ID: $EUR_ASSET_ID"
```

### 4. Create Segments

Segments are logical divisions within a ledger, such as business areas, product lines, or customer categories.

```bash
# Create a Retail segment
mdz segment create --organization-id "$ORGANIZATION_ID" \
                   --ledger-id "$LEDGER_ID" \
                   --name "Retail Customers" \
                   --status-code "ACTIVE"

# Create a Corporate segment
mdz segment create --organization-id "$ORGANIZATION_ID" \
                   --ledger-id "$LEDGER_ID" \
                   --name "Corporate Customers" \
                   --status-code "ACTIVE"
```

Key fields:
- `organization-id` and `ledger-id`: Parent entity IDs
- `name`: Descriptive name for the segment
- `status-code`: Status of the segment (usually "ACTIVE")

Store segment IDs:
```bash
# Store segment IDs in variables
RETAIL_SEGMENT_ID=$(mdz segment list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --output json | jq -r '.items[] | select(.name=="Retail Customers") | .id')
CORPORATE_SEGMENT_ID=$(mdz segment list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --output json | jq -r '.items[] | select(.name=="Corporate Customers") | .id')
echo "Retail Segment ID: $RETAIL_SEGMENT_ID"
echo "Corporate Segment ID: $CORPORATE_SEGMENT_ID"
```

### 5. Create Portfolios

Portfolios group related accounts and are typically associated with customers, business units, or departments.

```bash
# Create a Customer portfolio
mdz portfolio create --organization-id "$ORGANIZATION_ID" \
                     --ledger-id "$LEDGER_ID" \
                     --name "Customer Portfolio" \
                     --entity-id "CUST-001" \
                     --status-code "ACTIVE"

# Create an Investment portfolio
mdz portfolio create --organization-id "$ORGANIZATION_ID" \
                     --ledger-id "$LEDGER_ID" \
                     --name "Investment Portfolio" \
                     --entity-id "INV-001" \
                     --status-code "ACTIVE"
```

Key fields:
- `organization-id` and `ledger-id`: Parent entity IDs
- `name`: Descriptive name for the portfolio
- `entity-id`: External identifier for the portfolio (optional)
- `status-code`: Status of the portfolio (usually "ACTIVE")

Store portfolio IDs:
```bash
# Store portfolio IDs in variables
CUSTOMER_PORTFOLIO_ID=$(mdz portfolio list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --output json | jq -r '.items[] | select(.name=="Customer Portfolio") | .id')
INVESTMENT_PORTFOLIO_ID=$(mdz portfolio list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --output json | jq -r '.items[] | select(.name=="Investment Portfolio") | .id')
echo "Customer Portfolio ID: $CUSTOMER_PORTFOLIO_ID"
echo "Investment Portfolio ID: $INVESTMENT_PORTFOLIO_ID"
```

### 6. Create Accounts

Accounts represent individual financial entities and belong to portfolios.

```bash
# Create a USD Checking account for the Customer portfolio
mdz account create --organization-id "$ORGANIZATION_ID" \
                  --ledger-id "$LEDGER_ID" \
                  --portfolio-id "$CUSTOMER_PORTFOLIO_ID" \
                  --segment-id "$RETAIL_SEGMENT_ID" \
                  --name "Checking Account" \
                  --type "checking" \
                  --asset-code "USD" \
                  --alias "@checking" \
                  --status-code "ACTIVE"

# Create a EUR Savings account for the Investment portfolio
mdz account create --organization-id "$ORGANIZATION_ID" \
                  --ledger-id "$LEDGER_ID" \
                  --portfolio-id "$INVESTMENT_PORTFOLIO_ID" \
                  --segment-id "$CORPORATE_SEGMENT_ID" \
                  --name "Investment Account" \
                  --type "investment" \
                  --asset-code "EUR" \
                  --alias "@investment" \
                  --status-code "ACTIVE"
```

Key fields:
- `organization-id`, `ledger-id`, `portfolio-id`, and `segment-id`: Parent entity IDs
- `name`: Descriptive name for the account
- `type`: Account type (e.g., checking, savings, investment)
- `asset-code`: The code of the asset this account handles
- `alias`: A human-readable unique identifier for the account (starts with @)
- `status-code`: Status of the account (usually "ACTIVE")

## Advanced Configurations

### Using Metadata

All entities in Midaz support custom metadata to extend their functionality. Metadata is stored as key-value pairs in JSON format.

```bash
# Add metadata to an organization
mdz organization update --organization-id "$ORGANIZATION_ID" \
                       --metadata '{"industry": "Finance", "region": "North America"}'

# Add metadata to an account
mdz account update --organization-id "$ORGANIZATION_ID" \
                  --ledger-id "$LEDGER_ID" \
                  --portfolio-id "$CUSTOMER_PORTFOLIO_ID" \
                  --account-id "$ACCOUNT_ID" \
                  --metadata '{"purpose": "Operations", "category": "Retail"}'
```

### Hierarchical Accounts

Accounts can have parent-child relationships for representing hierarchical structures.

```bash
# Create a parent account
PARENT_ACCOUNT_ID=$(mdz account create --organization-id "$ORGANIZATION_ID" \
                                     --ledger-id "$LEDGER_ID" \
                                     --portfolio-id "$CUSTOMER_PORTFOLIO_ID" \
                                     --name "Parent Account" \
                                     --type "parent" \
                                     --asset-code "USD" \
                                     --status-code "ACTIVE" \
                                     --output json | jq -r '.id')

# Create a child account
mdz account create --organization-id "$ORGANIZATION_ID" \
                  --ledger-id "$LEDGER_ID" \
                  --portfolio-id "$CUSTOMER_PORTFOLIO_ID" \
                  --parent-account-id "$PARENT_ACCOUNT_ID" \
                  --name "Child Account" \
                  --type "child" \
                  --asset-code "USD" \
                  --status-code "ACTIVE"
```

## Complete Example

Below is a complete script that builds a financial structure from scratch:

```bash
#!/bin/bash

# Create an organization
echo "Creating organization..."
ORG_ID=$(mdz organization create --legal-name "Global Trading Inc." \
                                --doing-business-as "GloTrade" \
                                --legal-document "123456789" \
                                --code "ACTIVE" \
                                --country "US" \
                                --output json | jq -r '.id')
echo "Organization created with ID: $ORG_ID"

# Create a ledger
echo "Creating ledger..."
LEDGER_ID=$(mdz ledger create --organization-id "$ORG_ID" \
                             --name "Trading Ledger" \
                             --status-code "ACTIVE" \
                             --output json | jq -r '.id')
echo "Ledger created with ID: $LEDGER_ID"

# Create assets
echo "Creating assets..."
USD_ASSET_ID=$(mdz asset create --organization-id "$ORG_ID" \
                               --ledger-id "$LEDGER_ID" \
                               --name "US Dollar" \
                               --type "currency" \
                               --code "USD" \
                               --status-code "ACTIVE" \
                               --output json | jq -r '.id')
echo "USD Asset created with ID: $USD_ASSET_ID"

EUR_ASSET_ID=$(mdz asset create --organization-id "$ORG_ID" \
                               --ledger-id "$LEDGER_ID" \
                               --name "Euro" \
                               --type "currency" \
                               --code "EUR" \
                               --status-code "ACTIVE" \
                               --output json | jq -r '.id')
echo "EUR Asset created with ID: $EUR_ASSET_ID"

# Create segments
echo "Creating segments..."
RETAIL_SEGMENT_ID=$(mdz segment create --organization-id "$ORG_ID" \
                                      --ledger-id "$LEDGER_ID" \
                                      --name "Retail Customers" \
                                      --status-code "ACTIVE" \
                                      --output json | jq -r '.id')
echo "Retail Segment created with ID: $RETAIL_SEGMENT_ID"

CORPORATE_SEGMENT_ID=$(mdz segment create --organization-id "$ORG_ID" \
                                         --ledger-id "$LEDGER_ID" \
                                         --name "Corporate Customers" \
                                         --status-code "ACTIVE" \
                                         --output json | jq -r '.id')
echo "Corporate Segment created with ID: $CORPORATE_SEGMENT_ID"

# Create portfolios
echo "Creating portfolios..."
INVESTMENT_PORTFOLIO_ID=$(mdz portfolio create --organization-id "$ORG_ID" \
                                              --ledger-id "$LEDGER_ID" \
                                              --entity-id "inv-001" \
                                              --name "Investment Portfolio" \
                                              --status-code "ACTIVE" \
                                              --output json | jq -r '.id')
echo "Investment Portfolio created with ID: $INVESTMENT_PORTFOLIO_ID"

TRADING_PORTFOLIO_ID=$(mdz portfolio create --organization-id "$ORG_ID" \
                                           --ledger-id "$LEDGER_ID" \
                                           --entity-id "trade-001" \
                                           --name "Trading Portfolio" \
                                           --status-code "ACTIVE" \
                                           --output json | jq -r '.id')
echo "Trading Portfolio created with ID: $TRADING_PORTFOLIO_ID"

# Create accounts
echo "Creating accounts..."
USD_ACCOUNT_ID=$(mdz account create --organization-id "$ORG_ID" \
                                   --ledger-id "$LEDGER_ID" \
                                   --portfolio-id "$INVESTMENT_PORTFOLIO_ID" \
                                   --segment-id "$RETAIL_SEGMENT_ID" \
                                   --asset-code "USD" \
                                   --name "USD Investment Account" \
                                   --type "investment" \
                                   --alias "@usd_investment" \
                                   --status-code "ACTIVE" \
                                   --output json | jq -r '.id')
echo "USD Account created with ID: $USD_ACCOUNT_ID"

EUR_ACCOUNT_ID=$(mdz account create --organization-id "$ORG_ID" \
                                   --ledger-id "$LEDGER_ID" \
                                   --portfolio-id "$TRADING_PORTFOLIO_ID" \
                                   --segment-id "$CORPORATE_SEGMENT_ID" \
                                   --asset-code "EUR" \
                                   --name "EUR Trading Account" \
                                   --type "trading" \
                                   --alias "@eur_trading" \
                                   --status-code "ACTIVE" \
                                   --output json | jq -r '.id')
echo "EUR Account created with ID: $EUR_ACCOUNT_ID"

# Validate structure
echo "Validating structure..."
echo "Organization:"
mdz organization describe --organization-id "$ORG_ID"

echo "Ledgers:"
mdz ledger list --organization-id "$ORG_ID"

echo "Assets:"
mdz asset list --organization-id "$ORG_ID" --ledger-id "$LEDGER_ID"

echo "Segments:"
mdz segment list --organization-id "$ORG_ID" --ledger-id "$LEDGER_ID"

echo "Portfolios:"
mdz portfolio list --organization-id "$ORG_ID" --ledger-id "$LEDGER_ID"

echo "Accounts:"
mdz account list --organization-id "$ORG_ID" --ledger-id "$LEDGER_ID"

echo "Financial structure created successfully!"
```

Save this script as `create-financial-structure.sh`, make it executable with `chmod +x create-financial-structure.sh`, and run it to create a complete financial structure.

## Best Practices

When creating financial structures in Midaz, follow these best practices:

1. **Plan your hierarchy**: Design your financial structure before implementing it
2. **Use descriptive names**: Choose clear, meaningful names for all entities
3. **Store IDs in variables**: When creating child entities, store parent IDs in variables
4. **Use status codes consistently**: Standardize status codes across entities (e.g., "ACTIVE", "INACTIVE")
5. **Leverage metadata**: Use metadata to extend entities with custom attributes
6. **Validate as you go**: Check that each entity is created correctly before moving to the next
7. **Document your structure**: Keep track of entity IDs and relationships for future reference
8. **Use unique aliases**: Create memorable aliases for accounts to make them easier to reference

## Troubleshooting

### Common Issues

| Issue | Possible Cause | Solution |
|-------|---------------|----------|
| Entity creation fails | Missing required fields | Ensure all required fields are provided |
| "Entity not found" errors | Incorrect parent entity ID | Double-check parent entity IDs |
| Asset creation fails | Duplicate asset code | Use unique asset codes within a ledger |
| Account creation fails | Invalid asset code | Verify the asset exists in the ledger |

### Validating Your Structure

Use these commands to validate your financial structure at any time:

```bash
# Check organization details
mdz organization describe --organization-id "$ORGANIZATION_ID"

# List all ledgers in an organization
mdz ledger list --organization-id "$ORGANIZATION_ID"

# List all assets in a ledger
mdz asset list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID"

# List all accounts in a portfolio
mdz account list --organization-id "$ORGANIZATION_ID" --ledger-id "$LEDGER_ID" --portfolio-id "$PORTFOLIO_ID"
```

## References

- [MDZ CLI Commands](../components/mdz-cli/commands.md) - Comprehensive reference for all CLI commands
- [MDZ CLI Usage Guide](../components/mdz-cli/usage-guide.md) - Detailed guide for using the CLI
- [Entity Hierarchy](../domain-models/entity-hierarchy.md) - Detailed explanation of entity relationships
- [Financial Model](../domain-models/financial-model.md) - Overview of the financial model