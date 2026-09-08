// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"fmt"
	"maps"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// ErrBalanceEngineTransactionNotExecutable identifies transaction shapes that
// deliberately have no monetary execution, such as annotations.
var ErrBalanceEngineTransactionNotExecutable = errors.New("balance engine transaction is not executable")

// ErrInvalidBalanceEngineTranslation identifies inconsistent internal inputs at
// the command-to-engine translation seam.
var ErrInvalidBalanceEngineTranslation = errors.New("invalid balance engine translation")

// BalanceEngineTranslationInput carries ordered transaction legs and their
// validated intentions together with the complete scoped balance pool.
type BalanceEngineTranslationInput struct {
	TransactionID     uuid.UUID
	Action            string
	TransactionStatus string
	// RouteValidationEnabled is the ledger-level path decision. It is separate
	// from Amount.RouteValidationEnabled, which selects composed source postings.
	RouteValidationEnabled bool
	TransactionInput       mtransaction.Transaction
	Validate               *mtransaction.Responses
	Balances               []*mmodel.Balance
	RouteCache             *mmodel.TransactionRouteCache
}

// TranslateBalanceEngineTransaction converts command-layer transaction intent
// into ordered engine postings and immutable accounting-row context.
func TranslateBalanceEngineTransaction(input BalanceEngineTranslationInput) (engine.Transaction, []FrozenProjectionContext, error) {
	if input.TransactionStatus == constant.NOTED {
		return engine.Transaction{}, nil, ErrBalanceEngineTransactionNotExecutable
	}

	if input.TransactionID == uuid.Nil || input.Validate == nil {
		return engine.Transaction{}, nil, invalidBalanceEngineTranslation("missing transaction identity or validation")
	}

	balances, err := indexTranslationBalances(input.Balances)
	if err != nil {
		return engine.Transaction{}, nil, err
	}

	transaction := engine.Transaction{ID: input.TransactionID, Postings: make([]engine.Posting, 0)}
	projection := make([]FrozenProjectionContext, 0)

	for index, leg := range input.TransactionInput.Send.Source.From {
		amount, exists := input.Validate.From[leg.AccountAlias]
		if !exists {
			return engine.Transaction{}, nil, invalidBalanceEngineTranslation("missing validated source leg")
		}

		if err := appendLegTranslation(&transaction, &projection, input, balances, leg, amount, ProjectionSideFrom, index); err != nil {
			return engine.Transaction{}, nil, err
		}
	}

	for index, leg := range input.TransactionInput.Send.Distribute.To {
		amount, exists := input.Validate.To[leg.AccountAlias]
		if !exists {
			return engine.Transaction{}, nil, invalidBalanceEngineTranslation("missing validated destination leg")
		}

		if err := appendLegTranslation(&transaction, &projection, input, balances, leg, amount, ProjectionSideTo, index); err != nil {
			return engine.Transaction{}, nil, err
		}
	}

	return transaction, projection, nil
}

func appendLegTranslation(transaction *engine.Transaction, projection *[]FrozenProjectionContext, input BalanceEngineTranslationInput, balances map[string]*mmodel.Balance, leg mtransaction.FromTo, amount mtransaction.Amount, side string, index int) error {
	if !amount.Value.IsPositive() {
		return invalidBalanceEngineTranslation("posting amount must be positive")
	}

	if amount.OverdraftAmount.IsNegative() {
		return invalidBalanceEngineTranslation("overdraft amount must not be negative")
	}

	specs, err := translationPostingSpecs(input, amount, side)
	if err != nil {
		return err
	}

	if len(specs) == 0 {
		return nil
	}

	balanceRef := mtransaction.SplitAliasWithKey(leg.AccountAlias)

	balance, exists := balances[balanceRef]
	if !exists {
		return invalidBalanceEngineTranslation("ordered leg has no balance in scoped pool")
	}

	routeID := translationRouteID(input.Validate, leg, side)
	originRef := fmt.Sprintf("%s:%d", side, index)

	for _, spec := range specs {
		postingRef := fmt.Sprintf("%s:%s", originRef, spec.postingType)

		drawPolicy := engine.DrawForbidden
		if spec.postingType == engine.PostingDebit && spec.allowDraw {
			drawPolicy = engine.DrawAllowed
			if input.RouteValidationEnabled && !translationOverdraftRubricConfigured(input.RouteCache, routeID, constant.DirectionDebit) {
				drawPolicy = engine.DrawRouteDenied
			}
		}

		transaction.Postings = append(transaction.Postings, engine.Posting{
			Ref: postingRef, BalanceRef: balanceRef, Type: spec.postingType, Amount: amount.Value,
			DrawPolicy: drawPolicy, OverdraftAmount: spec.overdraftAmount,
		})

		primary := newFrozenProjectionContext(input, leg, balance, postingRef, originRef, side, spec.rowType, spec.direction, routeID, amount.Value, spec.compatibilityPath)
		*projection = append(*projection, primary)

		if !spec.mayMoveOverdraft {
			continue
		}

		companionRef := mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), constant.OverdraftBalanceKey)
		if companion, ok := balances[companionRef]; ok {
			companionContext := newFrozenProjectionContext(input, leg, companion, postingRef, originRef, side, constant.OVERDRAFT, spec.direction, routeID, amount.Value, ProjectionStandard)
			companionContext.Role = engine.RoleOverdraftCompanion
			companionContext.ChartOfAccounts = ""
			companionContext.Metadata = map[string]any{}
			companionContext.RouteCode, companionContext.RouteDescription = translationRubric(input.RouteCache, routeID, constant.ActionOverdraft, spec.direction)
			*projection = append(*projection, companionContext)
		}
	}

	return nil
}

