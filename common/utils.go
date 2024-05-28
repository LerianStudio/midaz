package common

import (
	"go.mongodb.org/mongo-driver/bson"
	"slices"
	"strconv"
	"strings"
)

// Contains checks if an item is in a slice. This function uses type parameters to work with any slice type.
func Contains[T comparable](slice []T, item T) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}

	return false
}

// CheckMetadataKeyAndValueLength check the length of key and value to a limit pass by on field limit
func CheckMetadataKeyAndValueLength(limit int, metadata map[string]any) error {
	for k, v := range metadata {
		if len(k) > limit {
			return ValidationError{
				Message: "Error the key: " + k + " must be less than 100 characters",
			}
		}

		var value string
		switch t := v.(type) {
		case int:
			value = strconv.Itoa(t)
		case float64:
			value = strconv.FormatFloat(t, 'f', -1, 64)
		case string:
			value = t
		case bool:
			value = strconv.FormatBool(t)
		}

		if len(value) > limit {
			return ValidationError{
				Message: "Error the value: " + value + " must be less than 100 characters",
			}
		}
	}

	return nil
}

// ValidateCountryAddress validate if country in object address contains in countries list using ISO 3166-1 alpha-2
func ValidateCountryAddress(country string) error {
	countries := []string{
		"AD", "AE", "AF", "AG", "AI", "AL", "AM", "AO", "AQ", "AR", "AS", "AT", "AU", "AW", "AX", "AZ",
		"BA", "BB", "BD", "BE", "BF", "BG", "BH", "BI", "BJ", "BL", "BM", "BN", "BO", "BQ", "BR", "BS", "BT", "BV", "BW",
		"BY", "BZ", "CA", "CC", "CD", "CF", "CG", "CH", "CI", "CK", "CL", "CM", "CN", "CO", "CR", "CU", "CV", "CW", "CX",
		"CY", "CZ", "DE", "DJ", "DK", "DM", "DO", "DZ", "EC", "EE", "EG", "EH", "ER", "ES", "ET", "FI", "FJ", "FK", "FM",
		"FO", "FR", "GA", "GB", "GD", "GE", "GF", "GG", "GH", "GI", "GL", "GM", "GN", "GP", "GQ", "GR", "GS", "GT", "GU",
		"GW", "GY", "HK", "HM", "HN", "HR", "HT", "HU", "ID", "IE", "IL", "IM", "IN", "IO", "IQ", "IR", "IS", "IT", "JE",
		"JM", "JO", "JP", "KE", "KG", "KH", "KI", "KM", "KN", "KP", "KR", "KW", "KY", "KZ", "LA", "LB", "LC", "LI", "LK",
		"LR", "LS", "LT", "LU", "LV", "LY", "MA", "MC", "MD", "ME", "MF", "MG", "MH", "MK", "ML", "MM", "MN", "MO", "MP",
		"MQ", "MR", "MS", "MT", "MU", "MV", "MW", "MX", "MY", "MZ", "NA", "NC", "NE", "NF", "NG", "NI", "NL", "NO", "NP",
		"NR", "NU", "NZ", "OM", "PA", "PE", "PF", "PG", "PH", "PK", "PL", "PM", "PN", "PR", "PS", "PT", "PW", "PY", "QA",
		"RE", "RO", "RS", "RU", "RW", "SA", "SB", "SC", "SD", "SE", "SG", "SH", "SI", "SJ", "SK", "SL", "SM", "SN", "SO",
		"SR", "SS", "ST", "SV", "SX", "SY", "SZ", "TC", "TD", "TF", "TG", "TH", "TJ", "TK", "TL", "TM", "TN", "TO", "TR",
		"TT", "TV", "TW", "TZ", "UA", "UG", "UM", "US", "UY", "UZ", "VA", "VC", "VE", "VG", "VI", "VN", "VU", "WF", "WS",
		"YE", "YT", "ZA", "ZM", "ZW",
	}

	if !slices.Contains(countries, country) {
		return ValidationError{
			Code:    "0032",
			Title:   "Invalid Country Code",
			Message: "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code.",
		}
	}

	return nil
}

// ValidateType validate type values of currencies
func ValidateType(t string) error {
	types := []string{"crypto", "currency", "commodity", "others"}

	if !slices.Contains(types, t) {
		return ValidationError{
			Code:    "0040",
			Title:   "Invalid Type",
			Message: "The provided type is not valid. Accepted types are: currency, crypto, commodities, or others. Please provide a valid type.",
		}
	}

	return nil
}

