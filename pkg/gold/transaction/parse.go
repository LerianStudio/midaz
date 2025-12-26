package transaction

import (
	"fmt"
	"strconv"
	"strings"

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
	err error
}

func NewTransactionVisitor() *TransactionVisitor {
	return new(TransactionVisitor)
}

func (v *TransactionVisitor) Visit(tree antlr.ParseTree) any { return tree.Accept(v) }

func (v *TransactionVisitor) VisitTransaction(ctx *parser.TransactionContext) any {
	if ctx == nil {
		v.setError(fmt.Errorf("transaction context is nil"))
		return nil
	}

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

	var send pkgTransaction.Send
	if ctx.Send() != nil {
		send = v.VisitSend(ctx.Send().(*parser.SendContext)).(pkgTransaction.Send)
	} else {
		v.setError(fmt.Errorf("send section is missing"))
	}

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
	if ctx == nil || ctx.UUID() == nil {
		v.setError(fmt.Errorf("chart-of-accounts-group-name must be UUID"))
		return ""
	}

	return ctx.UUID().GetText()
}

func (v *TransactionVisitor) VisitCode(ctx *parser.CodeContext) any {
	if ctx == nil {
		return ""
	}

	if ctx.UUID() != nil {
		return ctx.UUID().GetText()
	}

	return ""
}

func (v *TransactionVisitor) VisitPending(ctx *parser.PendingContext) any {
	if ctx == nil {
		return false
	}

	if ctx.TrueOrFalse() != nil {
		text := ctx.TrueOrFalse().GetText()
		pending, err := strconv.ParseBool(text)
		if err != nil {
			v.setError(fmt.Errorf("invalid pending boolean: %w", err))
			return false
		}

		return pending
	}

	return false
}

func (v *TransactionVisitor) VisitDescription(ctx *parser.DescriptionContext) any {
	if ctx == nil || ctx.STRING() == nil {
		return ""
	}

	return strings.Trim(ctx.STRING().GetText(), "\"")
}

func (v *TransactionVisitor) VisitVisitChartOfAccounts(ctx *parser.ChartOfAccountsContext) any {
	if ctx == nil || ctx.UUID() == nil {
		v.setError(fmt.Errorf("chart-of-accounts must be UUID"))
		return ""
	}

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
	if ctx == nil || ctx.Key() == nil || ctx.Value() == nil {
		v.setError(fmt.Errorf("invalid metadata pair"))
		return pkgTransaction.Metadata{}
	}

	return pkgTransaction.Metadata{
		Key:   ctx.Key().GetText(),
		Value: ctx.Value().GetText(),
	}
}

func (v *TransactionVisitor) VisitNumericValue(ctx *parser.NumericValueContext) any {
	if ctx == nil || ctx.INT() == nil {
		v.setError(fmt.Errorf("numeric value is missing"))
		return ""
	}

	return ctx.INT().GetText()
}

// parseDecimalWithScale parses a value|scale pair and returns the scaled decimal.
// For example, "100" with scale "2" returns 1.00 (100 * 10^-2).
func (v *TransactionVisitor) parseDecimalWithScale(valueCtx, scaleCtx parser.INumericValueContext, context string) decimal.Decimal {
	valueNode := numericValueContext(valueCtx)
	scaleNode := numericValueContext(scaleCtx)

	if valueNode == nil || scaleNode == nil {
		v.setError(fmt.Errorf("missing %s numeric value or scale", context))
		return decimal.Zero
	}

	valStr := v.VisitNumericValue(valueNode).(string)
	scaleStr := v.VisitNumericValue(scaleNode).(string)

	if valStr == "" || scaleStr == "" {
		v.setError(fmt.Errorf("invalid %s numeric value or scale", context))
		return decimal.Zero
	}

	value, err := decimal.NewFromString(valStr)
	if err != nil {
		v.setError(fmt.Errorf("invalid %s decimal value: %w", context, err))
		return decimal.Zero
	}

	scale, err := strconv.ParseInt(scaleStr, 10, 32)
	if err != nil {
		v.setError(fmt.Errorf("invalid %s scale: %w", context, err))
		return decimal.Zero
	}

	return value.Shift(-int32(scale))
}

