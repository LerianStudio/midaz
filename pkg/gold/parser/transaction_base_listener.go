// Code generated from Transaction.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // Transaction

import "github.com/antlr4-go/antlr/v4"

// BaseTransactionListener is a complete listener for a parse tree produced by TransactionParser.
type BaseTransactionListener struct{}

var _ TransactionListener = &BaseTransactionListener{}

// VisitTerminal is called when a terminal node is visited.
func (s *BaseTransactionListener) VisitTerminal(node antlr.TerminalNode) {}

// VisitErrorNode is called when an error node is visited.
func (s *BaseTransactionListener) VisitErrorNode(node antlr.ErrorNode) {}

// EnterEveryRule is called when any rule is entered.
func (s *BaseTransactionListener) EnterEveryRule(ctx antlr.ParserRuleContext) {}

// ExitEveryRule is called when any rule is exited.
func (s *BaseTransactionListener) ExitEveryRule(ctx antlr.ParserRuleContext) {}

// EnterTransaction is called when production transaction is entered.
func (s *BaseTransactionListener) EnterTransaction(ctx *TransactionContext) {}

// ExitTransaction is called when production transaction is exited.
func (s *BaseTransactionListener) ExitTransaction(ctx *TransactionContext) {}

// EnterChartOfAccountsGroupName is called when production chartOfAccountsGroupName is entered.
func (s *BaseTransactionListener) EnterChartOfAccountsGroupName(ctx *ChartOfAccountsGroupNameContext) {
}

// ExitChartOfAccountsGroupName is called when production chartOfAccountsGroupName is exited.
func (s *BaseTransactionListener) ExitChartOfAccountsGroupName(ctx *ChartOfAccountsGroupNameContext) {
}

// EnterCode is called when production code is entered.
func (s *BaseTransactionListener) EnterCode(ctx *CodeContext) {}

// ExitCode is called when production code is exited.
func (s *BaseTransactionListener) ExitCode(ctx *CodeContext) {}

// EnterTrueOrFalse is called when production trueOrFalse is entered.
func (s *BaseTransactionListener) EnterTrueOrFalse(ctx *TrueOrFalseContext) {}

// ExitTrueOrFalse is called when production trueOrFalse is exited.
func (s *BaseTransactionListener) ExitTrueOrFalse(ctx *TrueOrFalseContext) {}

// EnterPending is called when production pending is entered.
func (s *BaseTransactionListener) EnterPending(ctx *PendingContext) {}

// ExitPending is called when production pending is exited.
func (s *BaseTransactionListener) ExitPending(ctx *PendingContext) {}

// EnterDescription is called when production description is entered.
func (s *BaseTransactionListener) EnterDescription(ctx *DescriptionContext) {}

// ExitDescription is called when production description is exited.
func (s *BaseTransactionListener) ExitDescription(ctx *DescriptionContext) {}

// EnterChartOfAccounts is called when production chartOfAccounts is entered.
func (s *BaseTransactionListener) EnterChartOfAccounts(ctx *ChartOfAccountsContext) {}

// ExitChartOfAccounts is called when production chartOfAccounts is exited.
func (s *BaseTransactionListener) ExitChartOfAccounts(ctx *ChartOfAccountsContext) {}

// EnterMetadata is called when production metadata is entered.
func (s *BaseTransactionListener) EnterMetadata(ctx *MetadataContext) {}

// ExitMetadata is called when production metadata is exited.
func (s *BaseTransactionListener) ExitMetadata(ctx *MetadataContext) {}

// EnterPair is called when production pair is entered.
func (s *BaseTransactionListener) EnterPair(ctx *PairContext) {}

// ExitPair is called when production pair is exited.
func (s *BaseTransactionListener) ExitPair(ctx *PairContext) {}

// EnterKey is called when production key is entered.
func (s *BaseTransactionListener) EnterKey(ctx *KeyContext) {}

// ExitKey is called when production key is exited.
func (s *BaseTransactionListener) ExitKey(ctx *KeyContext) {}

// EnterValue is called when production value is entered.
func (s *BaseTransactionListener) EnterValue(ctx *ValueContext) {}

// ExitValue is called when production value is exited.
func (s *BaseTransactionListener) ExitValue(ctx *ValueContext) {}

// EnterValueOrVariable is called when production valueOrVariable is entered.
func (s *BaseTransactionListener) EnterValueOrVariable(ctx *ValueOrVariableContext) {}

// ExitValueOrVariable is called when production valueOrVariable is exited.
func (s *BaseTransactionListener) ExitValueOrVariable(ctx *ValueOrVariableContext) {}

// EnterAmount is called when production Amount is entered.
func (s *BaseTransactionListener) EnterAmount(ctx *AmountContext) {}

// ExitAmount is called when production Amount is exited.
func (s *BaseTransactionListener) ExitAmount(ctx *AmountContext) {}

// EnterShareIntOfInt is called when production ShareIntOfInt is entered.
func (s *BaseTransactionListener) EnterShareIntOfInt(ctx *ShareIntOfIntContext) {}

// ExitShareIntOfInt is called when production ShareIntOfInt is exited.
func (s *BaseTransactionListener) ExitShareIntOfInt(ctx *ShareIntOfIntContext) {}

// EnterShareInt is called when production ShareInt is entered.
func (s *BaseTransactionListener) EnterShareInt(ctx *ShareIntContext) {}

// ExitShareInt is called when production ShareInt is exited.
func (s *BaseTransactionListener) ExitShareInt(ctx *ShareIntContext) {}

// EnterRemaining is called when production Remaining is entered.
func (s *BaseTransactionListener) EnterRemaining(ctx *RemainingContext) {}

// ExitRemaining is called when production Remaining is exited.
func (s *BaseTransactionListener) ExitRemaining(ctx *RemainingContext) {}

// EnterAccount is called when production account is entered.
func (s *BaseTransactionListener) EnterAccount(ctx *AccountContext) {}

// ExitAccount is called when production account is exited.
func (s *BaseTransactionListener) ExitAccount(ctx *AccountContext) {}

// EnterRate is called when production rate is entered.
func (s *BaseTransactionListener) EnterRate(ctx *RateContext) {}

// ExitRate is called when production rate is exited.
func (s *BaseTransactionListener) ExitRate(ctx *RateContext) {}

// EnterFrom is called when production from is entered.
func (s *BaseTransactionListener) EnterFrom(ctx *FromContext) {}

// ExitFrom is called when production from is exited.
func (s *BaseTransactionListener) ExitFrom(ctx *FromContext) {}

// EnterSource is called when production source is entered.
func (s *BaseTransactionListener) EnterSource(ctx *SourceContext) {}

// ExitSource is called when production source is exited.
func (s *BaseTransactionListener) ExitSource(ctx *SourceContext) {}

// EnterTo is called when production to is entered.
func (s *BaseTransactionListener) EnterTo(ctx *ToContext) {}

// ExitTo is called when production to is exited.
func (s *BaseTransactionListener) ExitTo(ctx *ToContext) {}

// EnterDistribute is called when production distribute is entered.
func (s *BaseTransactionListener) EnterDistribute(ctx *DistributeContext) {}

// ExitDistribute is called when production distribute is exited.
func (s *BaseTransactionListener) ExitDistribute(ctx *DistributeContext) {}

// EnterSend is called when production send is entered.
func (s *BaseTransactionListener) EnterSend(ctx *SendContext) {}

// ExitSend is called when production send is exited.
func (s *BaseTransactionListener) ExitSend(ctx *SendContext) {}
