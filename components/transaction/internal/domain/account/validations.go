package account

import (
	"math"
	"math/big"
	"strings"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	a "github.com/LerianStudio/midaz/common/mgrpc/account"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
)

// Response return struct with total and per-account amounts
type Response struct {
	Total  int
	SD     []string
	FromTo map[string]gold.Amount
}

// Responses return struct with total send and per-accounts
type Responses struct {
	Total        int
	From         map[string]gold.Amount
	To           map[string]gold.Amount
	Sources      []string
	Destinations []string
	Aliases      []string
}

// ValidateAccounts function with some validates in accounts and DSL operations
func ValidateAccounts(validate Responses, accounts []*a.Account) error {
	if len(accounts) != (len(validate.From) + len(validate.To)) {
		return common.ValidationError{
			Code:    "0019",
			Title:   "Transaction Participation Error",
			Message: "One or more accounts listed in the transaction statement are ineligible to participate. Please review the account statuses and try again.",
		}
	}

	for _, acc := range accounts {
		for key := range validate.From {
			if acc.Id == key || acc.Alias == key && acc.Status.AllowSending {
				if acc.Balance.Available <= 0 && !strings.Contains(acc.Alias, "@external") {
					return common.ValidationError{
						Code:    "0025",
						Title:   "Insuficient balance",
						Message: strings.ReplaceAll("The account {Id} has insufficient balance. Try again sending the amount minor or equal to the available balance.", "{Id}", acc.Alias),
					}
				}
			}

			if acc.Id == key || acc.Alias == key && !acc.Status.AllowSending {
				return common.ValidationError{
					Code:    "0019",
					Title:   "Transaction Participation Error",
					Message: "One or more accounts listed in the transaction statement are ineligible to participate. Please review the account statuses and try again.",
				}
			}
		}

		for key := range validate.To {
			if acc.Id == key || acc.Alias == key && !acc.Status.AllowReceiving {
				return common.ValidationError{
					Code:    "0019",
					Title:   "Transaction Participation Error",
					Message: "One or more accounts listed in the transaction statement are ineligible to participate. Please review the account statuses and try again.",
				}
			}
		}
	}

	return nil
}

// ValidateFromToOperation func that validate operate balance amount
func ValidateFromToOperation(ft gold.FromTo, validate Responses, acc *a.Account) (o.Amount, o.Balance, error) {
	amount := o.Amount{}

	balanceAfter := o.Balance{}

	if ft.IsFrom {
		ba, err := OperateAmounts(validate.From[ft.Account], acc.Balance, constant.DEBIT)
		if err != nil {
			return amount, balanceAfter, err
		}

		amt := float64(validate.From[ft.Account].Value)
		scl := float64(validate.From[ft.Account].Scale)
		amount = o.Amount{
			Amount: &amt,
			Scale:  &scl,
		}

		balanceAfter = ba
	} else {
		ba, err := OperateAmounts(validate.To[ft.Account], acc.Balance, constant.CREDIT)
		if err != nil {
			return amount, balanceAfter, err
		}

		amt := float64(validate.From[ft.Account].Value)
		scl := float64(validate.From[ft.Account].Scale)
		amount = o.Amount{
			Amount: &amt,
			Scale:  &scl,
		}

		balanceAfter = ba
	}

	return amount, balanceAfter, nil
}

