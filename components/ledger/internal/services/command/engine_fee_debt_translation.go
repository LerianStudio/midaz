// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"

	"github.com/shopspring/decimal"

	transactionPostgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// feeDebtOriginLength is the length of the transaction UUID that leads every debt id.
const feeDebtOriginLength = 36

// feeDebtComposition declares one transaction's fee-debt work while it is
// translated. Go only declares: Lua decides every amount moved.
type feeDebtComposition struct {
	input    EngineTranslationInput
	balances map[string]*mmodel.Balance
	// pairs maps a feeDeferPair token to the ref of the payer debit it marked.
	pairs    map[string]string
	declared map[string]struct{}
}

func newFeeDebtComposition(input EngineTranslationInput, balances map[string]*mmodel.Balance) *feeDebtComposition {
	return &feeDebtComposition{input: input, balances: balances, pairs: map[string]string{}, declared: map[string]struct{}{}}
}

// bookTakeBack books a revert source that takes back a fee-debt settlement to the
// revert rubric its debt stored, which no route of the reverted credit resolves.
func (c *feeDebtComposition) bookTakeBack(primary *OperationRecordSpec) {
	if primary.Side != OperationSpecSideFrom || primary.RouteID == nil {
		return
	}

	if route := c.input.FeeDebtTakeBacks[transactionPostgres.FeeSettlementGroup{Ref: primary.BalanceRef, RouteID: *primary.RouteID}]; route != nil {
		primary.RouteCode, primary.RouteDescription = route.RevertCode, route.RevertDescription
	}
}

// markDeferral flags the two legs of one deferrable fee on a /v2 direct
// transaction: the payer debit defers its shortfall and the fee credit is funded
// by that debit, each carrying its primary's route for the debt to keep. Pairing
// is validated by the engine, not here.
func (c *feeDebtComposition) markDeferral(transaction *accounting.Transaction, posting *accounting.Posting, primary OperationRecordSpec) {
	if !c.input.FeeDebtEligible || c.input.Action != constant.ActionDirect {
		return
	}

	token, _ := primary.Metadata[constant.MetadataKeyFeeDeferPair].(string)
	if token == "" {
		return
	}

	if primary.RouteID != nil {
		reversed := constant.DirectionDebit
		if primary.Direction == constant.DirectionDebit {
			reversed = constant.DirectionCredit
		}

		route := &accounting.FeeDebtRoute{ID: *primary.RouteID, Code: primary.RouteCode, Description: primary.RouteDescription}
		route.RevertCode, route.RevertDescription = translationRubric(c.input.RouteCache, route.ID,
			crossLedgerRubricAction(c.input.RouteCache, route.ID, constant.ActionRevert), reversed)
		posting.DebtRoute = route
	}

	switch posting.Type {
	case accounting.PostingDebit:
		posting.DeferShortfall = true
		c.pairs[token] = posting.Ref
		c.declare(transaction, posting.BalanceRef)
	case accounting.PostingCredit:
		posting.FundedByRef = c.pairs[token]
	}
}

// appendCollect follows a /v2 credit or refund with a collect of the credited
// balance's open debts, bounded by the amount it credits.
func (c *feeDebtComposition) appendCollect(transaction *accounting.Transaction, projection *[]OperationRecordSpec, credit accounting.Posting) {
	if !c.input.FeeDebtEligible || (credit.Type != accounting.PostingCredit && credit.Type != accounting.PostingRefund) ||
		(c.input.Action != constant.ActionDirect && c.input.Action != constant.ActionCommit && c.input.Action != constant.ActionRevert) {
		return
	}

	c.collect(transaction, projection, credit.Ref+":collect", credit.BalanceRef, credit.Amount)
}

