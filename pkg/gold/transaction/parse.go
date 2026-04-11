// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/antlr4-go/antlr/v4"
	"github.com/shopspring/decimal"
)

type TransactionVisitor struct {
	*parser.BaseTransactionVisitor
}

func NewTransactionVisitor() *TransactionVisitor {
	return new(TransactionVisitor)
}

func (v *TransactionVisitor) Visit(tree antlr.ParseTree) any { return tree.Accept(v) }

func (v *TransactionVisitor) VisitTransaction(ctx *parser.TransactionContext) any {
	var description string
	if ctx.Description() != nil {
		description = v.VisitDescription(ctx.Description().(*parser.DescriptionContext)).(string)
	}

	var code string
	if ctx.Code() != nil {
		code = v.VisitCode(ctx.Code().(*parser.CodeContext)).(string)
	}

	var pending bool
	if ctx.Pending() != nil {
		pending = v.VisitPending(ctx.Pending().(*parser.PendingContext)).(bool)
	}

	var metadata map[string]any
	if ctx.Metadata() != nil {
		metadata = v.VisitMetadata(ctx.Metadata().(*parser.MetadataContext)).(map[string]any)
	}

	send := v.VisitSend(ctx.Send().(*parser.SendContext)).(mtransaction.Send)

	transaction := mtransaction.Transaction{
		ChartOfAccountsGroupName: v.VisitVisitChartOfAccountsGroupName(ctx.ChartOfAccountsGroupName().(*parser.ChartOfAccountsGroupNameContext)).(string),
		Description:              description,
		Code:                     code,
		Pending:                  pending,
		Metadata:                 metadata,
		Send:                     send,
	}

	return transaction
}

func (v *TransactionVisitor) VisitVisitChartOfAccountsGroupName(ctx *parser.ChartOfAccountsGroupNameContext) any {
	return ctx.UUID().GetText()
}

func (v *TransactionVisitor) VisitCode(ctx *parser.CodeContext) any {
	if ctx.UUID() != nil {
		return ctx.UUID().GetText()
	}

	return nil
}

func (v *TransactionVisitor) VisitPending(ctx *parser.PendingContext) any {
	if ctx.TrueOrFalse() != nil {
		pending, _ := strconv.ParseBool(ctx.TrueOrFalse().GetText())
		return pending
	}

	return false
}

func (v *TransactionVisitor) VisitDescription(ctx *parser.DescriptionContext) any {
	return strings.Trim(ctx.STRING().GetText(), "\"")
}

func (v *TransactionVisitor) VisitVisitChartOfAccounts(ctx *parser.ChartOfAccountsContext) any {
	return ctx.UUID().GetText()
}

func (v *TransactionVisitor) VisitMetadata(ctx *parser.MetadataContext) any {
	metadata := make(map[string]any, len(ctx.AllPair()))

	for _, pair := range ctx.AllPair() {
		m := v.VisitPair(pair.(*parser.PairContext)).(mtransaction.Metadata)
		metadata[m.Key] = m.Value
	}

	return metadata
}

func (v *TransactionVisitor) VisitPair(ctx *parser.PairContext) any {
	return mtransaction.Metadata{
		Key:   ctx.Key().GetText(),
		Value: ctx.Value().GetText(),
	}
}

func (v *TransactionVisitor) VisitValueOrVariable(ctx *parser.ValueOrVariableContext) any {
	if ctx.INT() != nil {
		return ctx.INT().GetText()
	}

	return ctx.VARIABLE().GetText()
}

func (v *TransactionVisitor) VisitSend(ctx *parser.SendContext) any {
	asset := ctx.UUID().GetText()
	val := v.VisitValueOrVariable(ctx.ValueOrVariable(0).(*parser.ValueOrVariableContext)).(string)
	source := v.VisitSource(ctx.Source().(*parser.SourceContext)).(mtransaction.Source)
	distribute := v.VisitDistribute(ctx.Distribute().(*parser.DistributeContext)).(mtransaction.Distribute)

	value, _ := decimal.NewFromString(val)

	return mtransaction.Send{
		Asset:      asset,
		Value:      value,
		Source:     source,
		Distribute: distribute,
	}
}

func (v *TransactionVisitor) VisitSource(ctx *parser.SourceContext) any {
	var remaining string
	if ctx.REMAINING() != nil {
		remaining = strings.Trim(ctx.REMAINING().GetText(), ":")
	}

	froms := make([]mtransaction.FromTo, 0, len(ctx.AllFrom()))

	for _, from := range ctx.AllFrom() {
		f := v.VisitFrom(from.(*parser.FromContext)).(mtransaction.FromTo)
		froms = append(froms, f)
	}

	return mtransaction.Source{
		Remaining: remaining,
		From:      froms,
	}
}

func (v *TransactionVisitor) VisitAccount(ctx *parser.AccountContext) any {
	switch {
	case ctx.UUID() != nil:
		return ctx.UUID().GetText()
	case ctx.ACCOUNT() != nil:
		return ctx.ACCOUNT().GetText()
	case ctx.VARIABLE() != nil:
		return ctx.VARIABLE().GetText()
	default:
		return ctx.GetText()
	}
}

func (v *TransactionVisitor) VisitRate(ctx *parser.RateContext) any {
	externalID := ctx.UUID(0).GetText()
	from := ctx.UUID(1).GetText()
	to := ctx.UUID(2).GetText()
	val := v.VisitValueOrVariable(ctx.ValueOrVariable(0).(*parser.ValueOrVariableContext)).(string)

	value, _ := decimal.NewFromString(val)

	return mtransaction.Rate{
		From:       from,
		To:         to,
		Value:      value,
		ExternalID: externalID,
	}
}

