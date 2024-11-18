package model

import (
	"math"
	"math/big"
	"strings"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	a "github.com/LerianStudio/midaz/common/mgrpc/account"
)

// ValidateAccounts function with some validates in accounts and DSL operations
func ValidateAccounts(validate Responses, accounts []*a.Account) error {
	if len(accounts) != (len(validate.From) + len(validate.To)) {
		return common.ValidateBusinessError(cn.ErrAccountIneligibility, "ValidateAccounts")
	}

	for _, acc := range accounts {
		for key := range validate.From {
			if acc.Id == key || acc.Alias == key && acc.AllowSending {
				if acc.Balance.Available <= 0 && !strings.Contains(acc.Alias, "@external") {
					return common.ValidateBusinessError(cn.ErrInsufficientAccountBalance, "ValidateAccounts", acc.Alias)
				}
			}

			if acc.Id == key || acc.Alias == key && !acc.AllowSending {
				return common.ValidateBusinessError(cn.ErrAccountIneligibility, "ValidateAccounts")
			}
		}

		for key := range validate.To {
			if acc.Id == key || acc.Alias == key && !acc.AllowReceiving {
				return common.ValidateBusinessError(cn.ErrAccountIneligibility, "ValidateAccounts")
			}
		}
	}

	return nil
}

// ValidateFromToOperation func that validate operate balance amount
func ValidateFromToOperation(ft FromTo, validate Responses, acc *a.Account) (Amount, Balance, error) {
	amount := Amount{}

	balanceAfter := Balance{}

	if ft.IsFrom {
		ba, err := OperateAmounts(validate.From[ft.Account], acc.Balance, cn.DEBIT)
		if err != nil {
			return amount, balanceAfter, err
		}

		amount = Amount{
			Value: validate.From[ft.Account].Value,
			Scale: validate.From[ft.Account].Scale,
		}

		balanceAfter = ba
	} else {
		ba, err := OperateAmounts(validate.To[ft.Account], acc.Balance, cn.CREDIT)
		if err != nil {
			return amount, balanceAfter, err
		}

		amount = Amount{
			Value: validate.To[ft.Account].Value,
			Scale: validate.To[ft.Account].Scale,
		}

		balanceAfter = ba
	}

	return amount, balanceAfter, nil
}

// UpdateAccounts function with some updates values in accounts and
func UpdateAccounts(operation string, fromTo map[string]Amount, accounts []*a.Account, result chan []*a.Account, e chan error) {
	accs := make([]*a.Account, 0)

	for _, acc := range accounts {
		for key := range fromTo {
			if acc.Id == key || acc.Alias == key {
				b, err := OperateAmounts(fromTo[key], acc.Balance, operation)
				if err != nil {
					e <- err
				}

				balance := a.Balance{
					Available: float64(b.Available),
					Scale:     float64(b.Scale),
					OnHold:    float64(b.OnHold),
				}

				status := a.Status{
					Code:        acc.Status.Code,
					Description: acc.Status.Description,
				}

				ac := a.Account{
					Id:              acc.Id,
					Alias:           acc.Alias,
					Name:            acc.Name,
					ParentAccountId: acc.ParentAccountId,
					EntityId:        acc.EntityId,
					OrganizationId:  acc.OrganizationId,
					LedgerId:        acc.LedgerId,
					PortfolioId:     acc.PortfolioId,
					ProductId:       acc.ProductId,
					AssetCode:       acc.AssetCode,
					Balance:         &balance,
					Status:          &status,
					AllowSending:    acc.AllowSending,
					AllowReceiving:  acc.AllowReceiving,
					Type:            acc.Type,
					CreatedAt:       acc.CreatedAt,
					UpdatedAt:       acc.UpdatedAt,
				}

				accs = append(accs, &ac)

				break
			}
		}
	}

	result <- accs
}

// Scale func scale: (V * 10^ (S0-S1))
func Scale(v, s0, s1 int) float64 {
	return float64(v) * math.Pow10(s1-s0)
}

// UndoScale Function to undo the scale calculation
func UndoScale(v float64, s int) int {
	return int(v * math.Pow10(s))
}