func (v *TransactionVisitor) VisitSend(ctx *parser.SendContext) any {
	if ctx == nil {
		v.setError(fmt.Errorf("send context is nil"))
		return pkgTransaction.Send{}
	}

	asset := ""
	if ctx.UUID() != nil {
		asset = ctx.UUID().GetText()
	} else {
		v.setError(fmt.Errorf("send asset must be UUID"))
	}

	var source pkgTransaction.Source
	if ctx.Source() != nil {
		source = v.VisitSource(ctx.Source().(*parser.SourceContext)).(pkgTransaction.Source)
	} else {
		v.setError(fmt.Errorf("send source is missing"))
	}

	var distribute pkgTransaction.Distribute
	if ctx.Distribute() != nil {
		distribute = v.VisitDistribute(ctx.Distribute().(*parser.DistributeContext)).(pkgTransaction.Distribute)
	} else {
		v.setError(fmt.Errorf("send distribute is missing"))
	}

	value := v.parseDecimalWithScale(
		ctx.NumericValue(0),
		ctx.NumericValue(1),
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
	if ctx == nil {
		v.setError(fmt.Errorf("source context is nil"))
		return pkgTransaction.Source{}
	}

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
	if ctx == nil {
		v.setError(fmt.Errorf("account context is nil"))
		return ""
	}

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
	if ctx == nil {
		v.setError(fmt.Errorf("rate context is nil"))
		return pkgTransaction.Rate{}
	}

	if ctx.UUID(0) == nil || ctx.UUID(1) == nil || ctx.UUID(rateToUUIDIndex) == nil {
		v.setError(fmt.Errorf("rate UUID fields missing"))
		return pkgTransaction.Rate{}
	}

	externalID := ctx.UUID(0).GetText()
	from := ctx.UUID(1).GetText()
	to := ctx.UUID(rateToUUIDIndex).GetText()

	value := v.parseDecimalWithScale(
		ctx.NumericValue(0),
		ctx.NumericValue(1),
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
	if ctx == nil {
		v.setError(fmt.Errorf("remaining context is nil"))
		return ""
	}

	return strings.Trim(ctx.GetText(), ":")
}

func (v *TransactionVisitor) VisitAmount(ctx *parser.AmountContext) any {
	if ctx == nil {
		v.setError(fmt.Errorf("amount context is nil"))
		return pkgTransaction.Amount{}
	}

	asset := ""
	if ctx.UUID() != nil {
		asset = ctx.UUID().GetText()
	} else {
		v.setError(fmt.Errorf("amount asset must be UUID"))
	}

	value := v.parseDecimalWithScale(
		ctx.NumericValue(0),
		ctx.NumericValue(1),
		"amount",
	)

	return pkgTransaction.Amount{
		Asset: asset,
		Value: value,
	}
}

func (v *TransactionVisitor) VisitShareInt(ctx *parser.ShareIntContext) any {
	if ctx == nil {
		v.setError(fmt.Errorf("share context is nil"))
		return pkgTransaction.Share{}
	}

	percentageStr := v.VisitNumericValue(numericValueContext(ctx.NumericValue())).(string)
	percentage, err := strconv.ParseInt(percentageStr, 10, 64)
	if err != nil {
		v.setError(fmt.Errorf("invalid share percentage: %w", err))
		return pkgTransaction.Share{}
	}

	return pkgTransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: 0,
	}
}

func (v *TransactionVisitor) VisitShareIntOfInt(ctx *parser.ShareIntOfIntContext) any {
	if ctx == nil {
		v.setError(fmt.Errorf("share context is nil"))
		return pkgTransaction.Share{}
	}

	percentageStr := v.VisitNumericValue(numericValueContext(ctx.NumericValue(0))).(string)
	percentage, err := strconv.ParseInt(percentageStr, 10, 64)
	if err != nil {
		v.setError(fmt.Errorf("invalid share percentage: %w", err))
		return pkgTransaction.Share{}
	}

	percentageOfPercentageStr := v.VisitNumericValue(numericValueContext(ctx.NumericValue(1))).(string)
	percentageOfPercentage, err := strconv.ParseInt(percentageOfPercentageStr, 10, 64)
	if err != nil {
		v.setError(fmt.Errorf("invalid share percentageOfPercentage: %w", err))
		return pkgTransaction.Share{}
	}

	return pkgTransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: percentageOfPercentage,
	}
}

func (v *TransactionVisitor) VisitFrom(ctx *parser.FromContext) any {
	if ctx == nil {
		v.setError(fmt.Errorf("from context is nil"))
		return pkgTransaction.FromTo{}
	}

	account := ""
	if ctx.Account() != nil {
		account = v.VisitAccount(ctx.Account().(*parser.AccountContext)).(string)
	} else {
		v.setError(fmt.Errorf("from account is missing"))
	}

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

	switch sendTypes := ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(sendTypes).(pkgTransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(sendTypes).(pkgTransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(sendTypes).(pkgTransaction.Share)
	case *parser.RemainingContext:
		remaining = v.VisitRemaining(sendTypes).(string)
	default:
		v.setError(fmt.Errorf("from send type is missing"))
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
	if ctx == nil {
		v.setError(fmt.Errorf("to context is nil"))
		return pkgTransaction.FromTo{}
	}

	account := ""
	if ctx.Account() != nil {
		account = v.VisitAccount(ctx.Account().(*parser.AccountContext)).(string)
	} else {
		v.setError(fmt.Errorf("to account is missing"))
	}

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

	switch sendTypes := ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(sendTypes).(pkgTransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(sendTypes).(pkgTransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(sendTypes).(pkgTransaction.Share)
	case *parser.RemainingContext:
		remaining = v.VisitRemaining(sendTypes).(string)
	default:
		v.setError(fmt.Errorf("to send type is missing"))
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
	if ctx == nil {
		v.setError(fmt.Errorf("distribute context is nil"))
		return pkgTransaction.Distribute{}
	}

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
	lexerErrors := &Error{}
	parserErrors := &Error{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(lexerErrors)
	p.RemoveErrorListeners()
	p.AddErrorListener(parserErrors)
	p.BuildParseTrees = true
	p.AddErrorListener(antlr.NewDiagnosticErrorListener(true))

	tree := p.Transaction()
	if len(lexerErrors.Errors) > 0 || len(parserErrors.Errors) > 0 {
		return nil
	}

	visitor := NewTransactionVisitor()
	transaction := visitor.Visit(tree)

	if visitor.err != nil {
		return nil
	}

	return transaction
}

func (v *TransactionVisitor) setError(err error) {
	if err == nil || v.err != nil {
		return
	}

	v.err = err
}

func numericValueContext(ctx parser.INumericValueContext) *parser.NumericValueContext {
	if ctx == nil {
		return nil
	}

	valueNode, ok := ctx.(*parser.NumericValueContext)
	if !ok {
		return nil
	}

	return valueNode
}
