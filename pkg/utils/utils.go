package utils

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os/exec"
	"reflect"
	"slices"
	"strconv"
	"time"
	"unicode"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"go.opentelemetry.io/otel/metric"
)

// Contains checks if an item is in a slice. This function uses type parameters to work with any slice type.
func Contains[T comparable](slice []T, item T) bool {
	return slices.Contains(slice, item)
}

// CheckMetadataKeyAndValueLength check the length of key and value to a limit pass by on field limit
func CheckMetadataKeyAndValueLength(limit int, metadata map[string]any) error {
	for k, v := range metadata {
		if len(k) > limit {
			return errors.New("0050")
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
			return errors.New("0051")
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
		return errors.New("0032")
	}

	return nil
}

// ValidateAccountType validate type values of accounts
func ValidateAccountType(t string) error {
	types := []string{"deposit", "savings", "loans", "marketplace", "creditCard"}

	if !slices.Contains(types, t) {
		return errors.New("0066")
	}

	return nil
}

// ValidateType validate type values of currencies
func ValidateType(t string) error {
	types := []string{"crypto", "currency", "commodity", "others"}

	if !slices.Contains(types, t) {
		return errors.New("0040")
	}

	return nil
}

func ValidateCode(code string) error {
	for _, r := range code {
		if !unicode.IsLetter(r) {
			return errors.New("0033")
		} else if !unicode.IsUpper(r) {
			return errors.New("0004")
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
		return errors.New("0005")
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

// SafeInt64ToInt safely converts int64 to int
func SafeInt64ToInt(val int64) int {
	if val > math.MaxInt {
		return math.MaxInt
	} else if val < math.MinInt {
		return math.MinInt
	}

	return int(val)
}

// SafeUintToInt converts a uint to int64 safely by capping values at math.MaxInt64.
func SafeUintToInt(val uint) int {
	if val > uint(math.MaxInt) {
		return math.MaxInt
	}

	return int(val)
}

// SafeIntToUint32 safely converts int to uint32 with overflow protection.
// Returns the converted value if in valid range [0, MaxUint32], otherwise returns defaultVal.
// This prevents G115 (CWE-190) integer overflow vulnerabilities.
func SafeIntToUint32(value int, defaultVal uint32, logger log.Logger, fieldName string) uint32 {
	if value < 0 {
		if logger != nil {
			logger.Debugf("Invalid %s value %d (negative), using default: %d", fieldName, value, defaultVal)
		}

		return defaultVal
	}

	uv := uint64(value)

	if uv > uint64(math.MaxUint32) {
		if logger != nil {
			logger.Debugf("%s value %d exceeds uint32 max (%d), using default %d", fieldName, value, uint64(math.MaxUint32), defaultVal)
		}

		return defaultVal
	}

	return uint32(uv)
}

// IsUUID Validate if the string pass through is an uuid
func IsUUID(s string) bool {
	_, err := uuid.Parse(s)

	return err == nil
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

// MergeMaps Following the JSON Merge Patch
func MergeMaps(source, target map[string]any) map[string]any {
	for key, value := range source {
		if value != nil {
			target[key] = value
		} else {
			delete(target, key)
		}
	}

	return target
}

type SyscmdI interface {
	ExecCmd(name string, arg ...string) ([]byte, error)
}

type Syscmd struct{}

func (r *Syscmd) ExecCmd(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).Output()
}

// GetCPUUsage get the current CPU usage
func GetCPUUsage(ctx context.Context, cpuGauge metric.Int64Gauge) {
	logger := libCommons.NewLoggerFromContext(ctx)

	out, err := cpu.Percent(100*time.Millisecond, false)
	if err != nil {
		logger.Warnf("Error to get cpu use: %v", err)
	}

	var percentageCPU int64 = 0
	if len(out) > 0 {
		percentageCPU = int64(out[0])
	}

	cpuGauge.Record(ctx, percentageCPU)
}

// GetMemUsage get the current memory usage
func GetMemUsage(ctx context.Context, memGauge metric.Int64Gauge) {
	logger := libCommons.NewLoggerFromContext(ctx)

	var percentageMem int64 = 0

	out, err := mem.VirtualMemory()
	if err != nil {
		logger.Warnf("Error to get info memory: %v", err)
	} else {
		percentageMem = int64(out.UsedPercent)
	}

	memGauge.Record(ctx, percentageMem)
}

// GetMapNumKinds get the map of numeric kinds to use in validations and conversions.
//
// The numeric kinds are:
// - int
// - int8
// - int16
// - int32
// - int64
// - float32
// - float64
func GetMapNumKinds() map[reflect.Kind]bool {
	numKinds := make(map[reflect.Kind]bool)

	numKinds[reflect.Int] = true
	numKinds[reflect.Int8] = true
	numKinds[reflect.Int16] = true
	numKinds[reflect.Int32] = true
	numKinds[reflect.Int64] = true
	numKinds[reflect.Float32] = true
	numKinds[reflect.Float64] = true

	return numKinds
}

// Reverse reverses a slice of any type.
func Reverse[T any](s []T) []T {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}

	return s
}

// IdempotencyInternalKey returns a key with the following format to be used on redis cluster:
// "idempotency:{organizationID:ledgerID:key}"
func IdempotencyInternalKey(organizationID, ledgerID uuid.UUID, key string) string {
	idempotency := GenericInternalKey("idempotency", "idempotency", organizationID.String(), ledgerID.String(), key) // TODO: Ver sobre a repetição de valores nos dois primeiros parametros

	return idempotency
}

// AccountingRoutesInternalKey returns a key with the following format to be used on redis cluster:
// "accounting_routes:{organizationID:ledgerID:key}"
func AccountingRoutesInternalKey(organizationID, ledgerID, key uuid.UUID) string {
	accountingRoutes := GenericInternalKey("accounting_routes", "accounting_routes", organizationID.String(), ledgerID.String(), key.String()) // TODO: Ver sobre a repetição de valores nos dois primeiros parametros

	return accountingRoutes
}

// UUIDsToStrings converts a slice of UUIDs to a slice of strings.
// It's optimized to minimize allocations and iterations.
func UUIDsToStrings(uuids []uuid.UUID) []string {
	result := make([]string, len(uuids))
	for i := range uuids {
		result[i] = uuids[i].String()
	}

	return result
}