// collect appends a collect of balanceRef's seeded debts, named oldest first, when
// the seed names any, and reports whether it did. Each debt books its debtor debit
// and creditor credit under the routes it stored.
func (c *feeDebtComposition) collect(transaction *accounting.Transaction, projection *[]OperationRecordSpec, ref, balanceRef string, amount decimal.Decimal) bool {
	seed, debtor := c.input.FeeDebtSeeds[balanceRef], c.balances[balanceRef]
	contexts := make([]OperationRecordSpec, 0, 2*len(seed))
	items := make([]string, 0, len(seed))

	var ordinal uint32

	// Lua stops the collect at the first creditor outside the pool, so the items end there.
	for _, item := range seed {
		creditor, pooled := c.balances[item.CreditRef]
		if !pooled {
			break
		}

		contexts = append(contexts,
			c.spec(ref, debtor, accounting.RoleFeeDebtDebit, ordinal, constant.FEE_SETTLEMENT, constant.DirectionDebit, amount, item.DebitRoute),
			c.spec(ref, creditor, accounting.RoleFeeDebtCredit, ordinal, constant.FEE_SETTLEMENT, constant.DirectionCredit, amount, item.CreditRoute))
		items = append(items, item.ID)
		ordinal++
	}

	if len(items) == 0 {
		return false
	}

	transaction.Postings = append(transaction.Postings, accounting.Posting{
		Ref: ref, BalanceRef: balanceRef, Type: accounting.PostingCollect, Amount: amount,
		DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero, Items: items,
	})
	*projection = append(*projection, contexts...)

	c.declare(transaction, balanceRef)

	return true
}

// appendRevert composes, on any revert, the refunds of the debts the parent
// opened and the reopens of the debts it settled, read from the reversal's
// inherited metadata.
func (c *feeDebtComposition) appendRevert(transaction *accounting.Transaction, projection *[]OperationRecordSpec) error {
	if c.input.Action != constant.ActionRevert {
		return nil
	}

	openings, settlements, err := feeDebtRevertFacts(c.input.TransactionInput.Metadata)
	if err != nil {
		return err
	}

	if err := c.appendRefunds(transaction, projection, openings); err != nil {
		return err
	}

	for _, settlement := range settlements {
		c.declare(transaction, settlement.DebtorRef)
	}

	transaction.ReopenFeeDebts = feeDebtReopens(settlements, c.input.TransactionInput.FeeDebtRevertedOrigins)
	for _, reopen := range transaction.ReopenFeeDebts {
		if _, exists := c.balances[reopen.DebtorRef]; !exists {
			return pkg.ValidateBusinessError(constant.ErrAccountIneligibility, balanceValidationEntity)
		}
	}

	return nil
}

// appendRefunds appends one refund posting per debtor of the parent's openings,
// in first-appearance order, after every other posting. Each debt refunds as the
// inverse of its fee, under its routes, expecting what its record says was paid.
func (c *feeDebtComposition) appendRefunds(transaction *accounting.Transaction, projection *[]OperationRecordSpec, openings []FeeDebtOpening) error {
	debtors := make([]string, 0)
	byDebtor := make(map[string][]FeeDebtOpening)

	for _, opening := range openings {
		if _, seen := byDebtor[opening.DebtorRef]; !seen {
			debtors = append(debtors, opening.DebtorRef)
		}

		byDebtor[opening.DebtorRef] = append(byDebtor[opening.DebtorRef], opening)
	}

	for n, debtorRef := range debtors {
		debtor, exists := c.balances[debtorRef]
		if !exists {
			return pkg.ValidateBusinessError(constant.ErrAccountIneligibility, balanceValidationEntity)
		}

		ref := "fee-refund:" + strconv.Itoa(n)
		posting := accounting.Posting{Ref: ref, BalanceRef: debtorRef, Type: accounting.PostingRefund, DrawPolicy: accounting.DrawForbidden, OverdraftAmount: decimal.Zero}
		refunded := decimal.Zero
		companion, hasCompanion := c.balances[mtransaction.AliasKey(mtransaction.SplitAlias(debtor.Alias), constant.OverdraftBalanceKey)]

		for i, opening := range byDebtor[debtorRef] {
			creditor, found := c.balances[opening.CreditRef]
			if !found {
				return pkg.ValidateBusinessError(constant.ErrAccountIneligibility, balanceValidationEntity)
			}

			ordinal, credited := uint32(i), c.refundRoute(opening.DebitRoute, false)
			posting.Amount = posting.Amount.Add(opening.Opened)
			posting.Refunds = append(posting.Refunds, accounting.FeeDebtRefund{
				DebtID: opening.DebtID, CreditRef: opening.CreditRef, Opened: opening.Opened, Seq: opening.Seq,
				ExpectedRefund: c.input.TransactionInput.FeeDebtExpectedRefunds[opening.DebtID],
			})
			refunded = refunded.Add(c.input.TransactionInput.FeeDebtExpectedRefunds[opening.DebtID])
			*projection = append(*projection, c.spec(ref, debtor, accounting.RoleFeeDebtRefundCredit, ordinal, constant.FEE_REFUND, constant.DirectionCredit, opening.Opened, credited))

			if hasCompanion {
				*projection = append(*projection, c.spec(ref, companion, accounting.RoleOverdraftCompanion, ordinal, constant.FEE_REFUND, constant.DirectionCredit, opening.Opened,
					c.refundRoute(opening.DebitRoute, true)))
			}

			*projection = append(*projection, c.spec(ref, creditor, accounting.RoleFeeDebtRefundDebit, ordinal, constant.FEE_REFUND, constant.DirectionDebit, opening.Opened,
				c.refundRoute(opening.CreditRoute, false)))
		}

		transaction.Postings = append(transaction.Postings, posting)

		c.declare(transaction, debtorRef)

		// The refund credits the debtor what it pays, so it settles the debtor's other debts
		// like any credit; the parent's own were canceled before any posting runs.
		if refunded.IsPositive() {
			c.appendCollect(transaction, projection, accounting.Posting{Ref: ref, BalanceRef: debtorRef, Type: accounting.PostingRefund, Amount: refunded})
		}
	}

	return nil
}

