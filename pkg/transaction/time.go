package transaction

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TimeError wraps a time-related error with context
type TimeError struct {
	Message string
	Cause   error
}

// Error implements the error interface
func (e TimeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}

	return e.Message
}

// Unwrap returns the underlying error
func (e TimeError) Unwrap() error {
	return e.Cause
}

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

	return TimeError{Message: "invalid date format: " + str}
}

func (td TransactionDate) MarshalJSON() ([]byte, error) {
	if td.IsZero() {
		return []byte("null"), nil
	}

	t := time.Time(td)

	if t.Nanosecond() != 0 {
		result, err := json.Marshal(t.Format("2006-01-02T15:04:05.000Z07:00"))
		if err != nil {
			return nil, TimeError{Message: "failed to marshal time", Cause: err}
		}

		return result, nil
	}

	result, err := json.Marshal(t.Format(time.RFC3339))
	if err != nil {
		return nil, TimeError{Message: "failed to marshal time", Cause: err}
	}

	return result, nil
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
