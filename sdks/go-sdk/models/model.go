// Package models defines the data models used by the Midaz SDK.
//
// This package provides models that either:
// 1. Directly align with backend types from pkg/mmodel where possible
// 2. Implement SDK-specific types only where necessary
//
// The goal is to maintain a simple, direct approach without unnecessary abstraction layers
// while ensuring the SDK interfaces cleanly with the backend API.
//
// Key Model Types:
//
// Account: Represents an account in the Midaz system, which is a fundamental
// entity for tracking assets and balances. Accounts belong to organizations
// and ledgers.
//
// Asset: Represents a type of value that can be tracked and transferred within
// the Midaz system, such as currencies, securities, or other financial instruments.
//
// Balance: Represents the current state of an account's holdings for a specific
// asset, including total, available, and on-hold amounts.
//
// Ledger: Represents a collection of accounts and transactions within an organization,
// providing a complete record of financial activities.
//
// Organization: Represents a business entity that owns ledgers, accounts, and other
// resources within the Midaz system.
//
// Portfolio: Represents a collection of accounts that belong to a specific entity
// within an organization and ledger, used for grouping and management.
//
// Segment: Represents a categorization unit for more granular organization of
// accounts or other entities within a ledger.
//
// Transaction: Represents a financial event that affects one or more accounts
// through a series of operations (debits and credits).
//
// Operation: Represents an individual accounting entry within a transaction,
// typically a debit or credit to a specific account.
//
// Queue: Represents a transaction queue for temporarily storing transaction data
// before processing, allowing for batched or asynchronous handling.
//
// Each model type includes constructors, conversion methods between SDK and backend
// models, and utility methods for setting optional fields. Input structures for
// creating and updating resources are also provided.
package models

// Note: This file serves as documentation for the model package architecture.
// The actual models are defined in their respective files.
