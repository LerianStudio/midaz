// Code generated from Transaction.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // Transaction

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/antlr4-go/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type TransactionParser struct {
	*antlr.BaseParser
}

var TransactionParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	LiteralNames           []string
	SymbolicNames          []string
	RuleNames              []string
	PredictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func transactionParserInit() {
	staticData := &TransactionParserStaticData
	staticData.LiteralNames = []string{
		"", "'('", "'transaction'", "'transaction-template'", "')'", "'chart-of-accounts-group-name'",
		"'code'", "'false'", "'true'", "'pending'", "'description'", "'chart-of-accounts'",
		"'metadata'", "':amount'", "'|'", "':share'", "':of'", "'rate'", "'->'",
		"'from'", "'source'", "'to'", "'distribute'", "'send'", "'V1'", "",
		"", "", "':remaining'",
	}
	staticData.SymbolicNames = []string{
		"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "",
		"", "", "", "", "", "", "", "VERSION", "INT", "STRING", "UUID", "REMAINING",
		"VARIABLE", "ACCOUNT", "WS",
	}
	staticData.RuleNames = []string{
		"transaction", "chartOfAccountsGroupName", "code", "trueOrFalse", "pending",
		"description", "chartOfAccounts", "metadata", "pair", "key", "value",
		"valueOrVariable", "sendTypes", "account", "rate", "from", "source",
		"to", "distribute", "send",
	}
	staticData.PredictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 31, 206, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 1, 0, 1, 0, 1,
		0, 1, 0, 1, 0, 3, 0, 46, 8, 0, 1, 0, 3, 0, 49, 8, 0, 1, 0, 3, 0, 52, 8,
		0, 1, 0, 3, 0, 55, 8, 0, 1, 0, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 3, 1, 3, 1, 4, 1, 4, 1, 4, 1, 4, 1, 4,
		1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 7, 1, 7,
		1, 7, 4, 7, 90, 8, 7, 11, 7, 12, 7, 91, 1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1,
		8, 1, 8, 1, 9, 1, 9, 1, 10, 1, 10, 1, 11, 1, 11, 1, 12, 1, 12, 1, 12, 1,
		12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12,
		3, 12, 121, 8, 12, 1, 13, 1, 13, 1, 14, 1, 14, 1, 14, 1, 14, 1, 14, 1,
		14, 1, 14, 1, 14, 1, 14, 1, 14, 1, 14, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15,
		3, 15, 141, 8, 15, 1, 15, 3, 15, 144, 8, 15, 1, 15, 3, 15, 147, 8, 15,
		1, 15, 3, 15, 150, 8, 15, 1, 15, 1, 15, 1, 16, 1, 16, 1, 16, 3, 16, 157,
		8, 16, 1, 16, 4, 16, 160, 8, 16, 11, 16, 12, 16, 161, 1, 16, 1, 16, 1,
		17, 1, 17, 1, 17, 1, 17, 1, 17, 3, 17, 171, 8, 17, 1, 17, 3, 17, 174, 8,
		17, 1, 17, 3, 17, 177, 8, 17, 1, 17, 3, 17, 180, 8, 17, 1, 17, 1, 17, 1,
		18, 1, 18, 1, 18, 3, 18, 187, 8, 18, 1, 18, 4, 18, 190, 8, 18, 11, 18,
		12, 18, 191, 1, 18, 1, 18, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1,
		19, 1, 19, 1, 19, 1, 19, 1, 19, 0, 0, 20, 0, 2, 4, 6, 8, 10, 12, 14, 16,
		18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 0, 5, 1, 0, 2, 3, 1, 0, 7,
		8, 2, 0, 25, 25, 27, 27, 2, 0, 25, 25, 29, 29, 2, 0, 27, 27, 29, 30, 205,
		0, 40, 1, 0, 0, 0, 2, 59, 1, 0, 0, 0, 4, 64, 1, 0, 0, 0, 6, 69, 1, 0, 0,
		0, 8, 71, 1, 0, 0, 0, 10, 76, 1, 0, 0, 0, 12, 81, 1, 0, 0, 0, 14, 86, 1,
		0, 0, 0, 16, 95, 1, 0, 0, 0, 18, 100, 1, 0, 0, 0, 20, 102, 1, 0, 0, 0,
		22, 104, 1, 0, 0, 0, 24, 120, 1, 0, 0, 0, 26, 122, 1, 0, 0, 0, 28, 124,
		1, 0, 0, 0, 30, 135, 1, 0, 0, 0, 32, 153, 1, 0, 0, 0, 34, 165, 1, 0, 0,
		0, 36, 183, 1, 0, 0, 0, 38, 195, 1, 0, 0, 0, 40, 41, 5, 1, 0, 0, 41, 42,
		7, 0, 0, 0, 42, 43, 5, 24, 0, 0, 43, 45, 3, 2, 1, 0, 44, 46, 3, 10, 5,
		0, 45, 44, 1, 0, 0, 0, 45, 46, 1, 0, 0, 0, 46, 48, 1, 0, 0, 0, 47, 49,
		3, 4, 2, 0, 48, 47, 1, 0, 0, 0, 48, 49, 1, 0, 0, 0, 49, 51, 1, 0, 0, 0,
		50, 52, 3, 8, 4, 0, 51, 50, 1, 0, 0, 0, 51, 52, 1, 0, 0, 0, 52, 54, 1,
		0, 0, 0, 53, 55, 3, 14, 7, 0, 54, 53, 1, 0, 0, 0, 54, 55, 1, 0, 0, 0, 55,
		56, 1, 0, 0, 0, 56, 57, 3, 38, 19, 0, 57, 58, 5, 4, 0, 0, 58, 1, 1, 0,
		0, 0, 59, 60, 5, 1, 0, 0, 60, 61, 5, 5, 0, 0, 61, 62, 5, 27, 0, 0, 62,
		63, 5, 4, 0, 0, 63, 3, 1, 0, 0, 0, 64, 65, 5, 1, 0, 0, 65, 66, 5, 6, 0,
		0, 66, 67, 5, 27, 0, 0, 67, 68, 5, 4, 0, 0, 68, 5, 1, 0, 0, 0, 69, 70,
		7, 1, 0, 0, 70, 7, 1, 0, 0, 0, 71, 72, 5, 1, 0, 0, 72, 73, 5, 9, 0, 0,
		73, 74, 3, 6, 3, 0, 74, 75, 5, 4, 0, 0, 75, 9, 1, 0, 0, 0, 76, 77, 5, 1,
		0, 0, 77, 78, 5, 10, 0, 0, 78, 79, 5, 26, 0, 0, 79, 80, 5, 4, 0, 0, 80,
		11, 1, 0, 0, 0, 81, 82, 5, 1, 0, 0, 82, 83, 5, 11, 0, 0, 83, 84, 5, 27,
		0, 0, 84, 85, 5, 4, 0, 0, 85, 13, 1, 0, 0, 0, 86, 87, 5, 1, 0, 0, 87, 89,
		5, 12, 0, 0, 88, 90, 3, 16, 8, 0, 89, 88, 1, 0, 0, 0, 90, 91, 1, 0, 0,
		0, 91, 89, 1, 0, 0, 0, 91, 92, 1, 0, 0, 0, 92, 93, 1, 0, 0, 0, 93, 94,
		5, 4, 0, 0, 94, 15, 1, 0, 0, 0, 95, 96, 5, 1, 0, 0, 96, 97, 3, 18, 9, 0,
		97, 98, 3, 20, 10, 0, 98, 99, 5, 4, 0, 0, 99, 17, 1, 0, 0, 0, 100, 101,
		7, 2, 0, 0, 101, 19, 1, 0, 0, 0, 102, 103, 7, 2, 0, 0, 103, 21, 1, 0, 0,
		0, 104, 105, 7, 3, 0, 0, 105, 23, 1, 0, 0, 0, 106, 107, 5, 13, 0, 0, 107,
		108, 5, 27, 0, 0, 108, 109, 3, 22, 11, 0, 109, 110, 5, 14, 0, 0, 110, 111,
		3, 22, 11, 0, 111, 121, 1, 0, 0, 0, 112, 113, 5, 15, 0, 0, 113, 114, 3,
		22, 11, 0, 114, 115, 5, 16, 0, 0, 115, 116, 3, 22, 11, 0, 116, 121, 1,
		0, 0, 0, 117, 118, 5, 15, 0, 0, 118, 121, 3, 22, 11, 0, 119, 121, 5, 28,
		0, 0, 120, 106, 1, 0, 0, 0, 120, 112, 1, 0, 0, 0, 120, 117, 1, 0, 0, 0,
		120, 119, 1, 0, 0, 0, 121, 25, 1, 0, 0, 0, 122, 123, 7, 4, 0, 0, 123, 27,
		1, 0, 0, 0, 124, 125, 5, 1, 0, 0, 125, 126, 5, 17, 0, 0, 126, 127, 5, 27,
		0, 0, 127, 128, 5, 27, 0, 0, 128, 129, 5, 18, 0, 0, 129, 130, 5, 27, 0,
		0, 130, 131, 3, 22, 11, 0, 131, 132, 5, 14, 0, 0, 132, 133, 3, 22, 11,
		0, 133, 134, 5, 4, 0, 0, 134, 29, 1, 0, 0, 0, 135, 136, 5, 1, 0, 0, 136,
		137, 5, 19, 0, 0, 137, 138, 3, 26, 13, 0, 138, 140, 3, 24, 12, 0, 139,
		141, 3, 28, 14, 0, 140, 139, 1, 0, 0, 0, 140, 141, 1, 0, 0, 0, 141, 143,
		1, 0, 0, 0, 142, 144, 3, 10, 5, 0, 143, 142, 1, 0, 0, 0, 143, 144, 1, 0,
		0, 0, 144, 146, 1, 0, 0, 0, 145, 147, 3, 12, 6, 0, 146, 145, 1, 0, 0, 0,
		146, 147, 1, 0, 0, 0, 147, 149, 1, 0, 0, 0, 148, 150, 3, 14, 7, 0, 149,
		148, 1, 0, 0, 0, 149, 150, 1, 0, 0, 0, 150, 151, 1, 0, 0, 0, 151, 152,
		5, 4, 0, 0, 152, 31, 1, 0, 0, 0, 153, 154, 5, 1, 0, 0, 154, 156, 5, 20,
		0, 0, 155, 157, 5, 28, 0, 0, 156, 155, 1, 0, 0, 0, 156, 157, 1, 0, 0, 0,
		157, 159, 1, 0, 0, 0, 158, 160, 3, 30, 15, 0, 159, 158, 1, 0, 0, 0, 160,
		161, 1, 0, 0, 0, 161, 159, 1, 0, 0, 0, 161, 162, 1, 0, 0, 0, 162, 163,
		1, 0, 0, 0, 163, 164, 5, 4, 0, 0, 164, 33, 1, 0, 0, 0, 165, 166, 5, 1,
		0, 0, 166, 167, 5, 21, 0, 0, 167, 168, 3, 26, 13, 0, 168, 170, 3, 24, 12,
		0, 169, 171, 3, 28, 14, 0, 170, 169, 1, 0, 0, 0, 170, 171, 1, 0, 0, 0,
		171, 173, 1, 0, 0, 0, 172, 174, 3, 10, 5, 0, 173, 172, 1, 0, 0, 0, 173,
		174, 1, 0, 0, 0, 174, 176, 1, 0, 0, 0, 175, 177, 3, 12, 6, 0, 176, 175,
		1, 0, 0, 0, 176, 177, 1, 0, 0, 0, 177, 179, 1, 0, 0, 0, 178, 180, 3, 14,
		7, 0, 179, 178, 1, 0, 0, 0, 179, 180, 1, 0, 0, 0, 180, 181, 1, 0, 0, 0,
		181, 182, 5, 4, 0, 0, 182, 35, 1, 0, 0, 0, 183, 184, 5, 1, 0, 0, 184, 186,
		5, 22, 0, 0, 185, 187, 5, 28, 0, 0, 186, 185, 1, 0, 0, 0, 186, 187, 1,
		0, 0, 0, 187, 189, 1, 0, 0, 0, 188, 190, 3, 34, 17, 0, 189, 188, 1, 0,
		0, 0, 190, 191, 1, 0, 0, 0, 191, 189, 1, 0, 0, 0, 191, 192, 1, 0, 0, 0,
		192, 193, 1, 0, 0, 0, 193, 194, 5, 4, 0, 0, 194, 37, 1, 0, 0, 0, 195, 196,
		5, 1, 0, 0, 196, 197, 5, 23, 0, 0, 197, 198, 5, 27, 0, 0, 198, 199, 3,
		22, 11, 0, 199, 200, 5, 14, 0, 0, 200, 201, 3, 22, 11, 0, 201, 202, 3,
		32, 16, 0, 202, 203, 3, 36, 18, 0, 203, 204, 5, 4, 0, 0, 204, 39, 1, 0,
		0, 0, 18, 45, 48, 51, 54, 91, 120, 140, 143, 146, 149, 156, 161, 170, 173,
		176, 179, 186, 191,
	}
	deserializer := antlr.NewATNDeserializer(nil)
	staticData.atn = deserializer.Deserialize(staticData.serializedATN)
	atn := staticData.atn
	staticData.decisionToDFA = make([]*antlr.DFA, len(atn.DecisionToState))
	decisionToDFA := staticData.decisionToDFA
	for index, state := range atn.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(state, index)
	}
}

