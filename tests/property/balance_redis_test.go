package property

import (
	"encoding/json"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// Property: BalanceRedis correctly unmarshals decimal values from float64
func TestProperty_BalanceRedisUnmarshalFloat64(t *testing.T) {
	f := func(available, onHold float64) bool {
		// Skip NaN and Inf
		if available != available || onHold != onHold { // NaN check
			return true
		}

		// Constrain to reasonable values
		if available > 1e15 || available < -1e15 || onHold > 1e15 || onHold < -1e15 {
			return true
		}

		jsonData := []byte(`{
			"id": "test-id",
			"accountId": "acc-id",
			"assetCode": "USD",
			"available": ` + decimal.NewFromFloat(available).String() + `,
			"onHold": ` + decimal.NewFromFloat(onHold).String() + `,
			"version": 1,
			"allowSending": 1,
			"allowReceiving": 1
		}`)

		var balance mmodel.BalanceRedis
		if err := json.Unmarshal(jsonData, &balance); err != nil {
			t.Logf("unmarshal failed: %v for data: %s", err, string(jsonData))
			return false
		}

		// Verify values are close (float64 has precision limits)
		expectedAvail := decimal.NewFromFloat(available)
		expectedOnHold := decimal.NewFromFloat(onHold)

		availDiff := balance.Available.Sub(expectedAvail).Abs()
		onHoldDiff := balance.OnHold.Sub(expectedOnHold).Abs()

		tolerance := decimal.NewFromFloat(0.0001)

		if availDiff.GreaterThan(tolerance) {
			t.Logf("available mismatch: expected %s, got %s", expectedAvail, balance.Available)
			return false
		}

		if onHoldDiff.GreaterThan(tolerance) {
			t.Logf("onHold mismatch: expected %s, got %s", expectedOnHold, balance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("BalanceRedis float64 unmarshal property failed: %v", err)
	}
}

// Property: BalanceRedis correctly unmarshals decimal values from string
func TestProperty_BalanceRedisUnmarshalString(t *testing.T) {
	f := func(availInt, availFrac, onHoldInt, onHoldFrac int64) bool {
		// Constrain to reasonable values
		availInt = availInt % 1_000_000_000
		onHoldInt = onHoldInt % 1_000_000_000
		availFrac = (availFrac % 1_000_000)
		onHoldFrac = (onHoldFrac % 1_000_000)

		if availFrac < 0 {
			availFrac = -availFrac
		}
		if onHoldFrac < 0 {
			onHoldFrac = -onHoldFrac
		}

		availStr := decimal.NewFromInt(availInt).String()
		if availFrac > 0 {
			availStr = availStr + "." + padLeft(availFrac, 6)
		}

		onHoldStr := decimal.NewFromInt(onHoldInt).String()
		if onHoldFrac > 0 {
			onHoldStr = onHoldStr + "." + padLeft(onHoldFrac, 6)
		}

		jsonData := []byte(`{
			"id": "test-id",
			"accountId": "acc-id",
			"assetCode": "USD",
			"available": "` + availStr + `",
			"onHold": "` + onHoldStr + `",
			"version": 1,
			"allowSending": 1,
			"allowReceiving": 1
		}`)

		var balance mmodel.BalanceRedis
		if err := json.Unmarshal(jsonData, &balance); err != nil {
			t.Logf("unmarshal failed: %v for data: %s", err, string(jsonData))
			return false
		}

		expectedAvail, _ := decimal.NewFromString(availStr)
		expectedOnHold, _ := decimal.NewFromString(onHoldStr)

		if !balance.Available.Equal(expectedAvail) {
			t.Logf("available mismatch: expected %s, got %s", expectedAvail, balance.Available)
			return false
		}

		if !balance.OnHold.Equal(expectedOnHold) {
			t.Logf("onHold mismatch: expected %s, got %s", expectedOnHold, balance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("BalanceRedis string unmarshal property failed: %v", err)
	}
}

// padLeft pads a number with leading zeros to reach the specified width
func padLeft(n int64, width int) string {
	s := decimal.NewFromInt(n).String()
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// Property: BalanceRedis unmarshal handles json.Number correctly
func TestProperty_BalanceRedisUnmarshalJSONNumber(t *testing.T) {
	// json.Number is produced when using json.Decoder with UseNumber()
	testCases := []struct {
		name     string
		jsonData string
		expected decimal.Decimal
	}{
		{"integer", `{"id":"t","accountId":"a","assetCode":"USD","available":12345,"onHold":0,"version":1}`, decimal.NewFromInt(12345)},
		{"float", `{"id":"t","accountId":"a","assetCode":"USD","available":123.45,"onHold":0,"version":1}`, decimal.NewFromFloat(123.45)},
		{"string", `{"id":"t","accountId":"a","assetCode":"USD","available":"999.99","onHold":0,"version":1}`, decimal.NewFromFloat(999.99)},
		{"zero", `{"id":"t","accountId":"a","assetCode":"USD","available":0,"onHold":0,"version":1}`, decimal.Zero},
		{"negative", `{"id":"t","accountId":"a","assetCode":"USD","available":-500,"onHold":0,"version":1}`, decimal.NewFromInt(-500)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var balance mmodel.BalanceRedis
			if err := json.Unmarshal([]byte(tc.jsonData), &balance); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if !balance.Available.Equal(tc.expected) {
				t.Errorf("expected %s, got %s", tc.expected, balance.Available)
			}
		})
	}
}
