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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// ErrEngineTransactionNotExecutable identifies transaction shapes that
// deliberately have no monetary execution, such as annotations.
var ErrEngineTransactionNotExecutable = errors.New("engine transaction is not executable")

// ErrInvalidEngineTranslation identifies inconsistent internal inputs at
// the command-to-engine translation seam.
var ErrInvalidEngineTranslation = errors.New("invalid engine translation")

// EngineTranslationInput carries ordered transaction legs and their
// validated intentions together with the complete scoped balance pool.
type EngineTranslationInput struct {
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

// TranslateEngineTransaction converts command-layer transaction intent
// into ordered engine postings and immutable accounting-row context.
func TranslateEngineTransaction(input EngineTranslationInput) (accounting.Transaction, []OperationRecordSpec, error) {
	if input.TransactionStatus == constant.NOTED {
		return accounting.Transaction{}, nil, ErrEngineTransactionNotExecutable
	}

	if input.TransactionID == uuid.Nil || input.Validate == nil {
		return accounting.Transaction{}, nil, invalidEngineTranslation("missing transaction identity or validation")
	}

	balances, err := indexTranslationBalances(input.Balances)
	if err != nil {
		return accounting.Transaction{}, nil, err
	}

	transaction := accounting.Transaction{
		ID:                    input.TransactionID,
		RejectBlockedBalances: input.Action != constant.ActionCancel,
		BalanceRequirements:   engineRequirements(input),
		Postings:              make([]accounting.Posting, 0),
	}
	projection := make([]OperationRecordSpec, 0)

	for index, leg := range input.TransactionInput.Send.Source.From {
		amount, exists := input.Validate.From[leg.AccountAlias]
		if !exists {
			return accounting.Transaction{}, nil, invalidEngineTranslation("missing validated source leg")
		}

		if err := appendLegTranslation(&transaction, &projection, input, balances, leg, amount, OperationSpecSideFrom, index); err != nil {
			return accounting.Transaction{}, nil, err
		}
	}

	for index, leg := range input.TransactionInput.Send.Distribute.To {
		amount, exists := input.Validate.To[leg.AccountAlias]
		if !exists {
			return accounting.Transaction{}, nil, invalidEngineTranslation("missing validated destination leg")
		}

		if err := appendLegTranslation(&transaction, &projection, input, balances, leg, amount, OperationSpecSideTo, index); err != nil {
			return accounting.Transaction{}, nil, err
		}
	}

	return transaction, projection, nil
}

func engineRequirements(input EngineTranslationInput) []accounting.BalanceRequirement {
	if input.Action == constant.ActionCommit || input.Action == constant.ActionCancel {
		return []accounting.BalanceRequirement{}
	}

	requirements := make([]accounting.BalanceRequirement, 0,
		len(input.TransactionInput.Send.Source.From)+len(input.TransactionInput.Send.Distribute.To))
	assetCode := input.TransactionInput.Send.Asset

	for _, leg := range input.TransactionInput.Send.Source.From {
		requirements = append(requirements, accounting.BalanceRequirement{
			BalanceRef:     mtransaction.SplitAliasWithKey(leg.AccountAlias),
			AssetCode:      assetCode,
			Permission:     accounting.BalancePermissionSend,
			ForbidExternal: input.Action == constant.ActionHold,
		})
	}

	for _, leg := range input.TransactionInput.Send.Distribute.To {
		requirements = append(requirements, accounting.BalanceRequirement{
			BalanceRef: mtransaction.SplitAliasWithKey(leg.AccountAlias),
			AssetCode:  assetCode,
			Permission: accounting.BalancePermissionReceive,
		})
	}

	return requirements
}

func appendLegTranslation(transaction *accounting.Transaction, projection *[]OperationRecordSpec, input EngineTranslationInput, balances map[string]*mmodel.Balance, leg mtransaction.FromTo, amount mtransaction.Amount, side string, index int) error {
	if !amount.Value.IsPositive() {
		return invalidEngineTranslation("posting amount must be positive")
	}

	if amount.OverdraftAmount.IsNegative() {
		return invalidEngineTranslation("overdraft amount must not be negative")
	}

	plan, err := buildPostingPlan(
		input.Action,
		input.TransactionStatus,
		side,
		amount.RouteValidationEnabled,
		amount.OverdraftAmount,
	)
	if err != nil {
		return err
	}

	if len(plan.items) == 0 {
		return nil
	}

	balanceRef := mtransaction.SplitAliasWithKey(leg.AccountAlias)

	balance, exists := balances[balanceRef]
	if !exists {
		return invalidEngineTranslation("ordered leg has no balance in scoped pool")
	}

	routeID := translationRouteID(input.Validate, leg, side)
	originRef := fmt.Sprintf("%s:%d", side, index)

	for _, item := range plan.items {
		postingRef := fmt.Sprintf("%s:%s", originRef, item.postingType)

		drawPolicy := accounting.DrawForbidden
		if item.postingType == accounting.PostingDebit && item.allowsOverdraftDraw {
			drawPolicy = accounting.DrawAllowed
			if input.RouteValidationEnabled && !translationOverdraftRubricConfigured(input.RouteCache, routeID, constant.DirectionDebit) {
				drawPolicy = accounting.DrawRouteDenied
			}
		}

		transaction.Postings = append(transaction.Postings, accounting.Posting{
			Ref: postingRef, BalanceRef: balanceRef, Type: item.postingType, Amount: amount.Value,
			DrawPolicy: drawPolicy, OverdraftAmount: item.historicalOverdraftCap,
		})

		primary := newOperationRecordSpec(input, leg, balance, postingRef, originRef, side, item.operationRowType, item.operationDirection, routeID, amount.Value, item.operationProjectionMode)
		*projection = append(*projection, primary)

		if !item.mayAffectOverdraft {
			continue
		}

		companionRef := mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), constant.OverdraftBalanceKey)
		if companion, ok := balances[companionRef]; ok {
			companionContext := newOperationRecordSpec(input, leg, companion, postingRef, originRef, side, constant.OVERDRAFT, item.operationDirection, routeID, amount.Value, OperationRecordStandard)
			companionContext.Role = accounting.RoleOverdraftCompanion
			companionContext.ChartOfAccounts = ""
			companionContext.Metadata = map[string]any{}
			companionContext.RouteCode, companionContext.RouteDescription = translationRubric(input.RouteCache, routeID, constant.ActionOverdraft, item.operationDirection)
			*projection = append(*projection, companionContext)
		}
	}

	return nil
}