// TransactionParserInit initializes any static state used to implement TransactionParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewTransactionParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func TransactionParserInit() {
	staticData := &TransactionParserStaticData
	staticData.once.Do(transactionParserInit)
}

// NewTransactionParser produces a new parser instance for the optional input antlr.TokenStream.
func NewTransactionParser(input antlr.TokenStream) *TransactionParser {
	TransactionParserInit()
	this := new(TransactionParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &TransactionParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.PredictionContextCache)
	this.RuleNames = staticData.RuleNames
	this.LiteralNames = staticData.LiteralNames
	this.SymbolicNames = staticData.SymbolicNames
	this.GrammarFileName = "Transaction.g4"

	return this
}

// TransactionParser tokens.
const (
	TransactionParserEOF       = antlr.TokenEOF
	TransactionParserT__0      = 1
	TransactionParserT__1      = 2
	TransactionParserT__2      = 3
	TransactionParserT__3      = 4
	TransactionParserT__4      = 5
	TransactionParserT__5      = 6
	TransactionParserT__6      = 7
	TransactionParserT__7      = 8
	TransactionParserT__8      = 9
	TransactionParserT__9      = 10
	TransactionParserT__10     = 11
	TransactionParserT__11     = 12
	TransactionParserT__12     = 13
	TransactionParserT__13     = 14
	TransactionParserT__14     = 15
	TransactionParserT__15     = 16
	TransactionParserT__16     = 17
	TransactionParserT__17     = 18
	TransactionParserT__18     = 19
	TransactionParserT__19     = 20
	TransactionParserT__20     = 21
	TransactionParserT__21     = 22
	TransactionParserT__22     = 23
	TransactionParserVERSION   = 24
	TransactionParserINT       = 25
	TransactionParserSTRING    = 26
	TransactionParserUUID      = 27
	TransactionParserREMAINING = 28
	TransactionParserVARIABLE  = 29
	TransactionParserACCOUNT   = 30
	TransactionParserWS        = 31
)

