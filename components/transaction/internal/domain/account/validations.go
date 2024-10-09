package account

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"math"
	"strconv"
	"strings"

	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
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
func ValidateAccounts(validate Responses, accounts []*account.Account) error {
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

// TranslateScale Function to translate value based on scale: (V * 10^-S)
func TranslateScale(value, scale string) (float64, error) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	s, err := strconv.Atoi(scale)
	if err != nil {
		return 0, err
	}
	return v * math.Pow(10, float64(-s)), nil
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
			Scale: "0",
			Asset: asset,
		}

		if ft.Share != nil {
			share := ft.Share.Percentage
			of := ft.Share.PercentageOfPercentage
			if of == 0 {
				of = 100
			}
			shareValue := totalSend * (float64(share) / float64(of))
			amount.Value = strconv.FormatFloat(shareValue, 'f', 2, 64)
			total += shareValue
			remaining -= shareValue
		}

		if ft.Amount.Value != "" && ft.Amount.Scale != "" {
			amountValue, err := TranslateScale(ft.Amount.Value, ft.Amount.Scale)
			if err != nil {
				e <- err
			}

			if !common.IsNilOrEmpty(&ft.Amount.Asset) {
				amount.Asset = ft.Amount.Asset
			}

			amount.Scale = ft.Amount.Scale
			amount.Value = strconv.FormatFloat(amountValue, 'f', 2, 64)
			total += amountValue
			remaining -= amountValue
		}

		response.SD = append(response.SD, ft.Account)
		response.FromTo[ft.Account] = amount
	}

	for i, source := range fromTos {
		if source.Remaining != "" {
			total += remaining

			amount := gold.Amount{
				Value: strconv.FormatFloat(remaining, 'f', 2, 64),
				Scale: "0",
				Asset: asset,
			}

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
