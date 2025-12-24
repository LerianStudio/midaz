package transaction

import (
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/antlr4-go/antlr/v4"
	"github.com/shopspring/decimal"
)

const (
	rateToUUIDIndex = 2
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

	send := v.VisitSend(ctx.Send().(*parser.SendContext)).(pkgTransaction.Send)

	transaction := pkgTransaction.Transaction{
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
		text := ctx.TrueOrFalse().GetText()
		pending, err := strconv.ParseBool(text)
		assert.NoError(err, "DSL boolean must be valid after ANTLR validation", "value", text)

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
		m := v.VisitPair(pair.(*parser.PairContext)).(pkgTransaction.Metadata)
		metadata[m.Key] = m.Value
	}

	return metadata
}

func (v *TransactionVisitor) VisitPair(ctx *parser.PairContext) any {
	return pkgTransaction.Metadata{
		Key:   ctx.Key().GetText(),
		Value: ctx.Value().GetText(),
	}
}

func (v *TransactionVisitor) VisitNumericValue(ctx *parser.NumericValueContext) any {
	return ctx.INT().GetText()
}

// parseDecimalWithScale parses a value|scale pair and returns the scaled decimal.
// For example, "100" with scale "2" returns 1.00 (100 * 10^-2).
func (v *TransactionVisitor) parseDecimalWithScale(valueCtx, scaleCtx *parser.NumericValueContext, context string) decimal.Decimal {
	valStr := v.VisitNumericValue(valueCtx).(string)
	scaleStr := v.VisitNumericValue(scaleCtx).(string)

	value, err := decimal.NewFromString(valStr)
	assert.NoError(err, "DSL "+context+" value must be valid decimal after ANTLR validation", "value", valStr)

	scale, err := strconv.ParseInt(scaleStr, 10, 32)
	assert.NoError(err, "DSL "+context+" scale must be valid integer after ANTLR validation", "scale", scaleStr)

	return value.Shift(-int32(scale))
}

func (v *TransactionVisitor) VisitSend(ctx *parser.SendContext) any {
	asset := ctx.UUID().GetText()
	source := v.VisitSource(ctx.Source().(*parser.SourceContext)).(pkgTransaction.Source)
	distribute := v.VisitDistribute(ctx.Distribute().(*parser.DistributeContext)).(pkgTransaction.Distribute)

	value := v.parseDecimalWithScale(
		ctx.NumericValue(0).(*parser.NumericValueContext),
		ctx.NumericValue(1).(*parser.NumericValueContext),
		"send",
	)

	return pkgTransaction.Send{
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

	froms := make([]pkgTransaction.FromTo, 0, len(ctx.AllFrom()))

	for _, from := range ctx.AllFrom() {
		f := v.VisitFrom(from.(*parser.FromContext)).(pkgTransaction.FromTo)
		froms = append(froms, f)
	}

	return pkgTransaction.Source{
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
	to := ctx.UUID(rateToUUIDIndex).GetText()

	value := v.parseDecimalWithScale(
		ctx.NumericValue(0).(*parser.NumericValueContext),
		ctx.NumericValue(1).(*parser.NumericValueContext),
		"rate",
	)

	return pkgTransaction.Rate{
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

	value := v.parseDecimalWithScale(
		ctx.NumericValue(0).(*parser.NumericValueContext),
		ctx.NumericValue(1).(*parser.NumericValueContext),
		"amount",
	)

	return pkgTransaction.Amount{
		Asset: asset,
		Value: value,
	}
}

func (v *TransactionVisitor) VisitShareInt(ctx *parser.ShareIntContext) any {
	percentageStr := v.VisitNumericValue(ctx.NumericValue().(*parser.NumericValueContext)).(string)
	percentage, err := strconv.ParseInt(percentageStr, 10, 64)
	assert.NoError(err, "DSL share percentage must be valid integer after ANTLR validation", "value", percentageStr)

	return pkgTransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: 0,
	}
}

func (v *TransactionVisitor) VisitShareIntOfInt(ctx *parser.ShareIntOfIntContext) any {
	percentageStr := v.VisitNumericValue(ctx.NumericValue(0).(*parser.NumericValueContext)).(string)
	percentage, err := strconv.ParseInt(percentageStr, 10, 64)
	assert.NoError(err, "DSL share percentage must be valid integer after ANTLR validation", "value", percentageStr)

	percentageOfPercentageStr := v.VisitNumericValue(ctx.NumericValue(1).(*parser.NumericValueContext)).(string)
	percentageOfPercentage, err := strconv.ParseInt(percentageOfPercentageStr, 10, 64)
	assert.NoError(err, "DSL share percentageOfPercentage must be valid integer after ANTLR validation", "value", percentageOfPercentageStr)

	return pkgTransaction.Share{
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

	var amount pkgTransaction.Amount

	var share pkgTransaction.Share

	var remaining string

	switch ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(ctx.SendTypes().(*parser.AmountContext)).(pkgTransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(ctx.SendTypes().(*parser.ShareIntContext)).(pkgTransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(ctx.SendTypes().(*parser.ShareIntOfIntContext)).(pkgTransaction.Share)
	default:
		remaining = v.VisitRemaining(ctx.SendTypes().(*parser.RemainingContext)).(string)
	}

	var rate *pkgTransaction.Rate

	if ctx.Rate() != nil {
		rateValue := v.VisitRate(ctx.Rate().(*parser.RateContext)).(pkgTransaction.Rate)

		if !rateValue.IsEmpty() {
			rate = &rateValue
		}
	}

	return pkgTransaction.FromTo{
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

	var amount pkgTransaction.Amount

	var share pkgTransaction.Share

	var remaining string

	switch ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(ctx.SendTypes().(*parser.AmountContext)).(pkgTransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(ctx.SendTypes().(*parser.ShareIntContext)).(pkgTransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(ctx.SendTypes().(*parser.ShareIntOfIntContext)).(pkgTransaction.Share)
	default:
		remaining = v.VisitRemaining(ctx.SendTypes().(*parser.RemainingContext)).(string)
	}

	var rate *pkgTransaction.Rate

	if ctx.Rate() != nil {
		rateValue := v.VisitRate(ctx.Rate().(*parser.RateContext)).(pkgTransaction.Rate)

		if !rateValue.IsEmpty() {
			rate = &rateValue
		}
	}

	return pkgTransaction.FromTo{
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

	tos := make([]pkgTransaction.FromTo, 0, len(ctx.AllTo()))

	for _, to := range ctx.AllTo() {
		t := v.VisitTo(to.(*parser.ToContext)).(pkgTransaction.FromTo)
		tos = append(tos, t)
	}

	return pkgTransaction.Distribute{
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