// TransactionParser rules.
const (
	TransactionParserRULE_transaction              = 0
	TransactionParserRULE_chartOfAccountsGroupName = 1
	TransactionParserRULE_code                     = 2
	TransactionParserRULE_trueOrFalse              = 3
	TransactionParserRULE_pending                  = 4
	TransactionParserRULE_description              = 5
	TransactionParserRULE_chartOfAccounts          = 6
	TransactionParserRULE_metadata                 = 7
	TransactionParserRULE_pair                     = 8
	TransactionParserRULE_key                      = 9
	TransactionParserRULE_value                    = 10
	TransactionParserRULE_valueOrVariable          = 11
	TransactionParserRULE_sendTypes                = 12
	TransactionParserRULE_account                  = 13
	TransactionParserRULE_rate                     = 14
	TransactionParserRULE_from                     = 15
	TransactionParserRULE_source                   = 16
	TransactionParserRULE_to                       = 17
	TransactionParserRULE_distribute               = 18
	TransactionParserRULE_send                     = 19
)

// ITransactionContext is an interface to support dynamic dispatch.
type ITransactionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	VERSION() antlr.TerminalNode
	ChartOfAccountsGroupName() IChartOfAccountsGroupNameContext
	Send() ISendContext
	Description() IDescriptionContext
	Code() ICodeContext
	Pending() IPendingContext
	Metadata() IMetadataContext

	// IsTransactionContext differentiates from other interfaces.
	IsTransactionContext()
}

type TransactionContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTransactionContext() *TransactionContext {
	var p = new(TransactionContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_transaction
	return p
}

func InitEmptyTransactionContext(p *TransactionContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_transaction
}

func (*TransactionContext) IsTransactionContext() {}

func NewTransactionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TransactionContext {
	var p = new(TransactionContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_transaction

	return p
}

func (s *TransactionContext) GetParser() antlr.Parser { return s.parser }

func (s *TransactionContext) VERSION() antlr.TerminalNode {
	return s.GetToken(TransactionParserVERSION, 0)
}

func (s *TransactionContext) ChartOfAccountsGroupName() IChartOfAccountsGroupNameContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IChartOfAccountsGroupNameContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IChartOfAccountsGroupNameContext)
}

func (s *TransactionContext) Send() ISendContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISendContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISendContext)
}

func (s *TransactionContext) Description() IDescriptionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDescriptionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDescriptionContext)
}

func (s *TransactionContext) Code() ICodeContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ICodeContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ICodeContext)
}

func (s *TransactionContext) Pending() IPendingContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPendingContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPendingContext)
}

func (s *TransactionContext) Metadata() IMetadataContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMetadataContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMetadataContext)
}

func (s *TransactionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TransactionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TransactionContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterTransaction(s)
	}
}

func (s *TransactionContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitTransaction(s)
	}
}

