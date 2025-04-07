# Onboarding Domain Model

**Navigation:** [Home](../../) > [Components](../) > [Onboarding](./README.md) > Domain Model

This document describes the domain model for the Onboarding Service, explaining the key entities, their relationships, and the design patterns used.

## Table of Contents

- [Overview](#overview)
- [Entity Hierarchy](#entity-hierarchy)
- [Core Entities](#core-entities)
  - [Organization](#organization)
  - [Ledger](#ledger)
  - [Asset](#asset)
  - [Segment](#segment)
  - [Portfolio](#portfolio)
  - [Account](#account)
- [Common Patterns](#common-patterns)
  - [Status Management](#status-management)
  - [Soft Deletion](#soft-deletion)
  - [Metadata Extension](#metadata-extension)
  - [Audit Tracking](#audit-tracking)
- [Entity Relationships](#entity-relationships)
- [Domain Rules](#domain-rules)
- [Repository Pattern](#repository-pattern)
- [Data Access Patterns](#data-access-patterns)

## Overview

The Onboarding Service domain model represents the financial entity hierarchy that forms the foundation of the Midaz system. It follows a structured approach with Organizations at the top level, followed by Ledgers, and then Asset, Segment, Portfolio, and Account entities that exist within the context of a Ledger.

The model is designed to provide:

1. **Hierarchical Organization**: Allows for complex organizational structures with parent-child relationships
2. **Flexible Financial Structures**: Supports different types of financial entities and their relationships
3. **Extensibility**: Uses metadata for custom attributes without schema changes
4. **Data Integrity**: Enforces constraints and validation rules for financial entities
5. **Soft Deletion**: Preserves historical data through soft deletion patterns

## Entity Hierarchy

The domain model follows a structured hierarchy:

```
Organization
└── Ledger
    ├── Asset
    ├── Segment
    ├── Portfolio
    │   └── Account
    └── Account
```

This hierarchy establishes clear ownership and containment relationships between entities, with specific rules governing these relationships.

## Core Entities

### Organization

Organizations are the top-level entities in the system, representing legal entities like companies or individuals.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `ParentOrganizationID`: Reference to parent organization (optional)
- `LegalName`: Official registered name
- `DoingBusinessAs`: Trading or brand name (optional)
- `LegalDocument`: Tax ID, registration number, or other legal identifier
- `Address`: Structured physical address
- `Status`: Current operating status (ACTIVE, INACTIVE, etc.)
- `Metadata`: Custom key-value attributes

**Special Features:**
- Self-referential relationship allowing hierarchical organization structures
- Structured address with standardized fields (follows ISO country codes)
- Soft deletion support

**Business Rules:**
- Legal name and legal document are required
- Valid parent organization ID must exist if specified
- Address follows standard postal format

### Ledger

Ledgers represent financial record-keeping systems within organizations.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `Name`: Display name
- `OrganizationID`: Reference to the owning organization
- `Status`: Current operating status
- `Metadata`: Custom key-value attributes

**Special Features:**
- Container for all financial entities within an organization
- Establishes boundaries for financial operations
- Segregates financial data across organizational units

**Business Rules:**
- Every ledger must belong to exactly one organization
- Name is required and must be unique within an organization
- Ledgers can be active or inactive

### Asset

Assets represent financial instruments or currencies used within a ledger.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `Name`: Display name
- `Type`: Asset type (currency, cryptocurrency, etc.)
- `Code`: Unique symbol or code for the asset
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger
- `Status`: Current operating status
- `Metadata`: Custom key-value attributes

**Special Features:**
- Defines the units of value used in accounts
- Enables multi-currency support
- Used in financial transactions

**Business Rules:**
- Every asset must belong to exactly one organization and one ledger
- Asset code must be unique within a ledger
- Name and code are required

### Segment

Segments represent logical divisions within a ledger, such as business areas or product lines.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `Name`: Display name
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger
- `Status`: Current operating status
- `Metadata`: Custom key-value attributes

**Special Features:**
- Enables categorization and reporting across business functions
- Provides a dimension for financial analysis
- Can be used to group accounts

**Business Rules:**
- Every segment must belong to exactly one organization and one ledger
- Name is required
- Segments can be active or inactive

### Portfolio

Portfolios represent collections of accounts grouped for specific purposes.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `Name`: Display name
- `EntityID`: Optional external entity identifier
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger
- `Status`: Current operating status
- `Metadata`: Custom key-value attributes

**Special Features:**
- Groups related accounts together
- Can represent business units, departments, or client portfolios
- Provides organizational context for accounts

**Business Rules:**
- Every portfolio must belong to exactly one organization and one ledger
- Name is required
- Can contain external entity reference for integration with other systems

### Account

Accounts are the basic units for tracking financial resources within a ledger.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `Name`: Display name
- `ParentAccountID`: Reference to parent account (optional)
- `EntityID`: Optional external entity identifier
- `AssetCode`: Reference to the asset used by this account
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger
- `PortfolioID`: Optional reference to containing portfolio
- `SegmentID`: Optional reference to segment categorization
- `Status`: Current operating status
- `Alias`: Unique human-readable identifier (e.g., "@customer1")
- `Type`: Account type (checking, savings, creditCard, expense, etc.)
- `Metadata`: Custom key-value attributes

**Special Features:**
- Self-referential relationship allowing account hierarchies
- Flexible categorization through portfolio and segment assignments
- Type-based classification for different account behaviors
- Alias system for human-readable references

**Business Rules:**
- Every account must belong to exactly one organization and one ledger
- AssetCode must reference a valid asset in the same ledger
- Account type is required
- Alias must be unique within a ledger
- Optional portfolio and segment must belong to the same ledger
- Valid parent account ID must exist if specified

## Common Patterns

### Status Management

All entities use a standardized `Status` structure:

```go
type Status struct {
    Code        string  `json:"code"`
    Description *string `json:"description"`
}
```

**Status Codes:**
- `ACTIVE`: Entity is operational and usable
- `INACTIVE`: Entity exists but is not currently operational
- `PENDING`: Entity is awaiting activation or approval
- `SUSPENDED`: Entity is temporarily disabled
- `DELETED`: Entity is marked for deletion (soft delete)

This pattern provides consistent status handling across all domain entities.

### Soft Deletion

All entities support soft deletion through the `DeletedAt` field:

- `DeletedAt` is a nullable timestamp
- When set, indicates the entity has been "deleted"
- Queries filter by `DeletedAt IS NULL` to exclude deleted records
- Preserves historical data and relationships

This pattern allows for data recovery and maintains the integrity of historical records.

### Metadata Extension

All entities include a flexible `Metadata` field:

```go
Metadata map[string]any `json:"metadata,omitempty"`
```

This provides:
- Schema-less extension for custom attributes
- Support for business-specific fields without model changes
- Validation rules: keys ≤ 100 chars, values ≤ 2000 chars
- No nested objects (for query performance)

The metadata pattern allows for extensibility while maintaining core model stability.

### Audit Tracking

All entities include standard audit fields:

- `CreatedAt`: Timestamp when the entity was created
- `UpdatedAt`: Timestamp when the entity was last updated
- `DeletedAt`: Timestamp when the entity was soft-deleted (nullable)

These fields provide an audit trail of entity lifecycle events.

## Entity Relationships

The domain model enforces several types of relationships:

1. **Containment Relationships**:
   - Organization contains Ledgers
   - Ledger contains Assets, Segments, Portfolios, and Accounts
   - Portfolio contains Accounts (optional)

2. **Hierarchical Relationships**:
   - Organizations can have parent-child relationships
   - Accounts can have parent-child relationships

3. **Categorization Relationships**:
   - Segments categorize Accounts
   - Portfolios group Accounts

4. **Asset Association**:
   - Assets define the currency or financial instrument for Accounts
   - Accounts reference Assets via the AssetCode field

All relationships enforce appropriate referential integrity constraints.

## Domain Rules

The domain model enforces several important business rules:

1. **Hierarchical Constraints**:
   - Parent-child relationships must be within the same organization
   - Account hierarchy must be within the same ledger

2. **Reference Integrity**:
   - All references to other entities must point to valid, non-deleted entities
   - References across organizations are prohibited

3. **Unique Identifiers**:
   - All entities have UUID-based primary keys
   - Accounts can have unique aliases for human-readable reference
   - Assets have unique codes within a ledger

4. **Required Fields**:
   - Names are required for all entities
   - Legal details are required for organizations
   - Asset information is required for accounts

5. **Validation Rules**:
   - String length constraints (e.g., max 256 characters for names)
   - Format validation (e.g., UUID validation)
   - Address format validation (e.g., country codes)

## Repository Pattern

The domain model is accessed through repository interfaces:

```go
type OrganizationRepository interface {
    Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)
    GetByID(ctx context.Context, id string) (*mmodel.Organization, error)
    List(ctx context.Context, filter map[string]interface{}) ([]*mmodel.Organization, error)
    Update(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)
    Delete(ctx context.Context, id string) error
}
```

Similar interfaces exist for all domain entities, providing:
- Standard CRUD operations
- Consistent patterns across all entities
- Dependency injection for testing
- Abstraction from specific database technologies

## Data Access Patterns

The domain model supports several data access patterns:

1. **Direct Access**: Retrieve entities by ID or unique identifier
2. **Filtered Queries**: List entities based on filter criteria
3. **Pagination**: Return results in pages to handle large datasets
4. **Time-based Filtering**: Filter by creation or update time
5. **Metadata Queries**: Filter by custom metadata fields
6. **Hierarchy Navigation**: Traverse parent-child relationships

These patterns provide flexible access to domain entities while enforcing business rules and constraints.