// FindScale Function to find the scale for any value of a value
func FindScale(asset string, v float64, s int) Amount {
	valueString := big.NewFloat(v).String()
	parts := strings.Split(valueString, ".")

	scale := s
	value := int(v)

	if len(parts) > 1 {
		scale = len(parts[1])
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

			total.Value = int(v0) + amount.Value
		} else {
			total.Value += amount.Value
		}

		total.Scale = amount.Scale
	} else {
		if total.Value != 0 {
			v0 := Scale(amount.Value, amount.Scale, total.Scale)

			total.Value += int(v0)

			amount.Value = int(v0)
			amount.Scale = total.Scale
		} else {
			total.Value += amount.Value
			total.Scale = amount.Scale
		}
	}

	if remaining.Scale < amount.Scale {
		v0 := Scale(remaining.Value, remaining.Scale, amount.Scale)

		remaining.Value = int(v0) - amount.Value
		remaining.Scale = amount.Scale
	} else {
		v0 := Scale(amount.Value, amount.Scale, remaining.Scale)

		remaining.Value -= int(v0)
	}
}

// OperateAmounts Function to sum or sub two amounts and normalize the scale
func OperateAmounts(amount Amount, balance *a.Balance, operation string) (Balance, error) {
	var scale float64

	var total float64

	switch operation {
	case cn.DEBIT:
		if int(balance.Scale) < amount.Scale {
			v0 := Scale(int(balance.Available), int(balance.Scale), amount.Scale)
			total = v0 - float64(amount.Value)
			scale = float64(amount.Scale)
		} else {
			v0 := Scale(amount.Value, amount.Scale, int(balance.Scale))
			total = balance.Available - v0
			scale = balance.Scale
		}
	default:
		if int(balance.Scale) < amount.Scale {
			v0 := Scale(int(balance.Available), int(balance.Scale), amount.Scale)
			total = v0 + float64(amount.Value)
			scale = float64(amount.Scale)
		} else {
			v0 := Scale(amount.Value, amount.Scale, int(balance.Scale))
			total = balance.Available + v0
			scale = balance.Scale
		}
	}

	blc := Balance{
		Available: int(total),
		OnHold:    int(balance.OnHold),
		Scale:     int(scale),
	}

	return blc, nil
}

// calculateTotal Calculate total for sources/destinations based on shares, amounts and remains
func calculateTotal(fromTos []FromTo, send Send, t chan int, ft chan map[string]Amount, sd chan []string) {
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
		if fromTos[i].Share.Percentage != 0 {
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

		if fromTos[i].Amount.Value > 0 && fromTos[i].Amount.Scale > 0 {
			amount := Amount{
				Asset: fromTos[i].Amount.Asset,
				Scale: fromTos[i].Amount.Scale,
				Value: fromTos[i].Amount.Value,
			}

			normalize(&total, &amount, &remaining)
			fmto[fromTos[i].Account] = amount
		}

		if fromTos[i].Remaining != "" {
			total.Value += remaining.Value

			fmto[fromTos[i].Account] = remaining
			fromTos[i].Amount = &remaining
		}

		scdt = append(scdt, fromTos[i].Account)
	}

	ttl := total.Value
	if total.Scale > send.Scale {
		ttl = int(Scale(total.Value, total.Scale, send.Scale))
	}

	t <- ttl
	ft <- fmto
	sd <- scdt
}

// ValidateSendSourceAndDistribute Validate send and distribute totals
func ValidateSendSourceAndDistribute(transaction Transaction) (*Responses, error) {
	response := &Responses{
		Total:        transaction.Send.Value,
		From:         make(map[string]Amount),
		To:           make(map[string]Amount),
		Sources:      make([]string, 0),
		Destinations: make([]string, 0),
		Aliases:      make([]string, 0),
	}

	var sourcesTotal int

	var destinationsTotal int

	t := make(chan int)
	ft := make(chan map[string]Amount)
	sd := make(chan []string)

	go calculateTotal(transaction.Send.Source.From, transaction.Send, t, ft, sd)
	sourcesTotal = <-t
	response.From = <-ft
	response.Sources = <-sd
	response.Aliases = append(response.Aliases, response.Sources...)

	go calculateTotal(transaction.Distribute.To, transaction.Send, t, ft, sd)
	destinationsTotal = <-t
	response.To = <-ft
	response.Destinations = <-sd
	response.Aliases = append(response.Aliases, response.Destinations...)

	if math.Abs(float64(sourcesTotal)-float64(destinationsTotal)) > 0.00001 {
		return nil, common.ValidateBusinessError(cn.ErrInsufficientFunds, "ValidateSendSourceAndDistribute")
	}

	return response, nil
}
