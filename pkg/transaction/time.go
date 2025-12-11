package transaction

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TransactionDate is a custom time type that supports multiple ISO 8601 formats including milliseconds
type TransactionDate time.Time

var transactionDateFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func (td *TransactionDate) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	if str == "null" || str == "" {
		*td = TransactionDate{}
		return nil
	}

	for _, format := range transactionDateFormats {
		if t, err := time.Parse(format, str); err == nil {
			if t.IsZero() {
				*td = TransactionDate{}
				return nil
			}
			*td = TransactionDate(t)
			return nil
		}
	}

	return fmt.Errorf("invalid date format: %s", str)
}

func (td TransactionDate) MarshalJSON() ([]byte, error) {
	if td.IsZero() {
		return []byte("null"), nil
	}

	t := time.Time(td)
	if t.Nanosecond() != 0 {
		return json.Marshal(t.Format("2006-01-02T15:04:05.000Z07:00"))
	}

	return json.Marshal(t.Format(time.RFC3339))
}

func (td TransactionDate) Time() time.Time {
	return time.Time(td)
}

func (td TransactionDate) IsZero() bool {
	return time.Time(td).IsZero()
}

func (td TransactionDate) After(t time.Time) bool {
	return time.Time(td).After(t)
}