func (c *feeDebtComposition) declare(transaction *accounting.Transaction, balanceRef string) {
	if _, seen := c.declared[balanceRef]; seen {
		return
	}

	c.declared[balanceRef] = struct{}{}
	transaction.FeeDebtRefs = append(transaction.FeeDebtRefs, balanceRef)
}

// refundRoute is route booked to the revert rubric its debt stored, or, for the
// overdraft its debtor credit repays, to the route's live overdraft rubric.
func (c *feeDebtComposition) refundRoute(route *accounting.FeeDebtRoute, overdraft bool) *accounting.FeeDebtRoute {
	if route == nil {
		return nil
	}

	if !overdraft {
		return &accounting.FeeDebtRoute{ID: route.ID, Code: route.RevertCode, Description: route.RevertDescription}
	}

	code, description := translationRubric(c.input.RouteCache, route.ID, constant.ActionOverdraft, constant.DirectionCredit)

	return &accounting.FeeDebtRoute{ID: route.ID, Code: code, Description: description}
}

// spec is the context of a fee-debt movement: no origin leg and no metadata,
// matched by posting ref, role and ordinal, booked under route when it has one.
// The requested amount bounds the movement the engine may record.
func (c *feeDebtComposition) spec(postingRef string, balance *mmodel.Balance, role string, ordinal uint32, rowType, direction string, requested decimal.Decimal, route *accounting.FeeDebtRoute) OperationRecordSpec {
	side := OperationSpecSideTo
	if direction == constant.DirectionDebit {
		side = OperationSpecSideFrom
	}

	stable := cloneTranslationBalance(balance)

	spec := OperationRecordSpec{
		TransactionID: c.input.TransactionID, PostingRef: postingRef,
		BalanceRef: mtransaction.AliasKey(stable.Alias, stable.Key), Role: role, Ordinal: ordinal,
		Side: side, RowType: rowType, Direction: direction, Description: c.input.TransactionInput.Description,
		Metadata: map[string]any{}, Balance: stable, RequestedAmount: requested, CompatibilityPath: OperationRecordStandard,
	}

	if route != nil {
		id := route.ID
		spec.RouteID, spec.RouteCode, spec.RouteDescription = &id, route.Code, route.Description
	}

	return spec
}

