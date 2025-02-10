package model

import (
	"context"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"math"
	"math/big"
	"strings"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
)

// ValidateBalancesRules function with some validates in accounts and DSL operations
func ValidateBalancesRules(ctx context.Context, transaction Transaction, validate Responses, balances []*mmodel.Balance) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	_, spanValidateBalances := tracer.Start(ctx, "validations.validate_balances_rules")

	if len(balances) != (len(validate.From) + len(validate.To)) {
		err := pkg.ValidateBusinessError(constant.ErrAccountIneligibility, "ValidateAccounts")

		mopentelemetry.HandleSpanError(&spanValidateBalances, "validations.validate_balances_rules", err)

		return err
	}

	for _, balance := range balances {
		if err := validateFromBalances(balance, validate.From, validate.Asset); err != nil {
			mopentelemetry.HandleSpanError(&spanValidateBalances, "validations.validate_from_balances_", err)

			logger.Errorf("validations.validate_from_balances_err: %s", err)

			return err
		}

		if err := validateToBalances(balance, validate.To, validate.Asset); err != nil {
			mopentelemetry.HandleSpanError(&spanValidateBalances, "validations.validate_to_balances_", err)

			logger.Errorf("validations.validate_to_balances_err: %s", err)

			return err
		}

		if err := validateBalance(balance, transaction, validate.From); err != nil {
			mopentelemetry.HandleSpanError(&spanValidateBalances, "validations.validate_to_balances_", err)

			logger.Errorf("validations.validate_balances_err: %s", err)

			return err
		}
	}

	spanValidateBalances.End()

	return nil
}

func validateBalance(balance *mmodel.Balance, dsl Transaction, from map[string]Amount) error {
	for key := range from {
		for _, f := range dsl.Send.Source.From {
			if balance.ID == key || balance.Alias == key {
				blc := Balance{
					Scale:     balance.Scale,
					Available: balance.Available,
					OnHold:    balance.OnHold,
				}

				ba := OperateBalances(from[f.Account], blc, constant.DEBIT)

				if ba.Available < 0 && balance.AccountType != constant.ExternalAccountType {
					return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance", balance.Alias)
				}
			}
		}
	}

	return nil
}

func validateFromBalances(balance *mmodel.Balance, from map[string]Amount, asset string) error {
	for key := range from {
		if balance.ID == key || balance.Alias == key {
			if balance.AssetCode != asset {
				return pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateFromAccounts")
			}

			if !balance.AllowSending {
				return pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateFromAccounts")
			}

			if balance.Available <= 0 && balance.AccountType != constant.ExternalAccountType {
				return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateFromAccounts", balance.Alias)
			}
		}
	}

	return nil
}

func validateToBalances(balance *mmodel.Balance, to map[string]Amount, asset string) error {
	for key := range to {
		if balance.ID == key || balance.Alias == key {
			if balance.AssetCode != asset {
				return pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateToAccounts")
			}

			if !balance.AllowReceiving {
				return pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateToAccounts")
			}

			if balance.Available > 0 && balance.AccountType == constant.ExternalAccountType {
				return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateToAccounts", balance.Alias)
			}
		}
	}

	return nil
}

// ValidateFromToOperation func that validate operate balance
func ValidateFromToOperation(ft FromTo, validate Responses, balance *mmodel.Balance) (Amount, Balance, error) {
	amount := Amount{}

	balanceAfter := Balance{}

	if ft.IsFrom {
		blc := Balance{
			Scale:     balance.Scale,
			Available: balance.Available,
			OnHold:    balance.OnHold,
		}

		ba := OperateBalances(validate.From[ft.Account], blc, constant.DEBIT)

		if ba.Available < 0 && balance.AccountType != constant.ExternalAccountType {
			return amount, balanceAfter, pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "ValidateFromToOperation", balance.Alias)
		}

		amount = Amount{
			Value: validate.From[ft.Account].Value,
			Scale: validate.From[ft.Account].Scale,
		}

		balanceAfter = ba
	} else {
		blc := Balance{
			Scale:     balance.Scale,
			Available: balance.Available,
			OnHold:    balance.OnHold,
		}

		ba := OperateBalances(validate.To[ft.Account], blc, constant.CREDIT)
		amount = Amount{
			Value: validate.To[ft.Account].Value,
			Scale: validate.To[ft.Account].Scale,
		}

		balanceAfter = ba
	}

	return amount, balanceAfter, nil
}

