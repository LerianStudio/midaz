package account

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/common"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	a "github.com/LerianStudio/midaz/common/mgrpc/account"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
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
	for _, acc := range accounts {

		if acc.Balance.Available == 0 {
			return common.ValidationError{
				Code:    "0025",
				Title:   "Insuficient balance",
				Message: strings.ReplaceAll("The account {Id} has insufficient balance. Try again sending the amount minor or equal to the available balance.", "{Id}", acc.Id),
			}
		}

		for key := range validate.From {
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

// Scale func scale: (V * 10^-S)
func Scale(v, s float64) float64 {
	return v * math.Pow(10, -s)
}

// UndoScale Function to undo the scale calculation
func UndoScale(v, s float64) float64 {
	return v * math.Pow(10, s)
}

// TranslateScale Function that translate string values to use scale
func TranslateScale(value, scale string) (float64, error) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}

	s, err := strconv.ParseFloat(scale, 64)
	if err != nil {
		return 0, err
	}

	return Scale(v, s), nil
}

// FindScaleForPercentage Function to find the scale for a percentage of a value
func FindScaleForPercentage(value float64, share gold.Share) (gold.Amount, float64) {
	sh := share.Percentage
	of := share.PercentageOfPercentage
	if of == 0 {
		of = 100
	}

	shareValue := value * (float64(sh) / float64(of))
	scale := int(math.Ceil(math.Log10(shareValue)))

	amount := gold.Amount{
		Value: strconv.FormatFloat(shareValue, 'f', 2, 64),
		Scale: strconv.Itoa(scale),
	}

	return amount, shareValue
}

// CalculateRemainingFromSend Function to calculate the remaining value when previous values are known
func CalculateRemainingFromSend(asset string, remainingValue float64) gold.Amount {
	remainingScale := int(math.Ceil(-math.Log10(remainingValue)))
	normalizedRemainingValue := UndoScale(remainingValue, float64(remainingScale))

	acc := gold.Amount{
		Asset: asset,
		Value: fmt.Sprintf("%.0f", normalizedRemainingValue),
		Scale: strconv.Itoa(remainingScale),
	}

	return acc
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
	case "sub":
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
func calculateTotal(fromTos []gold.FromTo, totalSend float64, asset string, result chan *Response, e chan error) {
	total := 0.0
	remaining := totalSend
	response := Response{
		Total:  0.0,
		FromTo: make(map[string]gold.Amount),
		SD:     make([]string, 0),
	}

	for _, ft := range fromTos {
		if ft.Remaining != "" {
			continue
		}

		amount := gold.Amount{
			Asset: asset,
		}

		if ft.Share.Percentage != 0 {
			amt, shareValue := FindScaleForPercentage(totalSend, *ft.Share)
			amount.Value = amt.Value
			amount.Scale = amt.Scale

			total += shareValue
			remaining -= shareValue
		}

		if !common.IsNilOrEmpty(&ft.Amount.Value) && !common.IsNilOrEmpty(&ft.Amount.Scale) {
			amountValue, err := TranslateScale(ft.Amount.Value, ft.Amount.Scale)
			if err != nil {
				e <- err
			}

			amount.Scale = ft.Amount.Scale
			amount.Value = strconv.FormatFloat(amountValue, 'f', 2, 64)
			if !common.IsNilOrEmpty(&ft.Amount.Asset) {
				amount.Asset = ft.Amount.Asset
			}

			total += amountValue
			remaining -= amountValue
		}

		response.SD = append(response.SD, ft.Account)
		response.FromTo[ft.Account] = amount
	}

	for i, source := range fromTos {
		if source.Remaining != "" {
			total += remaining

			amount := CalculateRemainingFromSend(asset, remaining)

			response.SD = append(response.SD, source.Account)
			response.FromTo[source.Account] = amount
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

	totalSend, err := TranslateScale(transaction.Send.Value, transaction.Send.Scale)
	if err != nil {
		return nil, err
	}
	response.Total = totalSend

	e := make(chan error)
	result := make(chan *Response)
	sourcesTotal := 0.0
	destinationsTotal := 0.0

	go calculateTotal(transaction.Send.Source.From, totalSend, transaction.Send.Asset, result, e)
	select {
	case from := <-result:
		sourcesTotal = from.Total
		response.From = from.FromTo
		response.Sources = from.SD
		response.Aliases = append(response.Aliases, from.SD...)
	case err := <-e:
		return nil, err
	}

	go calculateTotal(transaction.Distribute.To, totalSend, transaction.Send.Asset, result, e)
	select {
	case from := <-result:
		destinationsTotal = from.Total
		response.To = from.FromTo
		response.Destinations = from.SD
		response.Aliases = append(response.Aliases, from.SD...)
	case err := <-e:
		return nil, err
	}

	if math.Abs(sourcesTotal-totalSend) > 0.00001 || math.Abs(destinationsTotal-totalSend) > 0.00001 {
		return nil, common.ValidationError{
			Code:    "0018",
			Title:   "Insufficient Funds",
			Message: "The transaction could not be completed due to insufficient funds in the account. Please add funds to your account and try again.",
		}
	}

	return response, nil
}
