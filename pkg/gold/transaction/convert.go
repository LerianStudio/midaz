package transaction

import (
	"time"

	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// ConvertLibToPkgTransaction converts lib-commons Transaction to pkg Transaction.
// This is necessary because the Gold DSL parser returns lib-commons types,
// but the handler layer expects pkg types.
func ConvertLibToPkgTransaction(lib libTransaction.Transaction) pkgTransaction.Transaction {
	return pkgTransaction.Transaction{
		ChartOfAccountsGroupName: lib.ChartOfAccountsGroupName,
		Description:              lib.Description,
		Code:                     lib.Code,
		Pending:                  lib.Pending,
		Metadata:                 lib.Metadata,
		Route:                    lib.Route,
		TransactionDate:          convertTransactionDate(lib.TransactionDate),
		Send:                     convertSend(lib.Send),
	}
}

// convertTransactionDate converts time.Time to *pkgTransaction.TransactionDate.
func convertTransactionDate(t time.Time) *pkgTransaction.TransactionDate {
	if t.IsZero() {
		return nil
	}

	td := pkgTransaction.TransactionDate(t)

	return &td
}

// convertSend converts lib-commons Send to pkg Send.
func convertSend(lib libTransaction.Send) pkgTransaction.Send {
	return pkgTransaction.Send{
		Asset:      lib.Asset,
		Value:      lib.Value,
		Source:     convertSource(lib.Source),
		Distribute: convertDistribute(lib.Distribute),
	}
}

// convertSource converts lib-commons Source to pkg Source.
func convertSource(lib libTransaction.Source) pkgTransaction.Source {
	from := make([]pkgTransaction.FromTo, 0, len(lib.From))
	for _, f := range lib.From {
		from = append(from, convertFromTo(f))
	}

	return pkgTransaction.Source{
		Remaining: lib.Remaining,
		From:      from,
	}
}

// convertDistribute converts lib-commons Distribute to pkg Distribute.
func convertDistribute(lib libTransaction.Distribute) pkgTransaction.Distribute {
	to := make([]pkgTransaction.FromTo, 0, len(lib.To))
	for _, t := range lib.To {
		to = append(to, convertFromTo(t))
	}

	return pkgTransaction.Distribute{
		Remaining: lib.Remaining,
		To:        to,
	}
}

// convertFromTo converts lib-commons FromTo to pkg FromTo.
func convertFromTo(lib libTransaction.FromTo) pkgTransaction.FromTo {
	return pkgTransaction.FromTo{
		AccountAlias:    lib.AccountAlias,
		BalanceKey:      lib.BalanceKey,
		Amount:          convertAmount(lib.Amount),
		Share:           convertShare(lib.Share),
		Remaining:       lib.Remaining,
		Rate:            convertRate(lib.Rate),
		Description:     lib.Description,
		ChartOfAccounts: lib.ChartOfAccounts,
		Metadata:        lib.Metadata,
		IsFrom:          lib.IsFrom,
		Route:           lib.Route,
	}
}

// convertAmount converts lib-commons Amount pointer to pkg Amount pointer.
func convertAmount(lib *libTransaction.Amount) *pkgTransaction.Amount {
	if lib == nil {
		return nil
	}

	return &pkgTransaction.Amount{
		Asset:           lib.Asset,
		Value:           lib.Value,
		Operation:       lib.Operation,
		TransactionType: lib.TransactionType,
	}
}

// convertShare converts lib-commons Share pointer to pkg Share pointer.
func convertShare(lib *libTransaction.Share) *pkgTransaction.Share {
	if lib == nil {
		return nil
	}

	return &pkgTransaction.Share{
		Percentage:             lib.Percentage,
		PercentageOfPercentage: lib.PercentageOfPercentage,
	}
}

// convertRate converts lib-commons Rate pointer to pkg Rate pointer.
func convertRate(lib *libTransaction.Rate) *pkgTransaction.Rate {
	if lib == nil {
		return nil
	}

	return &pkgTransaction.Rate{
		From:       lib.From,
		To:         lib.To,
		Value:      lib.Value,
		ExternalID: lib.ExternalID,
	}
}