// UpdateBalances function with some updates values in balances and
func UpdateBalances(operation string, fromTo map[string]Amount, balances []*mmodel.Balance, result chan []*mmodel.Balance) {
	newBalances := make([]*mmodel.Balance, 0)

	for _, balance := range balances {
		for key := range fromTo {
			if balance.ID == key || balance.Alias == key {
				blc := Balance{
					Scale:     balance.Scale,
					Available: balance.Available,
					OnHold:    balance.OnHold,
				}

				b := OperateBalances(fromTo[key], blc, operation)

				ac := mmodel.Balance{
					ID:             balance.ID,
					Alias:          balance.Alias,
					OrganizationID: balance.OrganizationID,
					LedgerID:       balance.LedgerID,
					AssetCode:      balance.AssetCode,
					Available:      b.Available,
					Scale:          b.Scale,
					OnHold:         b.OnHold,
					AllowSending:   balance.AllowSending,
					AllowReceiving: balance.AllowReceiving,
					AccountType:    balance.AccountType,
					Version:        balance.Version,
					CreatedAt:      balance.CreatedAt,
					UpdatedAt:      balance.UpdatedAt,
				}

				newBalances = append(newBalances, &ac)

				break
			}
		}
	}

	result <- newBalances
}

// Scale func scale: (V * 10^ (S0-S1))
func Scale(v, s0, s1 int64) int64 {
	return int64(float64(v) * math.Pow(10, float64(s1)-float64(s0)))
}

// UndoScale Function to undo the scale calculation
func UndoScale(v float64, s int64) int64 {
	return int64(v * math.Pow(10, float64(s)))
}

// FindScale Function to find the scale for any value of a value
func FindScale(asset string, v float64, s int64) Amount {
	valueString := big.NewFloat(v).String()
	parts := strings.Split(valueString, ".")

	scale := s
	value := int64(v)

	if len(parts) > 1 {
		scale = int64(len(parts[1]))
		value = UndoScale(v, scale)

		if parts[1] != "0" {
			scale += s
		}
	}

	amount := Amount{
		Asset: asset,
		Value: value,
		Scale: scale,
	}

	return amount
}

// normalize func that normalize scale from all values
func normalize(total, amount, remaining *Amount) {
	if total.Scale < amount.Scale {
		if total.Value != 0 {
			v0 := Scale(total.Value, total.Scale, amount.Scale)

			total.Value = v0 + amount.Value
		} else {
			total.Value += amount.Value
		}

		total.Scale = amount.Scale
	} else {
		if total.Value != 0 {
			v0 := Scale(amount.Value, amount.Scale, total.Scale)

			total.Value += v0

			amount.Value = v0
			amount.Scale = total.Scale
		} else {
			total.Value += amount.Value
			total.Scale = amount.Scale
		}
	}

	if remaining.Scale < amount.Scale {
		v0 := Scale(remaining.Value, remaining.Scale, amount.Scale)

		remaining.Value = v0 - amount.Value
		remaining.Scale = amount.Scale
	} else {
		v0 := Scale(amount.Value, amount.Scale, remaining.Scale)

		remaining.Value -= v0
	}
}

// OperateBalances Function to sum or sub two balances and normalize the scale
func OperateBalances(amount Amount, balance Balance, operation string) Balance {
	var (
		scale int64
		total int64
	)

	switch operation {
	case constant.DEBIT:
		if balance.Scale < amount.Scale {
			v0 := Scale(balance.Available, balance.Scale, amount.Scale)
			total = v0 - amount.Value
			scale = amount.Scale
		} else {
			v0 := Scale(amount.Value, amount.Scale, balance.Scale)
			total = balance.Available - v0
			scale = balance.Scale
		}
	default:
		if balance.Scale < amount.Scale {
			v0 := Scale(balance.Available, balance.Scale, amount.Scale)
			total = v0 + amount.Value
			scale = amount.Scale
		} else {
			v0 := Scale(amount.Value, amount.Scale, balance.Scale)
			total = balance.Available + v0
			scale = balance.Scale
		}
	}

	blc := Balance{
		Available: total,
		OnHold:    balance.OnHold,
		Scale:     scale,
	}

	return blc
}

