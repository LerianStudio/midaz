// Gold DSL Grammar for Midaz Ledger System
//
// This ANTLR4 grammar defines the syntax for the Gold Domain-Specific Language (DSL),
// a language for defining complex financial transactions. The grammar specifies the
// tokens (lexical elements) and parsing rules (syntactic structure) of the DSL.
//
// The grammar is used by ANTLR to generate a parser and lexer in Go, which are
// then used to parse and validate Gold DSL scripts.
grammar Transaction;

// Token Definitions
//
// This section defines the basic building blocks (tokens) of the Gold DSL.
// Each token is defined by a regular expression.
VERSION: 'V1';
INT: [0-9]+;
STRING: '"' .*? '"';
UUID: [a-zA-Z0-9_\\-/]+; // Allows alphanumeric chars, underscore, hyphen, and slash
REMAINING: ':remaining';
VARIABLE: '$'[a-zA-Z0-9_\\-]*;
ACCOUNT: '@'[a-zA-Z0-9_\\-/]*;
WS: [ \\t\\r\\n]+ -> skip; // Skips whitespace characters

// Parser Rules
//
// This section defines the syntactic structure of the Gold DSL. Each rule specifies
// a valid combination of tokens and other rules.

// transaction is the root rule, defining the overall structure of a transaction script.
// It can be a 'transaction' or 'transaction-template' and includes metadata and a 'send' block.
transaction: '(' ('transaction' | 'transaction-template') VERSION chartOfAccountsGroupName description? code? pending? metadata? send ')';

// chartOfAccountsGroupName specifies the chart of accounts group for the transaction.
chartOfAccountsGroupName: '(' 'chart-of-accounts-group-name' UUID ')';

// code specifies an optional transaction code (UUID).
code: '(' 'code' UUID ')';

// trueOrFalse defines boolean values.
trueOrFalse: 'false'
           | 'true'
           ;

// pending specifies if the transaction should be in a pending state.
pending: '(' 'pending' trueOrFalse ')';

// description provides a human-readable description for the transaction.
description: '(' 'description' STRING ')';

// chartOfAccounts links the transaction to a specific chart of accounts.
chartOfAccounts: '(' 'chart-of-accounts' UUID ')';

// metadata allows attaching custom key-value pairs to the transaction.
metadata: '(' 'metadata' pair+ ')';
pair: '(' key value ')';

// key defines the key for a metadata pair.
key: UUID
   | INT
   ;

// value defines the value for a metadata pair.
value: UUID
     | INT
     ;

// valueOrVariable allows a value to be either an integer or a variable.
valueOrVariable: INT
               | VARIABLE
               ;

// sendTypes defines the different ways to specify amounts in 'from' and 'to' blocks:
// - :amount: A fixed amount of a specific asset.
// - :share: A percentage of the total amount.
// - :share of: A percentage of a percentage.
// - :remaining: The remaining amount after other distributions.
sendTypes: ':amount' UUID valueOrVariable '|' valueOrVariable               # Amount
         | ':share' valueOrVariable ':of' valueOrVariable                    # ShareIntOfInt
         | ':share' valueOrVariable                                          # ShareInt
         | REMAINING                                                         # Remaining
         ;

// account represents an account, which can be a variable, an alias, or a UUID.
account: VARIABLE
       | ACCOUNT
       | UUID
       ;


// rate defines a currency conversion rate.
rate: '(' 'rate' UUID UUID '->' UUID valueOrVariable '|' valueOrVariable ')';

// from specifies a source account and the amount to be taken from it.
from: '(' 'from' account sendTypes rate? description? chartOfAccounts? metadata? ')';
source: '(' 'source' REMAINING? from+ ')';

// to specifies a destination account and the amount to be sent to it.
to: '(' 'to' account sendTypes rate? description? chartOfAccounts? metadata? ')';
distribute: '(' 'distribute' REMAINING? to+ ')';

// send is the core of the transaction, defining the asset, total amount, source, and distribution.
send: '(' 'send' UUID valueOrVariable '|' valueOrVariable source distribute ')';