// feeDebtReopens sums the settlements of each debt into one reopen, ordered by
// seq, skipping every debt whose origin is already reverted.
func feeDebtReopens(settlements []FeeDebtSettlement, revertedOrigins []string) []accounting.FeeDebtReopen {
	var reopens []accounting.FeeDebtReopen

	positions := make(map[string]int)

	for _, settlement := range settlements {
		if slices.Contains(revertedOrigins, feeDebtOrigin(settlement.DebtID)) {
			continue
		}

		if position, seen := positions[settlement.DebtID]; seen {
			reopens[position].Amount = reopens[position].Amount.Add(settlement.Amount)
			continue
		}

		positions[settlement.DebtID] = len(reopens)
		reopens = append(reopens, accounting.FeeDebtReopen{
			DebtID: settlement.DebtID, DebtorRef: settlement.DebtorRef, CreditRef: settlement.CreditRef,
			Amount: settlement.Amount, Opened: settlement.Opened, Seq: settlement.Seq,
			DebitRoute: settlement.DebitRoute, CreditRoute: settlement.CreditRoute,
		})
	}

	sort.SliceStable(reopens, func(i, j int) bool { return reopens[i].Seq < reopens[j].Seq })

	return reopens
}

// feeDebtOrigin returns the transaction id that leads a debt id.
func feeDebtOrigin(debtID string) string {
	return debtID[:min(len(debtID), feeDebtOriginLength)]
}

// feeDebtRouteView records on translation what a revert takes back of fee-debt
// settlements and returns the intent route validation reads, naming those sources.
func feeDebtRouteView(translation *EngineTranslationInput) (*mtransaction.Responses, error) {
	takeBacks, err := feeDebtTakeBacks(*translation)
	if err != nil || len(takeBacks) == 0 {
		return translation.Validate, err
	}

	translation.FeeDebtTakeBacks = takeBacks
	view := *translation.Validate
	view.FeeDebtLegs = make(map[string]bool)

	for _, leg := range translation.TransactionInput.Send.Source.From {
		group := transactionPostgres.FeeSettlementGroup{Ref: mtransaction.SplitAliasWithKey(leg.AccountAlias)}
		if leg.RouteID != nil {
			group.RouteID = *leg.RouteID
		}

		if _, taken := takeBacks[group]; taken {
			view.FeeDebtLegs[leg.AccountAlias] = true
		}
	}

	return &view, nil
}

// feeDebtTakeBacks maps each creditor and route a revert takes back a settlement
// from to the settlement's stored route. A settlement whose origin is already
// reverted stays with its creditor and is taken back from no one.
func feeDebtTakeBacks(input EngineTranslationInput) (map[transactionPostgres.FeeSettlementGroup]*accounting.FeeDebtRoute, error) {
	if input.Action != constant.ActionRevert {
		return nil, nil
	}

	_, settlements, err := feeDebtRevertFacts(input.TransactionInput.Metadata)
	if err != nil {
		return nil, err
	}

	takeBacks := make(map[transactionPostgres.FeeSettlementGroup]*accounting.FeeDebtRoute, len(settlements))
	for _, settlement := range settlements {
		if !slices.Contains(input.TransactionInput.FeeDebtRevertedOrigins, feeDebtOrigin(settlement.DebtID)) {
			takeBacks[transactionPostgres.FeeSettlementGroup{Ref: settlement.CreditRef, RouteID: feeDebtRouteID(settlement.CreditRoute)}] = settlement.CreditRoute
		}
	}

	return takeBacks, nil
}

// feeDebtRevertFacts reads the debts a transaction opened and settled from its
// metadata, where completion stores each list as a JSON string.
func feeDebtRevertFacts(metadata map[string]any) ([]FeeDebtOpening, []FeeDebtSettlement, error) {
	openings, err := decodeFeeDebtMetadata[FeeDebtOpening](metadata, constant.MetadataKeyFeeDebtOpenings)
	if err != nil {
		return nil, nil, err
	}

	settlements, err := decodeFeeDebtMetadata[FeeDebtSettlement](metadata, constant.MetadataKeyFeeDebtSettlements)
	if err != nil {
		return nil, nil, err
	}

	return openings, settlements, nil
}

func decodeFeeDebtMetadata[T any](metadata map[string]any, key string) ([]T, error) {
	value, exists := metadata[key]
	if !exists {
		return nil, nil
	}

	text, isString := value.(string)
	if !isString {
		return nil, fmt.Errorf("%w: fee debt metadata %s is not a string", ErrInvalidEngineTranslation, key)
	}

	var decoded []T
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, fmt.Errorf("%w: decode fee debt metadata %s: %w", ErrInvalidEngineTranslation, key, err)
	}

	return decoded, nil
}