func (s *TransactionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitTransaction(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Transaction() (localctx ITransactionContext) {
	localctx = NewTransactionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, TransactionParserRULE_transaction)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(40)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(41)
		_la = p.GetTokenStream().LA(1)

		if !(_la == TransactionParserT__1 || _la == TransactionParserT__2) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(42)
		p.Match(TransactionParserVERSION)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(43)
		p.ChartOfAccountsGroupName()
	}
	p.SetState(45)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 0, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(44)
			p.Description()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(48)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 1, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(47)
			p.Code()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(51)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 2, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(50)
			p.Pending()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(54)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 3, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(53)
			p.Metadata()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	{
		p.SetState(56)
		p.Send()
	}
	{
		p.SetState(57)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IChartOfAccountsGroupNameContext is an interface to support dynamic dispatch.
type IChartOfAccountsGroupNameContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	UUID() antlr.TerminalNode

	// IsChartOfAccountsGroupNameContext differentiates from other interfaces.
	IsChartOfAccountsGroupNameContext()
}

type ChartOfAccountsGroupNameContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyChartOfAccountsGroupNameContext() *ChartOfAccountsGroupNameContext {
	var p = new(ChartOfAccountsGroupNameContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_chartOfAccountsGroupName
	return p
}

func InitEmptyChartOfAccountsGroupNameContext(p *ChartOfAccountsGroupNameContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_chartOfAccountsGroupName
}

func (*ChartOfAccountsGroupNameContext) IsChartOfAccountsGroupNameContext() {}

func NewChartOfAccountsGroupNameContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ChartOfAccountsGroupNameContext {
	var p = new(ChartOfAccountsGroupNameContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_chartOfAccountsGroupName

	return p
}

func (s *ChartOfAccountsGroupNameContext) GetParser() antlr.Parser { return s.parser }

func (s *ChartOfAccountsGroupNameContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *ChartOfAccountsGroupNameContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ChartOfAccountsGroupNameContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ChartOfAccountsGroupNameContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterChartOfAccountsGroupName(s)
	}
}

func (s *ChartOfAccountsGroupNameContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitChartOfAccountsGroupName(s)
	}
}

func (s *ChartOfAccountsGroupNameContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitChartOfAccountsGroupName(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) ChartOfAccountsGroupName() (localctx IChartOfAccountsGroupNameContext) {
	localctx = NewChartOfAccountsGroupNameContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, TransactionParserRULE_chartOfAccountsGroupName)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(59)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(60)
		p.Match(TransactionParserT__4)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(61)
		p.Match(TransactionParserUUID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(62)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ICodeContext is an interface to support dynamic dispatch.
type ICodeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	UUID() antlr.TerminalNode

	// IsCodeContext differentiates from other interfaces.
	IsCodeContext()
}

type CodeContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyCodeContext() *CodeContext {
	var p = new(CodeContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_code
	return p
}

func InitEmptyCodeContext(p *CodeContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_code
}

func (*CodeContext) IsCodeContext() {}

func NewCodeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *CodeContext {
	var p = new(CodeContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_code

	return p
}

func (s *CodeContext) GetParser() antlr.Parser { return s.parser }

func (s *CodeContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *CodeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CodeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *CodeContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterCode(s)
	}
}

func (s *CodeContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitCode(s)
	}
}

func (s *CodeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitCode(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Code() (localctx ICodeContext) {
	localctx = NewCodeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, TransactionParserRULE_code)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(64)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(65)
		p.Match(TransactionParserT__5)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(66)
		p.Match(TransactionParserUUID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(67)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ITrueOrFalseContext is an interface to support dynamic dispatch.
type ITrueOrFalseContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsTrueOrFalseContext differentiates from other interfaces.
	IsTrueOrFalseContext()
}

type TrueOrFalseContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTrueOrFalseContext() *TrueOrFalseContext {
	var p = new(TrueOrFalseContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_trueOrFalse
	return p
}

func InitEmptyTrueOrFalseContext(p *TrueOrFalseContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_trueOrFalse
}

func (*TrueOrFalseContext) IsTrueOrFalseContext() {}

func NewTrueOrFalseContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TrueOrFalseContext {
	var p = new(TrueOrFalseContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_trueOrFalse

	return p
}

func (s *TrueOrFalseContext) GetParser() antlr.Parser { return s.parser }
func (s *TrueOrFalseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TrueOrFalseContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TrueOrFalseContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterTrueOrFalse(s)
	}
}

func (s *TrueOrFalseContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitTrueOrFalse(s)
	}
}

func (s *TrueOrFalseContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitTrueOrFalse(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) TrueOrFalse() (localctx ITrueOrFalseContext) {
	localctx = NewTrueOrFalseContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, TransactionParserRULE_trueOrFalse)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(69)
		_la = p.GetTokenStream().LA(1)

		if !(_la == TransactionParserT__6 || _la == TransactionParserT__7) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IPendingContext is an interface to support dynamic dispatch.
type IPendingContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	TrueOrFalse() ITrueOrFalseContext

	// IsPendingContext differentiates from other interfaces.
	IsPendingContext()
}

type PendingContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyPendingContext() *PendingContext {
	var p = new(PendingContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_pending
	return p
}

func InitEmptyPendingContext(p *PendingContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_pending
}

func (*PendingContext) IsPendingContext() {}

func NewPendingContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *PendingContext {
	var p = new(PendingContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_pending

	return p
}

func (s *PendingContext) GetParser() antlr.Parser { return s.parser }

func (s *PendingContext) TrueOrFalse() ITrueOrFalseContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITrueOrFalseContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITrueOrFalseContext)
}

func (s *PendingContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PendingContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *PendingContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterPending(s)
	}
}

func (s *PendingContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitPending(s)
	}
}

func (s *PendingContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitPending(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Pending() (localctx IPendingContext) {
	localctx = NewPendingContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, TransactionParserRULE_pending)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(71)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(72)
		p.Match(TransactionParserT__8)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(73)
		p.TrueOrFalse()
	}
	{
		p.SetState(74)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IDescriptionContext is an interface to support dynamic dispatch.
type IDescriptionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	STRING() antlr.TerminalNode

	// IsDescriptionContext differentiates from other interfaces.
	IsDescriptionContext()
}

type DescriptionContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDescriptionContext() *DescriptionContext {
	var p = new(DescriptionContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_description
	return p
}

func InitEmptyDescriptionContext(p *DescriptionContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_description
}

func (*DescriptionContext) IsDescriptionContext() {}

func NewDescriptionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DescriptionContext {
	var p = new(DescriptionContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_description

	return p
}

func (s *DescriptionContext) GetParser() antlr.Parser { return s.parser }

func (s *DescriptionContext) STRING() antlr.TerminalNode {
	return s.GetToken(TransactionParserSTRING, 0)
}

func (s *DescriptionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DescriptionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DescriptionContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterDescription(s)
	}
}

func (s *DescriptionContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitDescription(s)
	}
}

func (s *DescriptionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitDescription(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Description() (localctx IDescriptionContext) {
	localctx = NewDescriptionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, TransactionParserRULE_description)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(76)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(77)
		p.Match(TransactionParserT__9)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(78)
		p.Match(TransactionParserSTRING)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(79)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IChartOfAccountsContext is an interface to support dynamic dispatch.
type IChartOfAccountsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	UUID() antlr.TerminalNode

	// IsChartOfAccountsContext differentiates from other interfaces.
	IsChartOfAccountsContext()
}

type ChartOfAccountsContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyChartOfAccountsContext() *ChartOfAccountsContext {
	var p = new(ChartOfAccountsContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_chartOfAccounts
	return p
}

func InitEmptyChartOfAccountsContext(p *ChartOfAccountsContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_chartOfAccounts
}

func (*ChartOfAccountsContext) IsChartOfAccountsContext() {}

func NewChartOfAccountsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ChartOfAccountsContext {
	var p = new(ChartOfAccountsContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_chartOfAccounts

	return p
}

func (s *ChartOfAccountsContext) GetParser() antlr.Parser { return s.parser }

func (s *ChartOfAccountsContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *ChartOfAccountsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ChartOfAccountsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ChartOfAccountsContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterChartOfAccounts(s)
	}
}

func (s *ChartOfAccountsContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitChartOfAccounts(s)
	}
}

func (s *ChartOfAccountsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitChartOfAccounts(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) ChartOfAccounts() (localctx IChartOfAccountsContext) {
	localctx = NewChartOfAccountsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, TransactionParserRULE_chartOfAccounts)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(81)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(82)
		p.Match(TransactionParserT__10)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(83)
		p.Match(TransactionParserUUID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(84)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IMetadataContext is an interface to support dynamic dispatch.
type IMetadataContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AllPair() []IPairContext
	Pair(i int) IPairContext

	// IsMetadataContext differentiates from other interfaces.
	IsMetadataContext()
}

type MetadataContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyMetadataContext() *MetadataContext {
	var p = new(MetadataContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_metadata
	return p
}

func InitEmptyMetadataContext(p *MetadataContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_metadata
}

func (*MetadataContext) IsMetadataContext() {}

func NewMetadataContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *MetadataContext {
	var p = new(MetadataContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_metadata

	return p
}

func (s *MetadataContext) GetParser() antlr.Parser { return s.parser }

func (s *MetadataContext) AllPair() []IPairContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IPairContext); ok {
			len++
		}
	}

	tst := make([]IPairContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IPairContext); ok {
			tst[i] = t.(IPairContext)
			i++
		}
	}

	return tst
}

func (s *MetadataContext) Pair(i int) IPairContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPairContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPairContext)
}

func (s *MetadataContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MetadataContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *MetadataContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterMetadata(s)
	}
}

