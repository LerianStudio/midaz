// Package transaction provides the Gold DSL parser and visitor for transaction definitions.
//
// This package implements a domain-specific language (DSL) parser for defining financial
// transactions in a human-readable format. The Gold DSL enables:
//   - Declarative transaction definitions
//   - Multi-source/multi-destination transfers
//   - Proportional distribution (shares, percentages)
//   - Currency conversion with rates
//   - Embedded metadata
//
// Gold DSL Syntax Overview:
//
// The Gold DSL uses a structured format to describe transactions:
//
//	chart_of_accounts(<uuid>)
//	description("<description>")
//	code(<uuid>)
//	pending(<true|false>)
//	metadata { key: value, ... }
//	send <amount> <asset> (
//	    source (
//	        from @<account> remaining
//	        from @<account> amount <value> <asset>
//	        from @<account> share <percentage>%
//	    )
//	    distribute (
//	        to @<account> remaining
//	        to @<account> amount <value> <asset>
//	        to @<account> share <percentage>%
//	    )
//	)
//
// Example Transaction:
//
//	chart_of_accounts(550e8400-e29b-41d4-a716-446655440000)
//	description("Transfer from savings to checking")
//	send 1000 USD (
//	    source (
//	        from @savings remaining
//	    )
//	    distribute (
//	        to @checking remaining
//	    )
//	)
//
// Distribution Types:
//
//   - remaining: Allocates whatever amount is left after other allocations
//   - amount: Fixed amount in specified asset
//   - share: Percentage of total (e.g., 50%)
//   - share of share: Percentage of percentage (e.g., 50% of 80%)
//
// Parser Implementation:
//
// The parser is generated using ANTLR4 from a grammar file. This package provides
// the visitor implementation that traverses the parse tree and constructs the
// libTransaction.Transaction struct.
//
// Related Packages:
//   - github.com/LerianStudio/lib-commons/v2/commons/transaction: Transaction domain types
//   - apps/midaz/pkg/gold/parser: ANTLR-generated parser code
package transaction

import (
	"strconv"
	"strings"

	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
	"github.com/antlr4-go/antlr/v4"
	"github.com/shopspring/decimal"
)

// TransactionVisitor implements the ANTLR visitor pattern for Gold DSL parsing.
//
// The visitor traverses the parse tree produced by the ANTLR-generated parser
// and constructs a libTransaction.Transaction struct. Each Visit* method
// handles a specific grammar rule.
//
// Visitor Pattern Benefits:
//   - Separates parsing logic from grammar definition
//   - Type-safe traversal of parse tree
//   - Easy to extend with new grammar rules
//
// Thread Safety:
// TransactionVisitor is NOT safe for concurrent use. Create a new visitor
// for each parsing operation.
type TransactionVisitor struct {
	*parser.BaseTransactionVisitor
}

// NewTransactionVisitor creates a new visitor for parsing Gold DSL transactions.
//
// Returns:
//   - *TransactionVisitor: A new visitor instance ready for parsing
//
// Usage:
//
//	visitor := NewTransactionVisitor()
//	transaction := visitor.Visit(parseTree)
func NewTransactionVisitor() *TransactionVisitor {
	return new(TransactionVisitor)
}

// Visit initiates the visitor traversal of the parse tree.
//
// This is the entry point for the visitor pattern. It dispatches to the
// appropriate Visit* method based on the tree node type.
//
// Parameters:
//   - tree: The ANTLR parse tree to traverse
//
// Returns:
//   - any: The result of visiting the tree (typically libTransaction.Transaction)
func (v *TransactionVisitor) Visit(tree antlr.ParseTree) any { return tree.Accept(v) }