func newOperationRecordSpec(input EngineTranslationInput, leg mtransaction.FromTo, balance *mmodel.Balance, postingRef, originRef, side, rowType, direction, routeID string, requestedAmount decimal.Decimal, compatibilityPath string) OperationRecordSpec {
	description := leg.Description
	if description == "" {
		description = input.TransactionInput.Description
	}

	var stableRouteID *string

	if routeID != "" {
		value := routeID
		stableRouteID = &value
	}

	routeCode, routeDescription := translationRubric(input.RouteCache, routeID, input.Action, direction)

	stableBalance := cloneTranslationBalance(balance)

	return OperationRecordSpec{
		TransactionID: input.TransactionID, PostingRef: postingRef, OriginRef: originRef,
		BalanceRef: mtransaction.AliasKey(stableBalance.Alias, stableBalance.Key),
		Role:       accounting.RolePrimary, Side: side, RowType: rowType, Direction: direction,
		Description: description, RouteID: stableRouteID, RouteCode: routeCode, RouteDescription: routeDescription,
		ChartOfAccounts: leg.ChartOfAccounts, Metadata: maps.Clone(leg.Metadata), Balance: stableBalance,
		RequestedAmount: requestedAmount, CompatibilityPath: compatibilityPath,
	}
}

func indexTranslationBalances(balances []*mmodel.Balance) (map[string]*mmodel.Balance, error) {
	indexed := make(map[string]*mmodel.Balance, len(balances))
	for _, balance := range balances {
		if balance == nil {
			return nil, invalidEngineTranslation("nil balance in scoped pool")
		}

		ref := mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), balance.Key)
		if _, duplicate := indexed[ref]; duplicate {
			return nil, invalidEngineTranslation("duplicate logical balance in scoped pool")
		}

		indexed[ref] = balance
	}

	return indexed, nil
}

func translationRouteID(validate *mtransaction.Responses, leg mtransaction.FromTo, side string) string {
	var routeID string
	if side == OperationSpecSideFrom {
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

func cloneTranslationBalance(balance *mmodel.Balance) OperationBalanceContext {
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

	return OperationBalanceContext(cloned)
}

func invalidEngineTranslation(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidEngineTranslation, message)
}
