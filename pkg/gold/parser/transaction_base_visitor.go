// Code generated from Transaction.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // Transaction

import "github.com/antlr4-go/antlr/v4"

type BaseTransactionVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseTransactionVisitor) VisitTransaction(ctx *TransactionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitChartOfAccountsGroupName(ctx *ChartOfAccountsGroupNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitCode(ctx *CodeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitTrueOrFalse(ctx *TrueOrFalseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitPending(ctx *PendingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitDescription(ctx *DescriptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitChartOfAccounts(ctx *ChartOfAccountsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitMetadata(ctx *MetadataContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitPair(ctx *PairContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitKey(ctx *KeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitValue(ctx *ValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitValueOrVariable(ctx *ValueOrVariableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitAmount(ctx *AmountContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitShareIntOfInt(ctx *ShareIntOfIntContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitShareInt(ctx *ShareIntContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitRemaining(ctx *RemainingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitAccount(ctx *AccountContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitRate(ctx *RateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitFrom(ctx *FromContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitSource(ctx *SourceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitTo(ctx *ToContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitDistribute(ctx *DistributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseTransactionVisitor) VisitSend(ctx *SendContext) interface{} {
	return v.VisitChildren(ctx)
}
