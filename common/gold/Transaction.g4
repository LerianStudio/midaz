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
transaction: '(' ('transaction' | 'transaction-template') VERSION chartOfAccountsGroupName description? code? pending? metadata? send distribute ')';

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

valueOrVariable: INT
               | VARIABLE
               ;

sendTypes: ':amount' UUID valueOrVariable '|' valueOrVariable                # Amount
         | ':share' valueOrVariable ':of' valueOrVariable ':desc whatever'   # ShareDescWhatever
         | ':share' valueOrVariable ':of' valueOrVariable                    # ShareIntOfInt
         | ':share' valueOrVariable                                          # ShareInt
         | REMAINING                                                         # Remaining
         ;

account: VARIABLE
       | ACCOUNT
       | UUID
       ;

from: '(' 'from' account sendTypes description? chartOfAccounts? metadata? ')';
send: '(' 'send' UUID valueOrVariable '|' valueOrVariable source ')';
source: '(' 'source' REMAINING? from+ source? ')';

to: '(' 'to' account sendTypes description? chartOfAccounts? metadata? ')';
distribute: '(' 'distribute' REMAINING? to+ distribute? ')';