func (s *MetadataContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitMetadata(s)
	}
}

func (s *MetadataContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitMetadata(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Metadata() (localctx IMetadataContext) {
	localctx = NewMetadataContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, TransactionParserRULE_metadata)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(86)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(87)
		p.Match(TransactionParserT__11)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(89)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == TransactionParserT__0 {
		{
			p.SetState(88)
			p.Pair()
		}

		p.SetState(91)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(93)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IPairContext is an interface to support dynamic dispatch.
type IPairContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Key() IKeyContext
	Value() IValueContext

	// IsPairContext differentiates from other interfaces.
	IsPairContext()
}

type PairContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyPairContext() *PairContext {
	var p = new(PairContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_pair
	return p
}

func InitEmptyPairContext(p *PairContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_pair
}

func (*PairContext) IsPairContext() {}

func NewPairContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *PairContext {
	var p = new(PairContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_pair

	return p
}

func (s *PairContext) GetParser() antlr.Parser { return s.parser }

func (s *PairContext) Key() IKeyContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IKeyContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IKeyContext)
}

func (s *PairContext) Value() IValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueContext)
}

func (s *PairContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PairContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *PairContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterPair(s)
	}
}

func (s *PairContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitPair(s)
	}
}

func (s *PairContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitPair(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Pair() (localctx IPairContext) {
	localctx = NewPairContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, TransactionParserRULE_pair)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(95)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(96)
		p.Key()
	}
	{
		p.SetState(97)
		p.Value()
	}
	{
		p.SetState(98)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IKeyContext is an interface to support dynamic dispatch.
type IKeyContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	UUID() antlr.TerminalNode
	INT() antlr.TerminalNode

	// IsKeyContext differentiates from other interfaces.
	IsKeyContext()
}

type KeyContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyKeyContext() *KeyContext {
	var p = new(KeyContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_key
	return p
}

func InitEmptyKeyContext(p *KeyContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_key
}

func (*KeyContext) IsKeyContext() {}

func NewKeyContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *KeyContext {
	var p = new(KeyContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_key

	return p
}

func (s *KeyContext) GetParser() antlr.Parser { return s.parser }

func (s *KeyContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *KeyContext) INT() antlr.TerminalNode {
	return s.GetToken(TransactionParserINT, 0)
}

func (s *KeyContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *KeyContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *KeyContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterKey(s)
	}
}

func (s *KeyContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitKey(s)
	}
}

func (s *KeyContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitKey(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Key() (localctx IKeyContext) {
	localctx = NewKeyContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, TransactionParserRULE_key)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(100)
		_la = p.GetTokenStream().LA(1)

		if !(_la == TransactionParserINT || _la == TransactionParserUUID) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IValueContext is an interface to support dynamic dispatch.
type IValueContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	UUID() antlr.TerminalNode
	INT() antlr.TerminalNode

	// IsValueContext differentiates from other interfaces.
	IsValueContext()
}

type ValueContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyValueContext() *ValueContext {
	var p = new(ValueContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_value
	return p
}

func InitEmptyValueContext(p *ValueContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_value
}

func (*ValueContext) IsValueContext() {}

func NewValueContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ValueContext {
	var p = new(ValueContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_value

	return p
}

func (s *ValueContext) GetParser() antlr.Parser { return s.parser }

func (s *ValueContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *ValueContext) INT() antlr.TerminalNode {
	return s.GetToken(TransactionParserINT, 0)
}

func (s *ValueContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ValueContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ValueContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterValue(s)
	}
}

func (s *ValueContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitValue(s)
	}
}

func (s *ValueContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Value() (localctx IValueContext) {
	localctx = NewValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, TransactionParserRULE_value)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(102)
		_la = p.GetTokenStream().LA(1)

		if !(_la == TransactionParserINT || _la == TransactionParserUUID) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IValueOrVariableContext is an interface to support dynamic dispatch.
type IValueOrVariableContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	INT() antlr.TerminalNode
	VARIABLE() antlr.TerminalNode

	// IsValueOrVariableContext differentiates from other interfaces.
	IsValueOrVariableContext()
}

type ValueOrVariableContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyValueOrVariableContext() *ValueOrVariableContext {
	var p = new(ValueOrVariableContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_valueOrVariable
	return p
}

func InitEmptyValueOrVariableContext(p *ValueOrVariableContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_valueOrVariable
}

func (*ValueOrVariableContext) IsValueOrVariableContext() {}

func NewValueOrVariableContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ValueOrVariableContext {
	var p = new(ValueOrVariableContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_valueOrVariable

	return p
}

func (s *ValueOrVariableContext) GetParser() antlr.Parser { return s.parser }

func (s *ValueOrVariableContext) INT() antlr.TerminalNode {
	return s.GetToken(TransactionParserINT, 0)
}

func (s *ValueOrVariableContext) VARIABLE() antlr.TerminalNode {
	return s.GetToken(TransactionParserVARIABLE, 0)
}

func (s *ValueOrVariableContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ValueOrVariableContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ValueOrVariableContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterValueOrVariable(s)
	}
}

func (s *ValueOrVariableContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitValueOrVariable(s)
	}
}

func (s *ValueOrVariableContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitValueOrVariable(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) ValueOrVariable() (localctx IValueOrVariableContext) {
	localctx = NewValueOrVariableContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, TransactionParserRULE_valueOrVariable)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(104)
		_la = p.GetTokenStream().LA(1)

		if !(_la == TransactionParserINT || _la == TransactionParserVARIABLE) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISendTypesContext is an interface to support dynamic dispatch.
type ISendTypesContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsSendTypesContext differentiates from other interfaces.
	IsSendTypesContext()
}

type SendTypesContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySendTypesContext() *SendTypesContext {
	var p = new(SendTypesContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_sendTypes
	return p
}

func InitEmptySendTypesContext(p *SendTypesContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_sendTypes
}

func (*SendTypesContext) IsSendTypesContext() {}

func NewSendTypesContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SendTypesContext {
	var p = new(SendTypesContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_sendTypes

	return p
}

func (s *SendTypesContext) GetParser() antlr.Parser { return s.parser }

func (s *SendTypesContext) CopyAll(ctx *SendTypesContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *SendTypesContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SendTypesContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type ShareIntContext struct {
	SendTypesContext
}

func NewShareIntContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ShareIntContext {
	var p = new(ShareIntContext)

	InitEmptySendTypesContext(&p.SendTypesContext)
	p.parser = parser
	p.CopyAll(ctx.(*SendTypesContext))

	return p
}

func (s *ShareIntContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ShareIntContext) ValueOrVariable() IValueOrVariableContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueOrVariableContext)
}

func (s *ShareIntContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterShareInt(s)
	}
}

func (s *ShareIntContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitShareInt(s)
	}
}

func (s *ShareIntContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitShareInt(s)

	default:
		return t.VisitChildren(s)
	}
}