// VisitTransaction processes the root transaction node and assembles the Transaction struct.
//
// This method is called for the top-level transaction rule in the grammar.
// It extracts all transaction components (description, code, metadata, send, etc.)
// and constructs the final libTransaction.Transaction.
//
// Parsing Process:
//
//	Step 1: Extract optional description (quoted string)
//	Step 2: Extract optional code (UUID for idempotency)
//	Step 3: Extract optional pending flag (deferred execution)
//	Step 4: Extract optional metadata (key-value pairs)
//	Step 5: Extract required send block (amounts and distributions)
//	Step 6: Assemble Transaction struct
//
// Parameters:
//   - ctx: The transaction context from the parse tree
//
// Returns:
//   - any: libTransaction.Transaction struct
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

	send := v.VisitSend(ctx.Send().(*parser.SendContext)).(libTransaction.Send)

	transaction := libTransaction.Transaction{
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
		m := v.VisitPair(pair.(*parser.PairContext)).(libTransaction.Metadata)
		metadata[m.Key] = m.Value
	}

	return metadata
}

func (v *TransactionVisitor) VisitPair(ctx *parser.PairContext) any {
	return libTransaction.Metadata{
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

// VisitSend processes the send block which defines the transaction's financial flow.
//
// The send block is the core of a Gold DSL transaction, specifying:
//   - Asset code (e.g., USD, BRL, BTC)
//   - Total amount to transfer
//   - Source accounts (where funds come from)
//   - Distribution accounts (where funds go to)
//
// Send Block Structure:
//
//	send <amount> <asset> (
//	    source ( ... )
//	    distribute ( ... )
//	)
//
// Parameters:
//   - ctx: The send context from the parse tree
//
// Returns:
//   - any: libTransaction.Send struct containing asset, value, source, and distribution
func (v *TransactionVisitor) VisitSend(ctx *parser.SendContext) any {
	asset := ctx.UUID().GetText()
	val := v.VisitValueOrVariable(ctx.ValueOrVariable(0).(*parser.ValueOrVariableContext)).(string)
	source := v.VisitSource(ctx.Source().(*parser.SourceContext)).(libTransaction.Source)
	distribute := v.VisitDistribute(ctx.Distribute().(*parser.DistributeContext)).(libTransaction.Distribute)

	value, _ := decimal.NewFromString(val)

	return libTransaction.Send{
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

	froms := make([]libTransaction.FromTo, 0, len(ctx.AllFrom()))

	for _, from := range ctx.AllFrom() {
		f := v.VisitFrom(from.(*parser.FromContext)).(libTransaction.FromTo)
		froms = append(froms, f)
	}

	return libTransaction.Source{
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

	return libTransaction.Rate{
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

	return libTransaction.Amount{
		Asset: asset,
		Value: value,
	}
}

func (v *TransactionVisitor) VisitShareInt(ctx *parser.ShareIntContext) any {
	percentage, _ := strconv.ParseInt(v.VisitValueOrVariable(ctx.ValueOrVariable().(*parser.ValueOrVariableContext)).(string), 10, 64)

	return libTransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: 0,
	}
}

func (v *TransactionVisitor) VisitShareIntOfInt(ctx *parser.ShareIntOfIntContext) any {
	percentage, _ := strconv.ParseInt(v.VisitValueOrVariable(ctx.ValueOrVariable(0).(*parser.ValueOrVariableContext)).(string), 10, 64)
	percentageOfPercentage, _ := strconv.ParseInt(v.VisitValueOrVariable(ctx.ValueOrVariable(1).(*parser.ValueOrVariableContext)).(string), 10, 64)

	return libTransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: percentageOfPercentage,
	}
}

// VisitFrom processes a source account entry in the transaction.
//
// A "from" entry specifies:
//   - Account alias (where funds are withdrawn)
//   - Allocation type (remaining, amount, or share)
//   - Optional currency conversion rate
//   - Optional entry-level description and metadata
//
// Allocation Types:
//   - remaining: Use whatever is left after other allocations
//   - amount: Fixed amount in specified asset
//   - share: Percentage of total (e.g., 50%)
//   - share of share: Nested percentage (e.g., 50% of 80%)
//
// Parameters:
//   - ctx: The from context from the parse tree
//
// Returns:
//   - any: libTransaction.FromTo struct with IsFrom=true
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

	var amount libTransaction.Amount

	var share libTransaction.Share

	var remaining string

	switch ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(ctx.SendTypes().(*parser.AmountContext)).(libTransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(ctx.SendTypes().(*parser.ShareIntContext)).(libTransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(ctx.SendTypes().(*parser.ShareIntOfIntContext)).(libTransaction.Share)
	default:
		remaining = v.VisitRemaining(ctx.SendTypes().(*parser.RemainingContext)).(string)
	}

	var rate *libTransaction.Rate

	if ctx.Rate() != nil {
		rateValue := v.VisitRate(ctx.Rate().(*parser.RateContext)).(libTransaction.Rate)

		if !rateValue.IsEmpty() {
			rate = &rateValue
		}
	}

	return libTransaction.FromTo{
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

// VisitTo processes a destination account entry in the transaction.
//
// A "to" entry specifies:
//   - Account alias (where funds are deposited)
//   - Allocation type (remaining, amount, or share)
//   - Optional currency conversion rate
//   - Optional entry-level description and metadata
//
// Parameters:
//   - ctx: The to context from the parse tree
//
// Returns:
//   - any: libTransaction.FromTo struct with IsFrom=false
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

	var amount libTransaction.Amount

	var share libTransaction.Share

	var remaining string

	switch ctx.SendTypes().(type) {
	case *parser.AmountContext:
		amount = v.VisitAmount(ctx.SendTypes().(*parser.AmountContext)).(libTransaction.Amount)
	case *parser.ShareIntContext:
		share = v.VisitShareInt(ctx.SendTypes().(*parser.ShareIntContext)).(libTransaction.Share)
	case *parser.ShareIntOfIntContext:
		share = v.VisitShareIntOfInt(ctx.SendTypes().(*parser.ShareIntOfIntContext)).(libTransaction.Share)
	default:
		remaining = v.VisitRemaining(ctx.SendTypes().(*parser.RemainingContext)).(string)
	}

	var rate *libTransaction.Rate

	if ctx.Rate() != nil {
		rateValue := v.VisitRate(ctx.Rate().(*parser.RateContext)).(libTransaction.Rate)

		if !rateValue.IsEmpty() {
			rate = &rateValue
		}
	}

	return libTransaction.FromTo{
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

	tos := make([]libTransaction.FromTo, 0, len(ctx.AllTo()))

	for _, to := range ctx.AllTo() {
		t := v.VisitTo(to.(*parser.ToContext)).(libTransaction.FromTo)
		tos = append(tos, t)
	}

	return libTransaction.Distribute{
		Remaining: remaining,
		To:        tos,
	}
}

// Parse parses a Gold DSL string and returns a Transaction struct.
//
// This is the primary entry point for parsing Gold DSL transaction definitions.
// It handles the complete parsing pipeline:
//
// Parsing Pipeline:
//
//	Step 1: Create input stream from DSL string
//	Step 2: Tokenize with ANTLR lexer
//	Step 3: Create token stream for parser
//	Step 4: Parse tokens into parse tree
//	Step 5: Visit parse tree to build Transaction
//
// Parameters:
//   - dsl: The Gold DSL transaction definition string
//
// Returns:
//   - any: libTransaction.Transaction struct (cast to use)
//
// Example:
//
//	dsl := `chart_of_accounts(550e8400-...)
//	        send 1000 USD (
//	            source ( from @savings remaining )
//	            distribute ( to @checking remaining )
//	        )`
//	result := transaction.Parse(dsl)
//	tx := result.(libTransaction.Transaction)
//
// Error Handling:
// The current implementation does not return parse errors explicitly.
// Invalid DSL may result in a panic or incomplete Transaction.
// Consider using ParseWithErrors for production use.
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