// ValidateCurrency validate if code contains in currencies list using ISO 4217
func ValidateCurrency(code string) error {
	currencies := []string{
		"AFN", "ALL", "DZD", "USD", "EUR", "AOA", "XCD", "XCD", "ARS", "AMD", "AWG", "AUD", "EUR", "AZN", "BSD", "BHD",
		"BDT", "BBD", "BYN", "EUR", "BZD", "XOF", "BMD", "BTN", "INR", "BOB", "BOV", "USD", "BAM", "BWP", "NOK", "BRL", "USD", "BND", "BGN",
		"XOF", "BIF", "CVE", "KHR", "XAF", "CAD", "KYD", "XAF", "XAF", "CLF", "CLP", "CNY", "AUD", "AUD", "COP", "COU", "KMF", "CDF", "XAF",
		"NZD", "CRC", "EUR", "CUC", "CUP", "ANG", "EUR", "CZK", "XOF", "DKK", "DJF", "XCD", "DOP", "USD", "EGP", "SVC", "USD", "XAF", "ERN",
		"EUR", "ETB", "EUR", "FKP", "DKK", "FJD", "EUR", "EUR", "EUR", "XPF", "EUR", "XAF", "GMD", "GEL", "EUR", "GHS", "GIP", "EUR", "DKK",
		"XCD", "EUR", "USD", "GTQ", "GBP", "GNF", "XOF", "GYD", "HTG", "USD", "AUD", "EUR", "HNL", "HKD", "HUF", "ISK", "INR", "IDR", "XDR",
		"IRR", "IQD", "EUR", "GBP", "ILS", "EUR", "JMD", "JPY", "GBP", "JOD", "KZT", "KES", "AUD", "KPW", "KRW", "KWD", "KGS", "LAK", "EUR",
		"LBP", "LSL", "ZAR", "LRD", "LYD", "CHF", "EUR", "EUR", "MOP", "MGA", "MWK", "MYR", "MVR", "XOF", "EUR", "USD", "EUR", "MRU", "MUR",
		"EUR", "XUA", "MXN", "MXV", "USD", "MDL", "EUR", "MNT", "EUR", "XCD", "MAD", "MZN", "MMK", "NAD", "ZAR", "AUD", "NPR", "EUR", "XPF",
		"NZD", "NIO", "XOF", "NGN", "NZD", "AUD", "USD", "NOK", "OMR", "PKR", "USD", "PAB", "USD", "PGK", "PYG", "PEN", "PHP", "NZD", "PLN",
		"EUR", "USD", "QAR", "MKD", "RON", "RUB", "RWF", "EUR", "EUR", "SHP", "XCD", "XCD", "EUR", "EUR", "XCD", "WST", "EUR", "STN", "SAR",
		"XOF", "RSD", "SCR", "SLE", "SGD", "ANG", "XSU", "EUR", "EUR", "SBD", "SOS", "ZAR", "SSP", "EUR", "LKR", "SDG", "SRD", "NOK", "SZL",
		"SEK", "CHE", "CHF", "CHW", "SYP", "TWD", "TJS", "TZS", "THB", "USD", "XOF", "NZD", "TOP", "TTD", "TND", "TRY", "TMT", "USD", "AUD",
		"UGX", "UAH", "AED", "GBP", "USD", "USD", "USN", "UYI", "UYU", "UZS", "VUV", "VEF", "VED", "VND", "USD", "USD", "XPF", "MAD", "YER",
		"ZMW", "ZWL", "EUR",
	}

	if !slices.Contains(currencies, code) {
		return ValidationError{
			Code:    "0033",
			Title:   "Invalid Code Format",
			Message: "The 'code' field must be alphanumeric, in upper case, and must contain at least one letter. Please provide a valid code.",
		}
	}

	return nil
}

// QueryHeader entity from query parameter from apis
type QueryHeader struct {
	Metadata *bson.M
	Limit    int
	Token    *string
}

// ValidateParameters validate and return struct of default parameters
func ValidateParameters(params map[string]string) *QueryHeader {

	var metadata *bson.M
	var limit = 10
	var token *string

	for key, value := range params {
		switch {
		case strings.Contains(key, "metadata."):
			metadata = &bson.M{key: value}
		case strings.Contains(key, "limit"):
			limit, _ = strconv.Atoi(value)
		case strings.Contains(key, "token"):
			token = &value
		}
	}

	query := &QueryHeader{
		Metadata: metadata,
		Limit:    limit,
		Token:    token,
	}

	return query
}
