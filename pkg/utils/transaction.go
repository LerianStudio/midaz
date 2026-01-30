package utils

import pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"

// SanitizeAccountAliases cleans the AccountAlias fields in a Transaction's FromTo entries.
// This is necessary because HandleAccountFields mutates aliases in-place using ConcatAlias,
// producing formats like "0#@alias#key". SplitAlias extracts the original alias back.
func SanitizeAccountAliases(transactionInput *pkgTransaction.Transaction) {
	if transactionInput == nil {
		return
	}

	for i := range transactionInput.Send.Source.From {
		transactionInput.Send.Source.From[i].AccountAlias = transactionInput.Send.Source.From[i].SplitAlias()
	}

	for i := range transactionInput.Send.Distribute.To {
		transactionInput.Send.Distribute.To[i].AccountAlias = transactionInput.Send.Distribute.To[i].SplitAlias()
	}
}