func (v *TransactionVisitor) VisitRemaining(ctx *parser.RemainingContext) any {
	return strings.Trim(ctx.GetText(), ":")
}

func (v *TransactionVisitor) VisitAmount(ctx *parser.AmountContext) any {
	asset := ctx.UUID().GetText()
	val := v.VisitValueOrVariable(ctx.ValueOrVariable(0).(*parser.ValueOrVariableContext)).(string)

	value, _ := decimal.NewFromString(val)

	return mtransaction.Amount{
		Asset: asset,
		Value: value,
	}
}

func (v *TransactionVisitor) VisitShareInt(ctx *parser.ShareIntContext) any {
	percentage, _ := strconv.ParseInt(v.VisitValueOrVariable(ctx.ValueOrVariable().(*parser.ValueOrVariableContext)).(string), 10, 64)

	return mtransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: 0,
	}
}

func (v *TransactionVisitor) VisitShareIntOfInt(ctx *parser.ShareIntOfIntContext) any {
	percentage, _ := strconv.ParseInt(v.VisitValueOrVariable(ctx.ValueOrVariable(0).(*parser.ValueOrVariableContext)).(string), 10, 64)
	percentageOfPercentage, _ := strconv.ParseInt(v.VisitValueOrVariable(ctx.ValueOrVariable(1).(*parser.ValueOrVariableContext)).(string), 10, 64)

	return mtransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: percentageOfPercentage,
	}
}

func (v *TransactionVisitor) VisitFrom(ctx *parser.FromContext) any {
	account := v.VisitAccount(ctx.Account().(*parser.AccountContext)).(string)

	var description string
	if ctx.Description() != nil {
		description = v.VisitDescription(ctx.Description().(*parser.DescriptionContext)).(string)
	}

	var metadata map[string]any
	if ctx.Metadata() != nil {
		metadata = v.VisitMetadata(ctx.Metadata().(*parser.MetadataContext)).(map[string]any)
	}

	var amount mtransaction.Amount

	var share mtransaction.Share

	var remaining string

	switch ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(ctx.SendTypes().(*parser.AmountContext)).(mtransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(ctx.SendTypes().(*parser.ShareIntContext)).(mtransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(ctx.SendTypes().(*parser.ShareIntOfIntContext)).(mtransaction.Share)
	default:
		remaining = v.VisitRemaining(ctx.SendTypes().(*parser.RemainingContext)).(string)
	}

	var rate *mtransaction.Rate

	if ctx.Rate() != nil {
		rateValue := v.VisitRate(ctx.Rate().(*parser.RateContext)).(mtransaction.Rate)

		if !rateValue.IsEmpty() {
			rate = &rateValue
		}
	}

	return mtransaction.FromTo{
		AccountAlias: account,
		Amount:       &amount,
		Share:        &share,
		Remaining:    remaining,
		Rate:         rate,
		Description:  description,
		Metadata:     metadata,
		IsFrom:       true,
	}
}

func (v *TransactionVisitor) VisitTo(ctx *parser.ToContext) any {
	account := v.VisitAccount(ctx.Account().(*parser.AccountContext)).(string)

	var description string
	if ctx.Description() != nil {
		description = v.VisitDescription(ctx.Description().(*parser.DescriptionContext)).(string)
	}

	var metadata map[string]any
	if ctx.Metadata() != nil {
		metadata = v.VisitMetadata(ctx.Metadata().(*parser.MetadataContext)).(map[string]any)
	}

	var amount mtransaction.Amount

	var share mtransaction.Share

	var remaining string

	switch ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(ctx.SendTypes().(*parser.AmountContext)).(mtransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(ctx.SendTypes().(*parser.ShareIntContext)).(mtransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(ctx.SendTypes().(*parser.ShareIntOfIntContext)).(mtransaction.Share)
	default:
		remaining = v.VisitRemaining(ctx.SendTypes().(*parser.RemainingContext)).(string)
	}

	var rate *mtransaction.Rate

	if ctx.Rate() != nil {
		rateValue := v.VisitRate(ctx.Rate().(*parser.RateContext)).(mtransaction.Rate)

		if !rateValue.IsEmpty() {
			rate = &rateValue
		}
	}

	return mtransaction.FromTo{
		AccountAlias: account,
		Amount:       &amount,
		Share:        &share,
		Remaining:    remaining,
		Rate:         rate,
		Description:  description,
		Metadata:     metadata,
		IsFrom:       false,
	}
}

func (v *TransactionVisitor) VisitDistribute(ctx *parser.DistributeContext) any {
	var remaining string
	if ctx.REMAINING() != nil {
		remaining = strings.Trim(ctx.REMAINING().GetText(), ":")
	}

	tos := make([]mtransaction.FromTo, 0, len(ctx.AllTo()))

	for _, to := range ctx.AllTo() {
		t := v.VisitTo(to.(*parser.ToContext)).(mtransaction.FromTo)
		tos = append(tos, t)
	}

	return mtransaction.Distribute{
		Remaining: remaining,
		To:        tos,
	}
}

func Parse(dsl string) any {
	input := antlr.NewInputStream(dsl)
	lexer := parser.NewTransactionLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)

	p := parser.NewTransactionParser(stream)
	p.BuildParseTrees = true
	visitor := NewTransactionVisitor()
	transaction := visitor.Visit(p.Transaction())

	return transaction
}
