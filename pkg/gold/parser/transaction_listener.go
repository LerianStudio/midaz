// Code generated from Transaction.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // Transaction

import "github.com/antlr4-go/antlr/v4"

// TransactionListener is a complete listener for a parse tree produced by TransactionParser.
type TransactionListener interface {
	antlr.ParseTreeListener

	// EnterTransaction is called when entering the transaction production.
	EnterTransaction(c *TransactionContext)

	// EnterChartOfAccountsGroupName is called when entering the chartOfAccountsGroupName production.
	EnterChartOfAccountsGroupName(c *ChartOfAccountsGroupNameContext)

	// EnterCode is called when entering the code production.
	EnterCode(c *CodeContext)

	// EnterTrueOrFalse is called when entering the trueOrFalse production.
	EnterTrueOrFalse(c *TrueOrFalseContext)

	// EnterPending is called when entering the pending production.
	EnterPending(c *PendingContext)

	// EnterDescription is called when entering the description production.
	EnterDescription(c *DescriptionContext)

	// EnterChartOfAccounts is called when entering the chartOfAccounts production.
	EnterChartOfAccounts(c *ChartOfAccountsContext)

	// EnterMetadata is called when entering the metadata production.
	EnterMetadata(c *MetadataContext)

	// EnterPair is called when entering the pair production.
	EnterPair(c *PairContext)

	// EnterKey is called when entering the key production.
	EnterKey(c *KeyContext)

	// EnterValue is called when entering the value production.
	EnterValue(c *ValueContext)

	// EnterValueOrVariable is called when entering the valueOrVariable production.
	EnterValueOrVariable(c *ValueOrVariableContext)

	// EnterAmount is called when entering the Amount production.
	EnterAmount(c *AmountContext)

	// EnterShareIntOfInt is called when entering the ShareIntOfInt production.
	EnterShareIntOfInt(c *ShareIntOfIntContext)

	// EnterShareInt is called when entering the ShareInt production.
	EnterShareInt(c *ShareIntContext)

	// EnterRemaining is called when entering the Remaining production.
	EnterRemaining(c *RemainingContext)

	// EnterAccount is called when entering the account production.
	EnterAccount(c *AccountContext)

	// EnterRate is called when entering the rate production.
	EnterRate(c *RateContext)

	// EnterFrom is called when entering the from production.
	EnterFrom(c *FromContext)

	// EnterSource is called when entering the source production.
	EnterSource(c *SourceContext)

	// EnterTo is called when entering the to production.
	EnterTo(c *ToContext)

	// EnterDistribute is called when entering the distribute production.
	EnterDistribute(c *DistributeContext)

	// EnterSend is called when entering the send production.
	EnterSend(c *SendContext)

	// ExitTransaction is called when exiting the transaction production.
	ExitTransaction(c *TransactionContext)

	// ExitChartOfAccountsGroupName is called when exiting the chartOfAccountsGroupName production.
	ExitChartOfAccountsGroupName(c *ChartOfAccountsGroupNameContext)

	// ExitCode is called when exiting the code production.
	ExitCode(c *CodeContext)

	// ExitTrueOrFalse is called when exiting the trueOrFalse production.
	ExitTrueOrFalse(c *TrueOrFalseContext)

	// ExitPending is called when exiting the pending production.
	ExitPending(c *PendingContext)

	// ExitDescription is called when exiting the description production.
	ExitDescription(c *DescriptionContext)

	// ExitChartOfAccounts is called when exiting the chartOfAccounts production.
	ExitChartOfAccounts(c *ChartOfAccountsContext)

	// ExitMetadata is called when exiting the metadata production.
	ExitMetadata(c *MetadataContext)

	// ExitPair is called when exiting the pair production.
	ExitPair(c *PairContext)

	// ExitKey is called when exiting the key production.
	ExitKey(c *KeyContext)

	// ExitValue is called when exiting the value production.
	ExitValue(c *ValueContext)

	// ExitValueOrVariable is called when exiting the valueOrVariable production.
	ExitValueOrVariable(c *ValueOrVariableContext)

	// ExitAmount is called when exiting the Amount production.
	ExitAmount(c *AmountContext)

	// ExitShareIntOfInt is called when exiting the ShareIntOfInt production.
	ExitShareIntOfInt(c *ShareIntOfIntContext)

	// ExitShareInt is called when exiting the ShareInt production.
	ExitShareInt(c *ShareIntContext)

	// ExitRemaining is called when exiting the Remaining production.
	ExitRemaining(c *RemainingContext)

	// ExitAccount is called when exiting the account production.
	ExitAccount(c *AccountContext)

	// ExitRate is called when exiting the rate production.
	ExitRate(c *RateContext)

	// ExitFrom is called when exiting the from production.
	ExitFrom(c *FromContext)

	// ExitSource is called when exiting the source production.
	ExitSource(c *SourceContext)

	// ExitTo is called when exiting the to production.
	ExitTo(c *ToContext)

	// ExitDistribute is called when exiting the distribute production.
	ExitDistribute(c *DistributeContext)

	// ExitSend is called when exiting the send production.
	ExitSend(c *SendContext)
}