type translationPostingSpec struct {
	postingType       engine.PostingType
	rowType           string
	direction         string
	compatibilityPath string
	overdraftAmount   decimal.Decimal
	allowDraw         bool
	mayMoveOverdraft  bool
}

func translationPostingSpecs(input BalanceEngineTranslationInput, amount mtransaction.Amount, side string) ([]translationPostingSpec, error) {
	credit := translationPostingSpec{
		postingType: engine.PostingCredit, rowType: constant.CREDIT, direction: constant.DirectionCredit,
		compatibilityPath: ProjectionStandard, overdraftAmount: amount.OverdraftAmount, mayMoveOverdraft: true,
	}
	debit := translationPostingSpec{
		postingType: engine.PostingDebit, rowType: constant.DEBIT, direction: constant.DirectionDebit,
		compatibilityPath: ProjectionStandard, allowDraw: true, mayMoveOverdraft: true,
	}

	switch input.Action {
	case constant.ActionDirect, constant.ActionRevert:
		if input.TransactionStatus != constant.CREATED {
			return nil, invalidBalanceEngineTranslation("direct or revert action requires created status")
		}

		if side == ProjectionSideFrom {
			return []translationPostingSpec{debit}, nil
		}

		return []translationPostingSpec{credit}, nil
	case constant.ActionHold:
		if input.TransactionStatus != constant.PENDING {
			return nil, invalidBalanceEngineTranslation("hold action requires pending status")
		}

		if side == ProjectionSideTo {
			return nil, nil
		}

		if !amount.RouteValidationEnabled {
			return []translationPostingSpec{{
				postingType: engine.PostingHold, rowType: constant.ONHOLD, direction: constant.DirectionDebit,
				compatibilityPath: ProjectionStandard,
			}}, nil
		}

		return []translationPostingSpec{
			{postingType: engine.PostingDebit, rowType: constant.DEBIT, direction: constant.DirectionDebit, compatibilityPath: ProjectionValidatedHoldDebit},
			{postingType: engine.PostingReserve, rowType: constant.ONHOLD, direction: constant.DirectionCredit, compatibilityPath: ProjectionValidatedHoldReserve},
		}, nil
	case constant.ActionCommit:
		if input.TransactionStatus != constant.APPROVED {
			return nil, invalidBalanceEngineTranslation("commit action requires approved status")
		}

		if side == ProjectionSideTo {
			return []translationPostingSpec{credit}, nil
		}

		rowType := constant.DEBIT
		if amount.RouteValidationEnabled {
			rowType = constant.ONHOLD
		}

		return []translationPostingSpec{{
			postingType: engine.PostingUnreserve, rowType: rowType, direction: constant.DirectionDebit,
			compatibilityPath: ProjectionStandard,
		}}, nil
	case constant.ActionCancel:
		if input.TransactionStatus != constant.CANCELED {
			return nil, invalidBalanceEngineTranslation("cancel action requires canceled status")
		}

		if side == ProjectionSideTo {
			return nil, nil
		}

		if !amount.RouteValidationEnabled {
			return []translationPostingSpec{{
				postingType: engine.PostingRelease, rowType: constant.RELEASE, direction: constant.DirectionCredit,
				compatibilityPath: ProjectionStandard, overdraftAmount: amount.OverdraftAmount,
				mayMoveOverdraft: amount.OverdraftAmount.IsPositive(),
			}}, nil
		}

		return []translationPostingSpec{
			{postingType: engine.PostingUnreserve, rowType: constant.RELEASE, direction: constant.DirectionDebit, compatibilityPath: ProjectionValidatedCancelRelease},
			{
				postingType: engine.PostingCredit, rowType: constant.CREDIT, direction: constant.DirectionCredit,
				compatibilityPath: ProjectionValidatedCancelCredit, overdraftAmount: amount.OverdraftAmount, mayMoveOverdraft: true,
			},
		}, nil
	default:
		return nil, invalidBalanceEngineTranslation("unsupported transaction action")
	}
}

