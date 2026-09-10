// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// postingPlan describes accounting intent without carrying live monetary state.
// The engine alone derives balances, overdraft movements, and versions from the
// state read during its atomic execution.
type postingPlan struct {
	items []postingPlanItem
}

type postingPlanItem struct {
	postingType             accounting.PostingType
	operationRowType        string
	operationDirection      string
	operationProjectionMode string
	historicalOverdraftCap  decimal.Decimal
	allowsOverdraftDraw     bool
	mayAffectOverdraft      bool
}

func buildPostingPlan(action, status, side string, routeValidationEnabled bool, historicalOverdraftCap decimal.Decimal) (postingPlan, error) {
	switch action {
	case constant.ActionDirect, constant.ActionRevert:
		if status != constant.CREATED {
			return postingPlan{}, invalidBalanceEngineTranslation("direct or revert action requires created status")
		}

		return buildConclusivePostingPlan(historicalOverdraftCap, side), nil
	case constant.ActionHold:
		if status != constant.PENDING {
			return postingPlan{}, invalidBalanceEngineTranslation("hold action requires pending status")
		}
	case constant.ActionCommit:
		if status != constant.APPROVED {
			return postingPlan{}, invalidBalanceEngineTranslation("commit action requires approved status")
		}
	case constant.ActionCancel:
		if status != constant.CANCELED {
			return postingPlan{}, invalidBalanceEngineTranslation("cancel action requires canceled status")
		}
	default:
		return postingPlan{}, invalidBalanceEngineTranslation("unsupported transaction action")
	}

	if routeValidationEnabled {
		return buildRouteValidatedPostingPlan(action, historicalOverdraftCap, side), nil
	}

	return buildPostingPlanWithoutRouteValidation(action, historicalOverdraftCap, side), nil
}

func buildConclusivePostingPlan(historicalOverdraftCap decimal.Decimal, side string) postingPlan {
	if side == OperationSpecSideFrom {
		return postingPlan{items: []postingPlanItem{{
			postingType:             accounting.PostingDebit,
			operationRowType:        constant.DEBIT,
			operationDirection:      constant.DirectionDebit,
			operationProjectionMode: OperationRecordStandard,
			allowsOverdraftDraw:     true,
			mayAffectOverdraft:      true,
		}}}
	}

	return postingPlan{items: []postingPlanItem{{
		postingType:             accounting.PostingCredit,
		operationRowType:        constant.CREDIT,
		operationDirection:      constant.DirectionCredit,
		operationProjectionMode: OperationRecordStandard,
		historicalOverdraftCap:  historicalOverdraftCap,
		mayAffectOverdraft:      true,
	}}}
}

func buildPostingPlanWithoutRouteValidation(action string, historicalOverdraftCap decimal.Decimal, side string) postingPlan {
	switch action {
	case constant.ActionHold:
		if side == OperationSpecSideTo {
			return postingPlan{}
		}

		return postingPlan{items: []postingPlanItem{{
			postingType:             accounting.PostingHold,
			operationRowType:        constant.ONHOLD,
			operationDirection:      constant.DirectionDebit,
			operationProjectionMode: OperationRecordStandard,
		}}}
	case constant.ActionCommit:
		if side == OperationSpecSideTo {
			return buildConclusivePostingPlan(historicalOverdraftCap, side)
		}

		return postingPlan{items: []postingPlanItem{{
			postingType:             accounting.PostingUnreserve,
			operationRowType:        constant.DEBIT,
			operationDirection:      constant.DirectionDebit,
			operationProjectionMode: OperationRecordStandard,
		}}}
	case constant.ActionCancel:
		if side == OperationSpecSideTo {
			return postingPlan{}
		}

		return postingPlan{items: []postingPlanItem{{
			postingType:             accounting.PostingRelease,
			operationRowType:        constant.RELEASE,
			operationDirection:      constant.DirectionCredit,
			operationProjectionMode: OperationRecordStandard,
			historicalOverdraftCap:  historicalOverdraftCap,
			mayAffectOverdraft:      historicalOverdraftCap.IsPositive(),
		}}}
	default:
		return postingPlan{}
	}
}

func buildRouteValidatedPostingPlan(action string, historicalOverdraftCap decimal.Decimal, side string) postingPlan {
	switch action {
	case constant.ActionHold:
		if side == OperationSpecSideTo {
			return postingPlan{}
		}

		return postingPlan{items: []postingPlanItem{
			{
				postingType:             accounting.PostingDebit,
				operationRowType:        constant.DEBIT,
				operationDirection:      constant.DirectionDebit,
				operationProjectionMode: OperationRecordValidatedHoldDebit,
			},
			{
				postingType:             accounting.PostingReserve,
				operationRowType:        constant.ONHOLD,
				operationDirection:      constant.DirectionCredit,
				operationProjectionMode: OperationRecordValidatedHoldReserve,
			},
		}}
	case constant.ActionCommit:
		if side == OperationSpecSideTo {
			return buildConclusivePostingPlan(historicalOverdraftCap, side)
		}

		return postingPlan{items: []postingPlanItem{{
			postingType:             accounting.PostingUnreserve,
			operationRowType:        constant.ONHOLD,
			operationDirection:      constant.DirectionDebit,
			operationProjectionMode: OperationRecordStandard,
		}}}
	case constant.ActionCancel:
		if side == OperationSpecSideTo {
			return postingPlan{}
		}

		return postingPlan{items: []postingPlanItem{
			{
				postingType:             accounting.PostingUnreserve,
				operationRowType:        constant.RELEASE,
				operationDirection:      constant.DirectionDebit,
				operationProjectionMode: OperationRecordValidatedCancelRelease,
			},
			{
				postingType:             accounting.PostingCredit,
				operationRowType:        constant.CREDIT,
				operationDirection:      constant.DirectionCredit,
				operationProjectionMode: OperationRecordValidatedCancelCredit,
				historicalOverdraftCap:  historicalOverdraftCap,
				mayAffectOverdraft:      true,
			},
		}}
	default:
		return postingPlan{}
	}
}
