// Code generated from Transaction.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // Transaction

import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by TransactionParser.
type TransactionVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by TransactionParser#transaction.
	VisitTransaction(ctx *TransactionContext) interface{}

	// Visit a parse tree produced by TransactionParser#chartOfAccountsGroupName.
	VisitChartOfAccountsGroupName(ctx *ChartOfAccountsGroupNameContext) interface{}

	// Visit a parse tree produced by TransactionParser#code.
	VisitCode(ctx *CodeContext) interface{}

	// Visit a parse tree produced by TransactionParser#trueOrFalse.
	VisitTrueOrFalse(ctx *TrueOrFalseContext) interface{}

	// Visit a parse tree produced by TransactionParser#pending.
	VisitPending(ctx *PendingContext) interface{}

	// Visit a parse tree produced by TransactionParser#description.
	VisitDescription(ctx *DescriptionContext) interface{}

	// Visit a parse tree produced by TransactionParser#chartOfAccounts.
	VisitChartOfAccounts(ctx *ChartOfAccountsContext) interface{}

	// Visit a parse tree produced by TransactionParser#metadata.
	VisitMetadata(ctx *MetadataContext) interface{}

	// Visit a parse tree produced by TransactionParser#pair.
	VisitPair(ctx *PairContext) interface{}

	// Visit a parse tree produced by TransactionParser#key.
	VisitKey(ctx *KeyContext) interface{}

	// Visit a parse tree produced by TransactionParser#value.
	VisitValue(ctx *ValueContext) interface{}

	// Visit a parse tree produced by TransactionParser#valueOrVariable.
	VisitValueOrVariable(ctx *ValueOrVariableContext) interface{}

	// Visit a parse tree produced by TransactionParser#Amount.
	VisitAmount(ctx *AmountContext) interface{}

	// Visit a parse tree produced by TransactionParser#ShareIntOfInt.
	VisitShareIntOfInt(ctx *ShareIntOfIntContext) interface{}

	// Visit a parse tree produced by TransactionParser#ShareInt.
	VisitShareInt(ctx *ShareIntContext) interface{}

	// Visit a parse tree produced by TransactionParser#Remaining.
	VisitRemaining(ctx *RemainingContext) interface{}

	// Visit a parse tree produced by TransactionParser#account.
	VisitAccount(ctx *AccountContext) interface{}

	// Visit a parse tree produced by TransactionParser#rate.
	VisitRate(ctx *RateContext) interface{}

	// Visit a parse tree produced by TransactionParser#from.
	VisitFrom(ctx *FromContext) interface{}

	// Visit a parse tree produced by TransactionParser#source.
	VisitSource(ctx *SourceContext) interface{}

	// Visit a parse tree produced by TransactionParser#to.
	VisitTo(ctx *ToContext) interface{}

	// Visit a parse tree produced by TransactionParser#distribute.
	VisitDistribute(ctx *DistributeContext) interface{}

	// Visit a parse tree produced by TransactionParser#send.
	VisitSend(ctx *SendContext) interface{}
}