type AmountContext struct {
	SendTypesContext
}

func NewAmountContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AmountContext {
	var p = new(AmountContext)

	InitEmptySendTypesContext(&p.SendTypesContext)
	p.parser = parser
	p.CopyAll(ctx.(*SendTypesContext))

	return p
}

func (s *AmountContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AmountContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *AmountContext) AllValueOrVariable() []IValueOrVariableContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			len++
		}
	}

	tst := make([]IValueOrVariableContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IValueOrVariableContext); ok {
			tst[i] = t.(IValueOrVariableContext)
			i++
		}
	}

	return tst
}

func (s *AmountContext) ValueOrVariable(i int) IValueOrVariableContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueOrVariableContext)
}

func (s *AmountContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterAmount(s)
	}
}

func (s *AmountContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitAmount(s)
	}
}

func (s *AmountContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitAmount(s)

	default:
		return t.VisitChildren(s)
	}
}

type ShareIntOfIntContext struct {
	SendTypesContext
}

func NewShareIntOfIntContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ShareIntOfIntContext {
	var p = new(ShareIntOfIntContext)

	InitEmptySendTypesContext(&p.SendTypesContext)
	p.parser = parser
	p.CopyAll(ctx.(*SendTypesContext))

	return p
}

func (s *ShareIntOfIntContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ShareIntOfIntContext) AllValueOrVariable() []IValueOrVariableContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			len++
		}
	}

	tst := make([]IValueOrVariableContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IValueOrVariableContext); ok {
			tst[i] = t.(IValueOrVariableContext)
			i++
		}
	}

	return tst
}

func (s *ShareIntOfIntContext) ValueOrVariable(i int) IValueOrVariableContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueOrVariableContext)
}

func (s *ShareIntOfIntContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterShareIntOfInt(s)
	}
}

func (s *ShareIntOfIntContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitShareIntOfInt(s)
	}
}

func (s *ShareIntOfIntContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitShareIntOfInt(s)

	default:
		return t.VisitChildren(s)
	}
}

type RemainingContext struct {
	SendTypesContext
}

func NewRemainingContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *RemainingContext {
	var p = new(RemainingContext)

	InitEmptySendTypesContext(&p.SendTypesContext)
	p.parser = parser
	p.CopyAll(ctx.(*SendTypesContext))

	return p
}

func (s *RemainingContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RemainingContext) REMAINING() antlr.TerminalNode {
	return s.GetToken(TransactionParserREMAINING, 0)
}

func (s *RemainingContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterRemaining(s)
	}
}

func (s *RemainingContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitRemaining(s)
	}
}

func (s *RemainingContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitRemaining(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) SendTypes() (localctx ISendTypesContext) {
	localctx = NewSendTypesContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, TransactionParserRULE_sendTypes)
	p.SetState(120)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 5, p.GetParserRuleContext()) {
	case 1:
		localctx = NewAmountContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(106)
			p.Match(TransactionParserT__12)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(107)
			p.Match(TransactionParserUUID)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(108)
			p.ValueOrVariable()
		}
		{
			p.SetState(109)
			p.Match(TransactionParserT__13)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(110)
			p.ValueOrVariable()
		}

	case 2:
		localctx = NewShareIntOfIntContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(112)
			p.Match(TransactionParserT__14)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(113)
			p.ValueOrVariable()
		}
		{
			p.SetState(114)
			p.Match(TransactionParserT__15)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(115)
			p.ValueOrVariable()
		}

	case 3:
		localctx = NewShareIntContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(117)
			p.Match(TransactionParserT__14)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(118)
			p.ValueOrVariable()
		}

	case 4:
		localctx = NewRemainingContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(119)
			p.Match(TransactionParserREMAINING)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case antlr.ATNInvalidAltNumber:
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAccountContext is an interface to support dynamic dispatch.
type IAccountContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	VARIABLE() antlr.TerminalNode
	ACCOUNT() antlr.TerminalNode
	UUID() antlr.TerminalNode

	// IsAccountContext differentiates from other interfaces.
	IsAccountContext()
}

type AccountContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAccountContext() *AccountContext {
	var p = new(AccountContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_account
	return p
}

func InitEmptyAccountContext(p *AccountContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_account
}

func (*AccountContext) IsAccountContext() {}

func NewAccountContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AccountContext {
	var p = new(AccountContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_account

	return p
}

func (s *AccountContext) GetParser() antlr.Parser { return s.parser }

func (s *AccountContext) VARIABLE() antlr.TerminalNode {
	return s.GetToken(TransactionParserVARIABLE, 0)
}

func (s *AccountContext) ACCOUNT() antlr.TerminalNode {
	return s.GetToken(TransactionParserACCOUNT, 0)
}

func (s *AccountContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *AccountContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AccountContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AccountContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterAccount(s)
	}
}

func (s *AccountContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitAccount(s)
	}
}

func (s *AccountContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitAccount(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Account() (localctx IAccountContext) {
	localctx = NewAccountContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, TransactionParserRULE_account)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(122)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&1744830464) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IRateContext is an interface to support dynamic dispatch.
type IRateContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AllUUID() []antlr.TerminalNode
	UUID(i int) antlr.TerminalNode
	AllValueOrVariable() []IValueOrVariableContext
	ValueOrVariable(i int) IValueOrVariableContext

	// IsRateContext differentiates from other interfaces.
	IsRateContext()
}

type RateContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRateContext() *RateContext {
	var p = new(RateContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_rate
	return p
}

func InitEmptyRateContext(p *RateContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_rate
}

func (*RateContext) IsRateContext() {}

func NewRateContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RateContext {
	var p = new(RateContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_rate

	return p
}

func (s *RateContext) GetParser() antlr.Parser { return s.parser }

func (s *RateContext) AllUUID() []antlr.TerminalNode {
	return s.GetTokens(TransactionParserUUID)
}

func (s *RateContext) UUID(i int) antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, i)
}

func (s *RateContext) AllValueOrVariable() []IValueOrVariableContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			len++
		}
	}

	tst := make([]IValueOrVariableContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IValueOrVariableContext); ok {
			tst[i] = t.(IValueOrVariableContext)
			i++
		}
	}

	return tst
}

func (s *RateContext) ValueOrVariable(i int) IValueOrVariableContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueOrVariableContext)
}

func (s *RateContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RateContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RateContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterRate(s)
	}
}

func (s *RateContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitRate(s)
	}
}

func (s *RateContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitRate(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Rate() (localctx IRateContext) {
	localctx = NewRateContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, TransactionParserRULE_rate)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(124)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(125)
		p.Match(TransactionParserT__16)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(126)
		p.Match(TransactionParserUUID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(127)
		p.Match(TransactionParserUUID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(128)
		p.Match(TransactionParserT__17)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(129)
		p.Match(TransactionParserUUID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(130)
		p.ValueOrVariable()
	}
	{
		p.SetState(131)
		p.Match(TransactionParserT__13)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(132)
		p.ValueOrVariable()
	}
	{
		p.SetState(133)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IFromContext is an interface to support dynamic dispatch.
type IFromContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Account() IAccountContext
	SendTypes() ISendTypesContext
	Rate() IRateContext
	Description() IDescriptionContext
	ChartOfAccounts() IChartOfAccountsContext
	Metadata() IMetadataContext

	// IsFromContext differentiates from other interfaces.
	IsFromContext()
}

type FromContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFromContext() *FromContext {
	var p = new(FromContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_from
	return p
}

func InitEmptyFromContext(p *FromContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_from
}

func (*FromContext) IsFromContext() {}

func NewFromContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FromContext {
	var p = new(FromContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_from

	return p
}

func (s *FromContext) GetParser() antlr.Parser { return s.parser }

func (s *FromContext) Account() IAccountContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAccountContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAccountContext)
}

func (s *FromContext) SendTypes() ISendTypesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISendTypesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISendTypesContext)
}

func (s *FromContext) Rate() IRateContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRateContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRateContext)
}

func (s *FromContext) Description() IDescriptionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDescriptionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDescriptionContext)
}

func (s *FromContext) ChartOfAccounts() IChartOfAccountsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IChartOfAccountsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IChartOfAccountsContext)
}

func (s *FromContext) Metadata() IMetadataContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMetadataContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMetadataContext)
}

func (s *FromContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FromContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FromContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterFrom(s)
	}
}

func (s *FromContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitFrom(s)
	}
}

func (s *FromContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitFrom(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) From() (localctx IFromContext) {
	localctx = NewFromContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, TransactionParserRULE_from)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(135)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(136)
		p.Match(TransactionParserT__18)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(137)
		p.Account()
	}
	{
		p.SetState(138)
		p.SendTypes()
	}
	p.SetState(140)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 6, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(139)
			p.Rate()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(143)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 7, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(142)
			p.Description()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(146)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 8, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(145)
			p.ChartOfAccounts()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(149)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == TransactionParserT__0 {
		{
			p.SetState(148)
			p.Metadata()
		}

	}
	{
		p.SetState(151)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISourceContext is an interface to support dynamic dispatch.
type ISourceContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	REMAINING() antlr.TerminalNode
	AllFrom() []IFromContext
	From(i int) IFromContext

	// IsSourceContext differentiates from other interfaces.
	IsSourceContext()
}

type SourceContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySourceContext() *SourceContext {
	var p = new(SourceContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_source
	return p
}

func InitEmptySourceContext(p *SourceContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_source
}

func (*SourceContext) IsSourceContext() {}

func NewSourceContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SourceContext {
	var p = new(SourceContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_source

	return p
}

func (s *SourceContext) GetParser() antlr.Parser { return s.parser }

func (s *SourceContext) REMAINING() antlr.TerminalNode {
	return s.GetToken(TransactionParserREMAINING, 0)
}

func (s *SourceContext) AllFrom() []IFromContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFromContext); ok {
			len++
		}
	}

	tst := make([]IFromContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFromContext); ok {
			tst[i] = t.(IFromContext)
			i++
		}
	}

	return tst
}

func (s *SourceContext) From(i int) IFromContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFromContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFromContext)
}

func (s *SourceContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SourceContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SourceContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterSource(s)
	}
}

func (s *SourceContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitSource(s)
	}
}