func newFrozenProjectionContext(input BalanceEngineTranslationInput, leg mtransaction.FromTo, balance *mmodel.Balance, postingRef, originRef, side, rowType, direction, routeID string, requestedAmount decimal.Decimal, compatibilityPath string) FrozenProjectionContext {
	description := leg.Description
	if description == "" {
		description = input.TransactionInput.Description
	}

	var frozenRouteID *string

	if routeID != "" {
		value := routeID
		frozenRouteID = &value
	}

	routeCode, routeDescription := translationRubric(input.RouteCache, routeID, input.Action, direction)

	frozenBalance := cloneTranslationBalance(balance)

	return FrozenProjectionContext{
		TransactionID: input.TransactionID, PostingRef: postingRef, OriginRef: originRef,
		BalanceRef: mtransaction.AliasKey(frozenBalance.Alias, frozenBalance.Key),
		Role:       engine.RolePrimary, Side: side, RowType: rowType, Direction: direction,
		Description: description, RouteID: frozenRouteID, RouteCode: routeCode, RouteDescription: routeDescription,
		ChartOfAccounts: leg.ChartOfAccounts, Metadata: maps.Clone(leg.Metadata), Balance: frozenBalance,
		RequestedAmount: requestedAmount, CompatibilityPath: compatibilityPath,
	}
}

func indexTranslationBalances(balances []*mmodel.Balance) (map[string]*mmodel.Balance, error) {
	indexed := make(map[string]*mmodel.Balance, len(balances))
	for _, balance := range balances {
		if balance == nil {
			return nil, invalidBalanceEngineTranslation("nil balance in scoped pool")
		}

		ref := mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), balance.Key)
		if _, duplicate := indexed[ref]; duplicate {
			return nil, invalidBalanceEngineTranslation("duplicate logical balance in scoped pool")
		}

		indexed[ref] = balance
	}

	return indexed, nil
}

func translationRouteID(validate *mtransaction.Responses, leg mtransaction.FromTo, side string) string {
	var routeID string
	if side == ProjectionSideFrom {
		routeID = validate.OperationRoutesFrom[leg.AccountAlias]
	} else {
		routeID = validate.OperationRoutesTo[leg.AccountAlias]
	}

	if routeID == "" && leg.RouteID != nil {
		routeID = *leg.RouteID
	}

	return routeID
}

func translationOverdraftRubricConfigured(cache *mmodel.TransactionRouteCache, routeID, direction string) bool {
	code, _ := translationRubric(cache, routeID, constant.ActionOverdraft, direction)
	return code != ""
}

func translationRubric(cache *mmodel.TransactionRouteCache, routeID, action, direction string) (string, string) {
	if cache == nil || routeID == "" {
		return "", ""
	}

	actionCache, ok := cache.Actions[action]
	if !ok {
		return "", ""
	}

	route, ok := actionCache.FindRoute(routeID)
	if !ok {
		return "", ""
	}

	rubric := resolveAccountingRubric(route.AccountingEntries, action, direction)
	if rubric == nil {
		return "", ""
	}

	return rubric.Code, rubric.Description
}

func cloneTranslationBalance(balance *mmodel.Balance) FrozenProjectionBalance {
	cloned := *balance

	cloned.Alias = mtransaction.SplitAlias(balance.Alias)
	if cloned.Key == "" {
		cloned.Key = constant.DefaultBalanceKey
	}

	cloned.Metadata = maps.Clone(balance.Metadata)
	if balance.Settings != nil {
		settings := *balance.Settings
		if balance.Settings.OverdraftLimit != nil {
			limit := *balance.Settings.OverdraftLimit
			settings.OverdraftLimit = &limit
		}

		cloned.Settings = &settings
	}

	if balance.DeletedAt != nil {
		deletedAt := *balance.DeletedAt
		cloned.DeletedAt = &deletedAt
	}

	return FrozenProjectionBalance(cloned)
}

func invalidBalanceEngineTranslation(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidBalanceEngineTranslation, message)
}
