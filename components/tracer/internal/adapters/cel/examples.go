// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package cel provides CEL expression compilation and evaluation.
// This file contains example expressions for testing and documentation.
package cel

// ExampleExpression represents a test expression with expected behavior.
type ExampleExpression struct {
	Name        string // Descriptive name for the expression
	Expression  string // CEL expression string
	Description string // What the expression checks
	Category    string // Category: amount, transaction, account, merchant, scope, metadata, combined
}

// AmountExpressions contains expressions that check transaction amounts.
// Amount is a decimal value (e.g., 1000.00 for $1000.00)
var AmountExpressions = []ExampleExpression{
	{
		Name:        "high_value_transaction",
		Expression:  "amount > 1000",
		Description: "Transactions over $1000",
		Category:    "amount",
	},
	{
		Name:        "low_value_transaction",
		Expression:  "amount <= 100",
		Description: "Transactions up to $100",
		Category:    "amount",
	},
	{
		Name:        "amount_range",
		Expression:  "amount >= 500 && amount <= 2000",
		Description: "Transactions between $500 and $2000",
		Category:    "amount",
	},
	{
		Name:        "pix_high_value",
		Expression:  `transactionType == "PIX" && amount > 5000`,
		Description: "PIX transactions over $5000",
		Category:    "amount",
	},
	{
		Name:        "decimal_amount_threshold",
		Expression:  "amount > 12.34",
		Description: "Transactions over $12.34",
		Category:    "amount",
	},
	{
		Name:        "exact_decimal_match",
		Expression:  "amount == 99.99",
		Description: "Exact amount match; works for direct values but avoid for computed amounts due to float precision",
		Category:    "amount",
	},
	{
		Name:        "decimal_amount_range",
		Expression:  "amount >= 1000.50 && amount <= 5000.75",
		Description: "Decimal amount range",
		Category:    "amount",
	},
}

// TransactionTypeExpressions contains expressions that check transaction types.
var TransactionTypeExpressions = []ExampleExpression{
	{
		Name:        "is_pix",
		Expression:  `transactionType == "PIX"`,
		Description: "PIX transactions only",
		Category:    "transaction",
	},
	{
		Name:        "is_wire",
		Expression:  `transactionType == "WIRE"`,
		Description: "Wire transfer transactions",
		Category:    "transaction",
	},
	{
		Name:        "is_card",
		Expression:  `transactionType == "CARD"`,
		Description: "Card transactions",
		Category:    "transaction",
	},
	{
		Name:        "high_risk_types",
		Expression:  `transactionType in ["WIRE", "CRYPTO"]`,
		Description: "High-risk transaction types",
		Category:    "transaction",
	},
	{
		Name:        "instant_pix",
		Expression:  `transactionType == "PIX" && subType == "instant"`,
		Description: "Instant PIX transactions",
		Category:    "transaction",
	},
}

// AccountExpressions contains expressions that check account context.
// Account fields are accessed as map: account["field"]
var AccountExpressions = []ExampleExpression{
	{
		Name:        "active_account",
		Expression:  `account["status"] == "active"`,
		Description: "Account must be active",
		Category:    "account",
	},
	{
		Name:        "checking_account",
		Expression:  `account["type"] == "checking"`,
		Description: "Checking accounts only",
		Category:    "account",
	},
	{
		Name:        "suspended_account",
		Expression:  `account["status"] == "suspended"`,
		Description: "Detect suspended accounts",
		Category:    "account",
	},
	{
		Name:        "active_checking",
		Expression:  `account["status"] == "active" && account["type"] == "checking"`,
		Description: "Active checking accounts",
		Category:    "account",
	},
}

// MerchantExpressions contains expressions that check merchant context.
// Merchant fields are accessed as map: merchant["field"]
var MerchantExpressions = []ExampleExpression{
	{
		Name:        "gambling_mcc",
		Expression:  `merchant["category"] == "7995"`,
		Description: "Gambling merchant (MCC 7995)",
		Category:    "merchant",
	},
	{
		Name:        "high_risk_mcc",
		Expression:  `merchant["category"] in ["5912", "5993", "7995"]`,
		Description: "High-risk MCCs (drug stores, cigar stores, gambling)",
		Category:    "merchant",
	},
	{
		Name:        "foreign_merchant",
		Expression:  `merchant["country"] != "BR"`,
		Description: "Non-Brazilian merchants",
		Category:    "merchant",
	},
	{
		Name:        "domestic_merchant",
		Expression:  `merchant["country"] == "BR"`,
		Description: "Brazilian merchants only",
		Category:    "merchant",
	},
}

