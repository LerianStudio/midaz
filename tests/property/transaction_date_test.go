package property

import (
	"encoding/json"
	"testing"
	"testing/quick"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// Property: JSON Marshal -> Unmarshal -> Marshal round-trip preserves value (within millisecond precision)
func TestProperty_TransactionDateJSONRoundtrip(t *testing.T) {
	f := func(year, month, day, hour, minute, second, milli uint16) bool {
		// Constrain to valid ranges
		year = 1900 + (year % 400)      // Year 1900-2299
		month = 1 + (month % 12)        // Month 1-12
		day = 1 + (day % 28)            // Day 1-28 (safe for all months)
		hour = hour % 24                // Hour 0-23
		minute = minute % 60            // Minute 0-59
		second = second % 60            // Second 0-59
		milli = milli % 1000            // Millisecond 0-999

		// Create a time with millisecond precision
		original := time.Date(int(year), time.Month(month), int(day),
			int(hour), int(minute), int(second), int(milli)*1e6, time.UTC)

		td := transaction.TransactionDate(original)

		// Marshal to JSON
		data, err := json.Marshal(td)
		if err != nil {
			t.Logf("marshal error: %v", err)
			return false
		}

		// Unmarshal from JSON
		var unmarshaled transaction.TransactionDate
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Logf("unmarshal error: %v", err)
			return false
		}

		// Marshal again
		data2, err := json.Marshal(unmarshaled)
		if err != nil {
			t.Logf("second marshal error: %v", err)
			return false
		}

		// Both marshaled values should be equal
		if string(data) != string(data2) {
			t.Logf("roundtrip mismatch: first=%s second=%s", string(data), string(data2))
			return false
		}

		// Time values should match within millisecond precision
		originalTime := td.Time().Truncate(time.Millisecond)
		unmarshaledTime := unmarshaled.Time().Truncate(time.Millisecond)
		if !originalTime.Equal(unmarshaledTime) {
			t.Logf("time mismatch: original=%v unmarshaled=%v", originalTime, unmarshaledTime)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("TransactionDate JSON roundtrip failed: %v", err)
	}
}

// Property: All supported ISO 8601 format strings parse successfully
func TestProperty_TransactionDateAllFormats(t *testing.T) {
	f := func(year, month, day, hour, minute, second, milli uint16) bool {
		// Constrain to valid ranges
		year = 1900 + (year % 400)      // Year 1900-2299
		month = 1 + (month % 12)        // Month 1-12
		day = 1 + (day % 28)            // Day 1-28 (safe for all months)
		hour = hour % 24                // Hour 0-23
		minute = minute % 60            // Minute 0-59
		second = second % 60            // Second 0-59
		milli = milli % 1000            // Millisecond 0-999

		baseTime := time.Date(int(year), time.Month(month), int(day),
			int(hour), int(minute), int(second), int(milli)*1e6, time.UTC)

		// All formats that TransactionDate supports
		formats := []struct {
			name   string
			value  string
		}{
			{"RFC3339Nano", baseTime.Format(time.RFC3339Nano)},
			{"RFC3339", baseTime.Truncate(time.Second).Format(time.RFC3339)},
			{"ISO8601_Milli_Z", baseTime.Format("2006-01-02T15:04:05.000Z")},
			{"ISO8601_Z", baseTime.Truncate(time.Second).Format("2006-01-02T15:04:05Z")},
			{"ISO8601_NoTZ", baseTime.Truncate(time.Second).Format("2006-01-02T15:04:05")},
			{"DateOnly", baseTime.Format("2006-01-02")},
		}

		for _, format := range formats {
			jsonStr := `"` + format.value + `"`
			var td transaction.TransactionDate
			if err := json.Unmarshal([]byte(jsonStr), &td); err != nil {
				t.Logf("format %s failed to parse: value=%s error=%v", format.name, format.value, err)
				return false
			}

			// Non-zero time should be parseable
			if td.IsZero() {
				t.Logf("format %s parsed to zero time: value=%s", format.name, format.value)
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("TransactionDate all formats parsing failed: %v", err)
	}
}

// Property: Zero TransactionDate marshals to "null"
func TestProperty_TransactionDateZeroMarshalsToNull(t *testing.T) {
	f := func(_ uint8) bool { // Use dummy arg for quick.Check
		var td transaction.TransactionDate // Zero value

		data, err := json.Marshal(td)
		if err != nil {
			t.Logf("marshal zero error: %v", err)
			return false
		}

		if string(data) != "null" {
			t.Logf("zero value should marshal to null, got: %s", string(data))
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("TransactionDate zero marshals to null failed: %v", err)
	}
}

// Property: "null" and empty string unmarshal to zero TransactionDate
func TestProperty_TransactionDateNullUnmarshalsToZero(t *testing.T) {
	testCases := []string{
		`null`,
		`"null"`,
		`""`,
	}

	f := func(caseIdx uint8) bool {
		idx := int(caseIdx) % len(testCases)
		jsonStr := testCases[idx]

		var td transaction.TransactionDate
		if err := json.Unmarshal([]byte(jsonStr), &td); err != nil {
			t.Logf("unmarshal %q error: %v", jsonStr, err)
			return false
		}

		if !td.IsZero() {
			t.Logf("%q should unmarshal to zero, got: %v", jsonStr, td.Time())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("TransactionDate null unmarshal to zero failed: %v", err)
	}
}

// Property: Marshal preserves time precision up to milliseconds
func TestProperty_TransactionDateMillisecondPrecision(t *testing.T) {
	f := func(year, month, day, hour, minute, second, milli uint16) bool {
		// Constrain to valid ranges
		year = 1900 + (year % 400)
		month = 1 + (month % 12)
		day = 1 + (day % 28)
		hour = hour % 24
		minute = minute % 60
		second = second % 60
		milli = milli % 1000

		// Create time with specific milliseconds
		original := time.Date(int(year), time.Month(month), int(day),
			int(hour), int(minute), int(second), int(milli)*1e6, time.UTC)

		td := transaction.TransactionDate(original)

		// Marshal
		data, err := json.Marshal(td)
		if err != nil {
			t.Logf("marshal error: %v", err)
			return false
		}

		// Unmarshal
		var unmarshaled transaction.TransactionDate
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Logf("unmarshal error: %v", err)
			return false
		}

		// Check millisecond preservation
		origMilli := original.Nanosecond() / 1e6
		unmarshaledMilli := unmarshaled.Time().Nanosecond() / 1e6

		if origMilli != unmarshaledMilli {
			t.Logf("millisecond mismatch: original=%d unmarshaled=%d", origMilli, unmarshaledMilli)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("TransactionDate millisecond precision failed: %v", err)
	}
}