func (s *SourceContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitSource(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Source() (localctx ISourceContext) {
	localctx = NewSourceContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, TransactionParserRULE_source)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(153)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(154)
		p.Match(TransactionParserT__19)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(156)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == TransactionParserREMAINING {
		{
			p.SetState(155)
			p.Match(TransactionParserREMAINING)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	}
	p.SetState(159)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == TransactionParserT__0 {
		{
			p.SetState(158)
			p.From()
		}

		p.SetState(161)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(163)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IToContext is an interface to support dynamic dispatch.
type IToContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Account() IAccountContext
	SendTypes() ISendTypesContext
	Rate() IRateContext
	Description() IDescriptionContext
	ChartOfAccounts() IChartOfAccountsContext
	Metadata() IMetadataContext

	// IsToContext differentiates from other interfaces.
	IsToContext()
}

type ToContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyToContext() *ToContext {
	var p = new(ToContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_to
	return p
}

func InitEmptyToContext(p *ToContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_to
}

func (*ToContext) IsToContext() {}

func NewToContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ToContext {
	var p = new(ToContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_to

	return p
}

func (s *ToContext) GetParser() antlr.Parser { return s.parser }

func (s *ToContext) Account() IAccountContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAccountContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAccountContext)
}

func (s *ToContext) SendTypes() ISendTypesContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISendTypesContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISendTypesContext)
}

func (s *ToContext) Rate() IRateContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRateContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRateContext)
}

func (s *ToContext) Description() IDescriptionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDescriptionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDescriptionContext)
}

func (s *ToContext) ChartOfAccounts() IChartOfAccountsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IChartOfAccountsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IChartOfAccountsContext)
}

func (s *ToContext) Metadata() IMetadataContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMetadataContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMetadataContext)
}

func (s *ToContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ToContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ToContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterTo(s)
	}
}

func (s *ToContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitTo(s)
	}
}

func (s *ToContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitTo(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) To() (localctx IToContext) {
	localctx = NewToContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, TransactionParserRULE_to)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(165)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(166)
		p.Match(TransactionParserT__20)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(167)
		p.Account()
	}
	{
		p.SetState(168)
		p.SendTypes()
	}
	p.SetState(170)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 12, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(169)
			p.Rate()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(173)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 13, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(172)
			p.Description()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(176)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 14, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(175)
			p.ChartOfAccounts()
		}

	} else if p.HasError() { // JIM
		goto errorExit
	}
	p.SetState(179)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == TransactionParserT__0 {
		{
			p.SetState(178)
			p.Metadata()
		}

	}
	{
		p.SetState(181)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IDistributeContext is an interface to support dynamic dispatch.
type IDistributeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	REMAINING() antlr.TerminalNode
	AllTo() []IToContext
	To(i int) IToContext

	// IsDistributeContext differentiates from other interfaces.
	IsDistributeContext()
}

type DistributeContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDistributeContext() *DistributeContext {
	var p = new(DistributeContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_distribute
	return p
}

func InitEmptyDistributeContext(p *DistributeContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_distribute
}

func (*DistributeContext) IsDistributeContext() {}

func NewDistributeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *DistributeContext {
	var p = new(DistributeContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_distribute

	return p
}

func (s *DistributeContext) GetParser() antlr.Parser { return s.parser }

func (s *DistributeContext) REMAINING() antlr.TerminalNode {
	return s.GetToken(TransactionParserREMAINING, 0)
}

func (s *DistributeContext) AllTo() []IToContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IToContext); ok {
			len++
		}
	}

	tst := make([]IToContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IToContext); ok {
			tst[i] = t.(IToContext)
			i++
		}
	}

	return tst
}

func (s *DistributeContext) To(i int) IToContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IToContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IToContext)
}

func (s *DistributeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *DistributeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *DistributeContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterDistribute(s)
	}
}

func (s *DistributeContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitDistribute(s)
	}
}

func (s *DistributeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitDistribute(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Distribute() (localctx IDistributeContext) {
	localctx = NewDistributeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, TransactionParserRULE_distribute)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(183)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(184)
		p.Match(TransactionParserT__21)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(186)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == TransactionParserREMAINING {
		{
			p.SetState(185)
			p.Match(TransactionParserREMAINING)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	}
	p.SetState(189)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == TransactionParserT__0 {
		{
			p.SetState(188)
			p.To()
		}

		p.SetState(191)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(193)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISendContext is an interface to support dynamic dispatch.
type ISendContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	UUID() antlr.TerminalNode
	AllValueOrVariable() []IValueOrVariableContext
	ValueOrVariable(i int) IValueOrVariableContext
	Source() ISourceContext
	Distribute() IDistributeContext

	// IsSendContext differentiates from other interfaces.
	IsSendContext()
}

type SendContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySendContext() *SendContext {
	var p = new(SendContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_send
	return p
}

func InitEmptySendContext(p *SendContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = TransactionParserRULE_send
}

func (*SendContext) IsSendContext() {}

func NewSendContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SendContext {
	var p = new(SendContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = TransactionParserRULE_send

	return p
}

func (s *SendContext) GetParser() antlr.Parser { return s.parser }

func (s *SendContext) UUID() antlr.TerminalNode {
	return s.GetToken(TransactionParserUUID, 0)
}

func (s *SendContext) AllValueOrVariable() []IValueOrVariableContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			len++
		}
	}

	tst := make([]IValueOrVariableContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IValueOrVariableContext); ok {
			tst[i] = t.(IValueOrVariableContext)
			i++
		}
	}

	return tst
}

func (s *SendContext) ValueOrVariable(i int) IValueOrVariableContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueOrVariableContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueOrVariableContext)
}

func (s *SendContext) Source() ISourceContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISourceContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISourceContext)
}

func (s *SendContext) Distribute() IDistributeContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDistributeContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDistributeContext)
}

func (s *SendContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SendContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SendContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.EnterSend(s)
	}
}

func (s *SendContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(TransactionListener); ok {
		listenerT.ExitSend(s)
	}
}

func (s *SendContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case TransactionVisitor:
		return t.VisitSend(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *TransactionParser) Send() (localctx ISendContext) {
	localctx = NewSendContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, TransactionParserRULE_send)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(195)
		p.Match(TransactionParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(196)
		p.Match(TransactionParserT__22)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(197)
		p.Match(TransactionParserUUID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(198)
		p.ValueOrVariable()
	}
	{
		p.SetState(199)
		p.Match(TransactionParserT__13)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(200)
		p.ValueOrVariable()
	}
	{
		p.SetState(201)
		p.Source()
	}
	{
		p.SetState(202)
		p.Distribute()
	}
	{
		p.SetState(203)
		p.Match(TransactionParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}
