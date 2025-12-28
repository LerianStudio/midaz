package fuzzy

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	fuzz "github.com/google/gofuzz"
	"github.com/shopspring/decimal"
)

// FuzzBalanceRedisUnmarshalJSON tests the custom JSON unmarshaler for BalanceRedis
// with gofuzz-generated diverse structs plus malformed and edge case inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzBalanceRedisUnmarshalJSON -run=^$ -fuzztime=60s
func FuzzBalanceRedisUnmarshalJSON(f *testing.F) {
	// Use gofuzz to generate diverse BalanceRedis structs then serialize to JSON
	fuzzer := fuzz.New().NilChance(0.1).NumElements(1, 5).Funcs(
		// Custom fuzzer for decimal.Decimal (use float64 for JSON representation)
		func(d *decimal.Decimal, c fuzz.Continue) {
			choices := []float64{
				0, 1, -1, 100.50, -100.50, 1000.123,
				9007199254740992,  // 2^53
				9007199254740993,  // beyond 2^53
				0.000000001,       // very small
				999999999999.9999, // large with decimals
			}
			if c.RandBool() {
				*d = decimal.NewFromFloat(choices[c.Intn(len(choices))])
			} else {
				*d = decimal.NewFromFloat(c.Float64())
			}
		},
		// Custom fuzzer for string fields
		func(s *string, c fuzz.Continue) {
			choices := []string{
				"test-id", "@person1", "USD", "BRL", "checking", "savings",
				"00000000-0000-0000-0000-000000000000",
				"", // empty
			}
			if c.RandBool() {
				*s = choices[c.Intn(len(choices))]
			} else {
				c.Fuzz(s)
			}
		},
	)

	// Generate 20 diverse JSON seeds using gofuzz
	for i := 0; i < 20; i++ {
		var br mmodel.BalanceRedis
		fuzzer.Fuzz(&br)
		// Serialize to JSON
		jsonBytes, err := json.Marshal(br)
		if err == nil {
			f.Add(string(jsonBytes))
		}
	}

	// Manual seeds: valid JSON with different available/onHold types

	// float64 type
	f.Add(`{"id":"test-id","alias":"@test","accountId":"acc-1","assetCode":"USD","available":1000.50,"onHold":500.25,"version":1,"accountType":"checking","allowSending":1,"allowReceiving":1,"key":"default"}`)

	// string type
	f.Add(`{"id":"test-id","alias":"@test","accountId":"acc-1","assetCode":"USD","available":"1000.50","onHold":"500.25","version":1,"accountType":"checking","allowSending":1,"allowReceiving":1,"key":"default"}`)

	// integer type
	f.Add(`{"id":"test-id","alias":"@test","accountId":"acc-1","assetCode":"USD","available":1000,"onHold":500,"version":1,"accountType":"checking","allowSending":1,"allowReceiving":1,"key":"default"}`)

	// json.Number type (via UseNumber)
	f.Add(`{"id":"test-id","available":9007199254740993,"onHold":0}`)

	// Manual seeds: boundary values
	f.Add(`{"available":9223372036854775807,"onHold":0}`)           // max int64
	f.Add(`{"available":-9223372036854775808,"onHold":0}`)          // min int64
	f.Add(`{"available":"9223372036854775807","onHold":"0"}`)       // max int64 as string
	f.Add(`{"available":9007199254740992,"onHold":0}`)              // float64 precision boundary
	f.Add(`{"available":9007199254740993,"onHold":0}`)              // beyond float64 precision

	// Manual seeds: large decimal strings
	f.Add(`{"available":"123456789012345678901234567890.123456789","onHold":"0"}`)
	f.Add(`{"available":"0.000000000000000000000000001","onHold":"0"}`)

	// Manual seeds: scientific notation
	f.Add(`{"available":1e18,"onHold":0}`)
	f.Add(`{"available":"1e18","onHold":"0"}`)
	f.Add(`{"available":1.5e10,"onHold":0}`)

	// Manual seeds: empty values
	f.Add(`{"available":"","onHold":""}`)
	f.Add(`{"available":null,"onHold":null}`)
	f.Add(`{}`)

	// Manual seeds: wrong types
	f.Add(`{"available":true,"onHold":false}`)
	f.Add(`{"available":[],"onHold":{}}`)
	f.Add(`{"available":"not-a-number","onHold":"invalid"}`)

	// Manual seeds: malformed JSON
	f.Add(`{"available":1000`)
	f.Add(`{available:1000}`)
	f.Add(``)
	f.Add(`null`)
	f.Add(`[]`)

	// Manual seeds: injection attempts
	f.Add(`{"available":"0; DROP TABLE balances;--","onHold":"0"}`)
	f.Add(`{"available":"{{.Cmd}}","onHold":"0"}`)

	// Manual seeds: unicode edge cases
	f.Add(`{"available":"\u0030","onHold":"0"}`)                    // Unicode 0
	f.Add(`{"id":"test\u0000id","available":1000,"onHold":0}`)      // null byte
	f.Add(`{"id":"test\u200Bid","available":1000,"onHold":0}`)      // zero-width space

	f.Fuzz(func(t *testing.T, jsonData string) {
		// Skip invalid UTF-8 early
		if !utf8.ValidString(jsonData) {
			return
		}

		// The unmarshaler should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("UnmarshalJSON panicked on input (len=%d): %v\nInput: %q",
					len(jsonData), r, truncateString(jsonData, 200))
			}
		}()

		var balance mmodel.BalanceRedis

		// Call unmarshal - we expect either success or a proper error, never a panic
		err := json.Unmarshal([]byte(jsonData), &balance)

		// If successful, verify the result is internally consistent
		if err == nil {
			// Available and OnHold should be valid decimals (not panic when accessed)
			_ = balance.Available.String()
			_ = balance.OnHold.String()

			// Version should be non-negative
			if balance.Version < 0 {
				t.Logf("Warning: negative version: %d", balance.Version)
			}
		}
	})
}
