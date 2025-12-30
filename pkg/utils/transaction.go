package utils

import (
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
)

// SanitizeAccountAliases cleans the AccountAlias fields in a Transaction's FromTo entries.
// This is necessary because HandleAccountFields mutates aliases in-place using ConcatAlias,
// producing formats like "0#@alias#key". SplitAlias extracts the original alias back.
func SanitizeAccountAliases(parserDSL *libTransaction.Transaction) {
	if parserDSL == nil {
		return
	}

	for i := range parserDSL.Send.Source.From {
		parserDSL.Send.Source.From[i].AccountAlias = parserDSL.Send.Source.From[i].SplitAlias()
	}

	for i := range parserDSL.Send.Distribute.To {
		parserDSL.Send.Distribute.To[i].AccountAlias = parserDSL.Send.Distribute.To[i].SplitAlias()
	}
}