// UpdateAccounts function with some updates values in accounts and
func UpdateAccounts(operation string, fromTo map[string]gold.Amount, accounts []*a.Account, result chan []*a.Account, e chan error) {
	accs := make([]*a.Account, 0)

	for _, acc := range accounts {
		for key := range fromTo {
			if acc.Id == key || acc.Alias == key {
				b, err := OperateAmounts(fromTo[key], acc.Balance, operation)
				if err != nil {
					e <- err
				}

				balance := a.Balance{
					Available: *b.Available,
					Scale:     *b.Scale,
					OnHold:    *b.OnHold,
				}

				status := a.Status{
					Code:           acc.Status.Code,
					Description:    acc.Status.Description,
					AllowSending:   acc.Status.AllowSending,
					AllowReceiving: acc.Status.AllowReceiving,
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
func FindScale(asset string, v float64, s int) gold.Amount {
	valueString := big.NewFloat(v).String()
	parts := strings.Split(valueString, ".")

	scale := len(parts[1])
	value := UndoScale(v, scale)

	if parts[1] != "0" {
		scale += s
	}

	amount := gold.Amount{
		Asset: asset,
		Value: value,
		Scale: scale,
	}

	return amount
}

// normalize func that normalize scale from all values
func normalize(total, amount, remaining *gold.Amount) {
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
func OperateAmounts(amount gold.Amount, balance *a.Balance, operation string) (o.Balance, error) {
	var scale float64

	var total float64

	switch operation {
	case constant.DEBIT:
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

	blc := o.Balance{
		Available: &total,
		OnHold:    &balance.OnHold,
		Scale:     &scale,
	}

	return blc, nil
}

// calculateTotal Calculate total for sources/destinations based on shares, amounts and remains
func calculateTotal(fromTos []gold.FromTo, send, scale int, asset string, result chan *Response) {
	response := Response{
		Total:  0,
		FromTo: make(map[string]gold.Amount),
		SD:     make([]string, 0),
	}

	total := gold.Amount{
		Asset: asset,
		Scale: 0,
		Value: 0,
	}

	remaining := gold.Amount{
		Asset: asset,
		Scale: scale,
		Value: send,
	}

	for i := range fromTos {
		if fromTos[i].Share.Percentage != 0 {
			percentage := fromTos[i].Share.Percentage

			percentageOfPercentage := fromTos[i].Share.PercentageOfPercentage
			if percentageOfPercentage == 0 {
				percentageOfPercentage = 100
			}

			shareValue := float64(send) * (float64(percentage) / float64(percentageOfPercentage))
			amount := FindScale(asset, shareValue, scale)

			normalize(&total, &amount, &remaining)
			response.FromTo[fromTos[i].Account] = amount
		}

		if fromTos[i].Amount.Value > 0 && fromTos[i].Amount.Scale > 0 {
			amount := gold.Amount{
				Asset: fromTos[i].Amount.Asset,
				Scale: fromTos[i].Amount.Scale,
				Value: fromTos[i].Amount.Value,
			}

			normalize(&total, &amount, &remaining)
			response.FromTo[fromTos[i].Account] = amount
		}

		if fromTos[i].Remaining != "" {
			total.Value += remaining.Value

			response.FromTo[fromTos[i].Account] = remaining
			fromTos[i].Amount = &remaining
		}

		response.SD = append(response.SD, fromTos[i].Account)
	}

	response.Total = total.Value
	if total.Scale > scale {
		response.Total = int(Scale(total.Value, total.Scale, scale))
	}

	result <- &response
}

// ValidateSendSourceAndDistribute Validate send and distribute totals
func ValidateSendSourceAndDistribute(transaction gold.Transaction) (*Responses, error) {
	response := &Responses{
		Total:        transaction.Send.Value,
		From:         make(map[string]gold.Amount),
		To:           make(map[string]gold.Amount),
		Sources:      make([]string, 0),
		Destinations: make([]string, 0),
		Aliases:      make([]string, 0),
	}

	var sourcesTotal int

	var destinationsTotal int

	result := make(chan *Response)

	go calculateTotal(transaction.Send.Source.From, transaction.Send.Value, transaction.Send.Scale, transaction.Send.Asset, result)
	from := <-result
	sourcesTotal = from.Total
	response.From = from.FromTo
	response.Sources = from.SD
	response.Aliases = append(response.Aliases, from.SD...)

	go calculateTotal(transaction.Distribute.To, transaction.Send.Value, transaction.Send.Scale, transaction.Send.Asset, result)
	to := <-result
	destinationsTotal = to.Total
	response.To = to.FromTo
	response.Destinations = to.SD
	response.Aliases = append(response.Aliases, to.SD...)

	if math.Abs(float64(sourcesTotal)-float64(destinationsTotal)) > 0.00001 {
		return nil, common.ValidationError{
			Code:    "0018",
			Title:   "Insufficient Funds",
			Message: "The transaction could not be completed due to insufficient funds in the account. Please add funds to your account and try again.",
		}
	}

	return response, nil
}
