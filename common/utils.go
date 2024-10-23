package common

import (
	"encoding/json"
	"github.com/google/uuid"
	"regexp"
	"slices"
	"strconv"
	"unicode"

	cn "github.com/LerianStudio/midaz/common/constant"
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
			return cn.ErrMetadataKeyLengthExceeded
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
			return cn.ErrMetadataValueLengthExceeded
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
		return cn.ErrInvalidCountryCode
	}

	return nil
}

// ValidateAccountType validate type values of accounts
func ValidateAccountType(t string) error {
	types := []string{"deposit", "savings", "loans", "marketplace", "creditCard", "external"}

	if !slices.Contains(types, t) {
		return cn.ErrInvalidAccountType
	}

	return nil
}

// ValidateType validate type values of currencies
func ValidateType(t string) error {
	types := []string{"crypto", "currency", "commodity", "others"}

	if !slices.Contains(types, t) {
		return cn.ErrInvalidType
	}

	return nil
}

func ValidateCode(code string) error {
	for _, r := range code {
		if !unicode.IsLetter(r) {
			return cn.ErrInvalidCodeFormat
		} else if !unicode.IsUpper(r) {
			return cn.ErrCodeUppercaseRequirement
		}
	}

	return nil
}

// ValidateCurrency validate if code contains in currencies list using ISO 4217
func ValidateCurrency(code string) error {
	currencies := []string{
		"AED", "AFN", "ALL", "AMD", "ANG", "AOA", "ARS", "AUD", "AWG", "AZN", "BAM", "BBD", "BDT", "BGN", "BHD", "BIF", "BMD", "BND", "BOB",
		"BOV", "BRL", "BSD", "BTN", "BWP", "BYN", "BZD", "CAD", "CDF", "CHE", "CHF", "CHW", "CLF", "CLP", "CNY", "COP", "COU", "CRC", "CUC",
		"CUP", "CVE", "CZK", "DJF", "DKK", "DOP", "DZD", "EGP", "ERN", "ETB", "EUR", "FJD", "FKP", "GBP", "GEL", "GHS", "GIP", "GMD", "GNF",
		"GTQ", "GYD", "HKD", "HNL", "HTG", "HUF", "IDR", "ILS", "INR", "IQD", "IRR", "ISK", "JMD", "JOD", "JPY", "KES", "KGS", "KHR", "KMF",
		"KPW", "KRW", "KWD", "KYD", "KZT", "LAK", "LBP", "LKR", "LRD", "LSL", "LYD", "MAD", "MDL", "MGA", "MKD", "MMK", "MNT", "MOP", "MRU",
		"MUR", "MVR", "MWK", "MXN", "MXV", "MYR", "MZN", "NAD", "NGN", "NIO", "NOK", "NPR", "NZD", "OMR", "PAB", "PEN", "PGK", "PHP", "PKR",
		"PLN", "PYG", "QAR", "RON", "RSD", "RUB", "RWF", "SAR", "SBD", "SCR", "SDG", "SEK", "SGD", "SHP", "SLE", "SOS", "SRD", "SSP", "STN",
		"SVC", "SYP", "SZL", "THB", "TJS", "TMT", "TND", "TOP", "TRY", "TTD", "TWD", "TZS", "UAH", "UGX", "USD", "USN", "UYI", "UYU", "UZS",
		"VED", "VEF", "VND", "VUV", "WST", "XAF", "XCD", "XDR", "XOF", "XPF", "XSU", "XUA", "YER", "ZAR", "ZMW", "ZWL",
	}

	if !slices.Contains(currencies, code) {
		return cn.ErrCurrencyCodeStandardCompliance
	}

	return nil
}

// SafeIntToUint64 safe mode to converter int to uint64
func SafeIntToUint64(val int) uint64 {
	if val < 0 {
		return uint64(1)
	}

	return uint64(val)
}

// IsUUID Validate if the string pass through is an uuid
func IsUUID(s string) bool {
	r := regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[1-5][a-fA-F0-9]{3}-[89abAB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$")
	return r.MatchString(s)
}

// GenerateUUIDv7 generate a new uuid v7 using google/uuid package and return it. If an error occurs, it will return the error.
func GenerateUUIDv7() uuid.UUID {
	u := uuid.Must(uuid.NewV7())

	return u
}

// StructToJSONString convert a struct to json string
func StructToJSONString(s any) (string, error) {
	jsonByte, err := json.Marshal(s)
	if err != nil {
		return "", err
	}

	return string(jsonByte), nil
}
