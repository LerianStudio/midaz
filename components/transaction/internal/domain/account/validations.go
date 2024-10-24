package account

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	a "github.com/LerianStudio/midaz/common/mgrpc/account"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"math"
	"strconv"
	"strings"
)

// Response return struct with total and per-account amounts
type Response struct {
	Total  float64
	SD     []string
	FromTo map[string]gold.Amount
}

// Responses return struct with total send and per-accounts
type Responses struct {
	Total        float64
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
		amt, ba, err := OperateAmounts(validate.From[ft.Account], acc.Balance, constant.DEBIT)
		if err != nil {
			return amount, balanceAfter, err
		}

		amount = amt

		balanceAfter = ba
	} else {
		amt, ba, err := OperateAmounts(validate.To[ft.Account], acc.Balance, constant.CREDIT)
		if err != nil {
			return amount, balanceAfter, err
		}

		amount = amt

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
				_, b, err := OperateAmounts(fromTo[key], acc.Balance, operation)
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

// Scale func scale: (V * 10^-S)
func Scale(v, s float64) float64 {
	return v * math.Pow(10, -s)
}

// UndoScale Function to undo the scale calculation
func UndoScale(value, scale float64) float64 {
	v := strconv.FormatFloat(value*math.Pow(10, scale), 'f', 0, 64)
	valueFinal, _ := strconv.ParseFloat(v, 64)

	return valueFinal
}

// FindScale Function to find the scale for any value of a value
func FindScale(asset string, value float64) gold.Amount {
	absAmount := math.Abs(value)

	percentageScale := math.Ceil(-math.Log10(absAmount))

	percentageValue := absAmount * math.Pow(10, percentageScale)

	if percentageValue != math.Trunc(percentageValue) {
		percentageValue = math.Trunc(percentageValue * 10)
		percentageScale++
	}

	amount := gold.Amount{
		Asset: asset,
		Value: strconv.Itoa(int(percentageValue)),
		Scale: strconv.Itoa(int(percentageScale)),
	}

	return amount
}

// OperateAmounts Function to sum or sub two amounts and normalize the scale
func OperateAmounts(amount gold.Amount, balance *a.Balance, operation string) (o.Amount, o.Balance, error) {
	value, err := strconv.ParseFloat(amount.Value, 64)
	if err != nil {
		return o.Amount{}, o.Balance{}, err
	}

	scale, err := strconv.ParseFloat(amount.Scale, 64)
	if err != nil {
		return o.Amount{}, o.Balance{}, err
	}

	amt := o.Amount{
		Amount: &value,
		Scale:  &scale,
	}

	var total float64

	switch operation {
	case constant.DEBIT:
		total = Scale(balance.Available, balance.Scale) - Scale(value, scale)
	default:
		total = Scale(balance.Available, balance.Scale) + Scale(value, scale)
	}

	s := balance.Scale
	if int(balance.Scale) < int(scale) {
		s = scale
	}

	undo := UndoScale(total, s)
	b := o.Balance{
		Available: &undo,
		OnHold:    &balance.OnHold,
		Scale:     &s,
	}

	return amt, b, nil
}

// calculateTotal Calculate total for sources/destinations based on shares, amounts and remains
func calculateTotal(fromTos []gold.FromTo, totalSend, scale float64, asset string, result chan *Response, e chan error) {
	total := 0.0
	response := Response{
		Total:  0.0,
		FromTo: make(map[string]gold.Amount),
		SD:     make([]string, 0),
	}

	totalSendScale := Scale(totalSend, scale)
	remaining := totalSend
	if scale > 1.0 {
		remaining = totalSendScale
	}

	for i := range fromTos {
		if fromTos[i].Remaining != "" {
			continue
		}

		amount := gold.Amount{}

		if fromTos[i].Share.Percentage != 0 {
			percentage := fromTos[i].Share.Percentage

			percentageOfPercentage := fromTos[i].Share.PercentageOfPercentage
			if percentageOfPercentage == 0 {
				percentageOfPercentage = 100
			}

			var shareValue = 0.0
			if scale > 1.0 {
				shareValue = totalSendScale * (float64(percentage) / float64(percentageOfPercentage))
				amount = FindScale(asset, shareValue)
			} else {
				shareValue = totalSend * (float64(percentage) / float64(percentageOfPercentage))
				amount.Asset = asset
				amount.Scale = strconv.Itoa(int(scale))
				amount.Value = strconv.Itoa(int(shareValue))
			}

			total += shareValue

			remaining -= shareValue
		}

		if !common.IsNilOrEmpty(&fromTos[i].Amount.Value) && !common.IsNilOrEmpty(&fromTos[i].Amount.Scale) {
			amount = gold.Amount{
				Asset: asset,
				Scale: fromTos[i].Amount.Scale,
				Value: fromTos[i].Amount.Value,
			}

			if !common.IsNilOrEmpty(&fromTos[i].Amount.Asset) {
				amount.Asset = fromTos[i].Amount.Asset
			}

			amountValue, err := strconv.ParseFloat(fromTos[i].Amount.Value, 64)
			if err != nil {
				e <- err
			}

			amountScale, err := strconv.ParseFloat(fromTos[i].Amount.Scale, 64)
			if err != nil {
				e <- err
			}

			if scale > 1.0 {
				amountValue = Scale(amountValue, amountScale)
			}

			total += amountValue
			remaining -= amountValue
		}

		response.SD = append(response.SD, fromTos[i].Account)
		response.FromTo[fromTos[i].Account] = amount
	}

	for i, tos := range fromTos {
		if tos.Remaining != "" {
			total += remaining

			response.SD = append(response.SD, tos.Account)

			amount := gold.Amount{}
			if scale > 1.0 {
				amount = FindScale(asset, remaining)
			} else {
				amount.Asset = asset
				amount.Scale = strconv.Itoa(int(scale))
				amount.Value = strconv.Itoa(int(remaining))
			}

			response.FromTo[tos.Account] = amount
			fromTos[i].Amount = &amount

			break
		}
	}

	response.Total = total
	result <- &response
}

// ValidateSendSourceAndDistribute Validate send and distribute totals
func ValidateSendSourceAndDistribute(transaction gold.Transaction) (*Responses, error) {
	response := &Responses{
		Total:        0.0,
		From:         make(map[string]gold.Amount),
		To:           make(map[string]gold.Amount),
		Sources:      make([]string, 0),
		Destinations: make([]string, 0),
		Aliases:      make([]string, 0),
	}

	send, err := strconv.ParseFloat(transaction.Send.Value, 64)
	if err != nil {
		return nil, err
	}

	scale, err := strconv.ParseFloat(transaction.Send.Scale, 64)
	if err != nil {
		return nil, err
	}

	response.Total = send

	e := make(chan error)
	result := make(chan *Response)

	var sourcesTotal float64

	var destinationsTotal float64

	go calculateTotal(transaction.Send.Source.From, send, scale, transaction.Send.Asset, result, e)
	select {
	case from := <-result:
		sourcesTotal = from.Total
		response.From = from.FromTo
		response.Sources = from.SD
		response.Aliases = append(response.Aliases, from.SD...)
	case err := <-e:
		return nil, err
	}

	go calculateTotal(transaction.Distribute.To, send, scale, transaction.Send.Asset, result, e)
	select {
	case from := <-result:
		destinationsTotal = from.Total
		response.To = from.FromTo
		response.Destinations = from.SD
		response.Aliases = append(response.Aliases, from.SD...)
	case err := <-e:
		return nil, err
	}

	total := 0.0
	if scale > 1.0 {
		total = Scale(send, scale)
	} else {
		total = send
	}

	if math.Abs(sourcesTotal-total) > 0.00001 || math.Abs(destinationsTotal-total) > 0.00001 {
		return nil, common.ValidationError{
			Code:    "0018",
			Title:   "Insufficient Funds",
			Message: "The transaction could not be completed due to insufficient funds in the account. Please add funds to your account and try again.",
		}
	}

	return response, nil
}