// calculateTotal Calculate total for sources/destinations based on shares, amounts and remains
func calculateTotal(fromTos []FromTo, send Send, t chan int64, ft chan map[string]Amount, sd chan []string) {
	fmto := make(map[string]Amount)
	scdt := make([]string, 0)

	total := Amount{
		Asset: send.Asset,
		Scale: 0,
		Value: 0,
	}

	remaining := Amount{
		Asset: send.Asset,
		Scale: send.Scale,
		Value: send.Value,
	}

	for i := range fromTos {
		if fromTos[i].Share != nil && fromTos[i].Share.Percentage != 0 {
			percentage := fromTos[i].Share.Percentage

			percentageOfPercentage := fromTos[i].Share.PercentageOfPercentage
			if percentageOfPercentage == 0 {
				percentageOfPercentage = 100
			}

			shareValue := float64(send.Value) * (float64(percentage) / float64(percentageOfPercentage))
			amount := FindScale(send.Asset, shareValue, send.Scale)

			normalize(&total, &amount, &remaining)
			fmto[fromTos[i].Account] = amount
		}

		if fromTos[i].Amount != nil && fromTos[i].Amount.Value > 0 && fromTos[i].Amount.Scale > -1 {
			amount := Amount{
				Asset: fromTos[i].Amount.Asset,
				Scale: fromTos[i].Amount.Scale,
				Value: fromTos[i].Amount.Value,
			}

			normalize(&total, &amount, &remaining)
			fmto[fromTos[i].Account] = amount
		}

		if !pkg.IsNilOrEmpty(&fromTos[i].Remaining) {
			total.Value += remaining.Value

			fmto[fromTos[i].Account] = remaining
			fromTos[i].Amount = &remaining
		}

		scdt = append(scdt, fromTos[i].Account)
	}

	ttl := total.Value
	if total.Scale > send.Scale {
		ttl = Scale(total.Value, total.Scale, send.Scale)
	}

	t <- ttl
	ft <- fmto
	sd <- scdt
}

// ValidateSendSourceAndDistribute Validate send and distribute totals
func ValidateSendSourceAndDistribute(transaction Transaction) (*Responses, error) {
	response := &Responses{
		Total:        transaction.Send.Value,
		Asset:        transaction.Send.Asset,
		From:         make(map[string]Amount),
		To:           make(map[string]Amount),
		Sources:      make([]string, 0),
		Destinations: make([]string, 0),
		Aliases:      make([]string, 0),
	}

	var (
		sourcesTotal      int64
		destinationsTotal int64
	)

	t := make(chan int64)
	ft := make(chan map[string]Amount)
	sd := make(chan []string)

	go calculateTotal(transaction.Send.Source.From, transaction.Send, t, ft, sd)
	sourcesTotal = <-t
	response.From = <-ft
	response.Sources = <-sd
	response.Aliases = append(response.Aliases, response.Sources...)

	go calculateTotal(transaction.Send.Distribute.To, transaction.Send, t, ft, sd)
	destinationsTotal = <-t
	response.To = <-ft
	response.Destinations = <-sd
	response.Aliases = append(response.Aliases, response.Destinations...)

	for _, source := range response.Sources {
		if _, ok := response.To[source]; ok {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		}
	}

	for _, destination := range response.Destinations {
		if _, ok := response.From[destination]; ok {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		}
	}

	if math.Abs(float64(response.Total)-float64(sourcesTotal)) != 0 {
		return nil, pkg.ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
	}

	if math.Abs(float64(sourcesTotal)-float64(destinationsTotal)) != 0 {
		return nil, pkg.ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
	}

	return response, nil
}
