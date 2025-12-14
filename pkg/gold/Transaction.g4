grammar Transaction;

// Tokens
VERSION: 'V1';
INT: [0-9]+;
STRING: '"' .*? '"';
UUID: [a-zA-Z0-9_\-/]+;
REMAINING: ':remaining';
VARIABLE: '$'[a-zA-Z0-9_\-]*;
ACCOUNT: '@'[a-zA-Z0-9_\-/]*;
WS: [ \t\r\n]+ -> skip;


// Rules
transaction: '(' ('transaction' | 'transaction-template') VERSION chartOfAccountsGroupName description? code? pending? metadata? send ')';

chartOfAccountsGroupName: '(' 'chart-of-accounts-group-name' UUID ')';
code: '(' 'code' UUID ')';

trueOrFalse: 'false'
           | 'true'
           ;

pending: '(' 'pending' trueOrFalse ')';

description: '(' 'description' STRING ')';
chartOfAccounts: '(' 'chart-of-accounts' UUID ')';
metadata: '(' 'metadata' pair+ ')';
pair: '(' key value ')';

key: UUID
   | INT
   ;

value: UUID
     | INT
     ;

// NOTE: Variables ($var) are NOT supported in numeric positions (amounts, shares, rates, scales).
// Variables ARE supported for account references (see 'account' rule below).
// For template variable support, resolve variables to concrete values before parsing.
numericValue: INT;

sendTypes: ':amount' UUID numericValue '|' numericValue               # Amount
         | ':share' numericValue ':of' numericValue                    # ShareIntOfInt
         | ':share' numericValue                                          # ShareInt
         | REMAINING                                                         # Remaining
         ;

account: VARIABLE
       | ACCOUNT
       | UUID
       ;


rate: '(' 'rate' UUID UUID '->' UUID numericValue '|' numericValue ')';

from: '(' 'from' account sendTypes rate? description? chartOfAccounts? metadata? ')';
source: '(' 'source' REMAINING? from+ ')';

to: '(' 'to' account sendTypes rate? description? chartOfAccounts? metadata? ')';
distribute: '(' 'distribute' REMAINING? to+ ')';

send: '(' 'send' UUID numericValue '|' numericValue source distribute ')';