// SegmentPortfolioExpressions contains expressions that check segment and portfolio.
// Segment and Portfolio are optional context objects with id and name fields.
var SegmentPortfolioExpressions = []ExampleExpression{
	{
		Name:        "has_segment",
		Expression:  `size(segment) > 0`,
		Description: "Request has a segment context",
		Category:    "segment",
	},
	{
		Name:        "segment_is_retail",
		Expression:  `segment["name"] == "retail"`,
		Description: "Segment is retail",
		Category:    "segment",
	},
	{
		Name:        "has_portfolio",
		Expression:  `size(portfolio) > 0`,
		Description: "Request has a portfolio context",
		Category:    "portfolio",
	},
	{
		Name:        "portfolio_is_premium",
		Expression:  `portfolio["name"] == "premium"`,
		Description: "Portfolio is premium",
		Category:    "portfolio",
	},
}

// MetadataExpressions contains expressions that check custom metadata.
// Metadata is accessed as map: metadata["key"]
var MetadataExpressions = []ExampleExpression{
	{
		Name:        "has_risk_score",
		Expression:  `"risk_score" in metadata`,
		Description: "Request has risk_score metadata",
		Category:    "metadata",
	},
	{
		Name:        "high_risk_score",
		Expression:  `"risk_score" in metadata && metadata["risk_score"] > 70`,
		Description: "Risk score above 70",
		Category:    "metadata",
	},
	{
		Name:        "mobile_channel",
		Expression:  `metadata["channel"] == "mobile"`,
		Description: "Mobile channel transactions",
		Category:    "metadata",
	},
	{
		Name:        "web_channel",
		Expression:  `metadata["channel"] == "web"`,
		Description: "Web channel transactions",
		Category:    "metadata",
	},
}

// CombinedExpressions contains complex expressions combining multiple checks.
var CombinedExpressions = []ExampleExpression{
	{
		Name:        "high_value_active_account",
		Expression:  `amount > 1000 && account["status"] == "active"`,
		Description: "High-value transaction from active account",
		Category:    "combined",
	},
	{
		Name:        "pix_high_value_active",
		Expression:  `transactionType == "PIX" && amount > 5000 && account["status"] == "active"`,
		Description: "High-value PIX from active account",
		Category:    "combined",
	},
	{
		Name:        "foreign_high_value",
		Expression:  `merchant["country"] != "BR" && amount > 2000`,
		Description: "High-value foreign transaction",
		Category:    "combined",
	},
	{
		Name:        "risky_transaction",
		Expression:  `amount > 1000 && merchant["category"] in ["7995"] && account["status"] == "active"`,
		Description: "High-value gambling from active account",
		Category:    "combined",
	},
	{
		Name:        "full_validation",
		Expression:  `transactionType == "PIX" && amount > 100 && account["status"] == "active" && currency == "BRL"`,
		Description: "Full PIX validation with multiple checks",
		Category:    "combined",
	},
}

// AllExpressions returns all example expressions.
func AllExpressions() []ExampleExpression {
	totalLen := len(AmountExpressions) +
		len(TransactionTypeExpressions) +
		len(AccountExpressions) +
		len(MerchantExpressions) +
		len(SegmentPortfolioExpressions) +
		len(MetadataExpressions) +
		len(CombinedExpressions)

	all := make([]ExampleExpression, 0, totalLen)

	all = append(all, AmountExpressions...)
	all = append(all, TransactionTypeExpressions...)
	all = append(all, AccountExpressions...)
	all = append(all, MerchantExpressions...)
	all = append(all, SegmentPortfolioExpressions...)
	all = append(all, MetadataExpressions...)
	all = append(all, CombinedExpressions...)

	return all
}
