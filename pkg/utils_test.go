package pkg

import (
	"context"
	"errors"
	"reflect"
	"testing"

	cn "github.com/LerianStudio/midaz/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"
)

func TestContains(t *testing.T) {
	type args struct {
		slice []any
		item  any
	}

	type examploToTestCheck struct {
		Msg string
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "checks for an integer value returning positive",
			args: args{
				slice: []any{1, 2, 3, 4, 5},
				item:  4,
			},
			want: true,
		},
		{
			name: "checks for integer value returns false because the value entered is not in the list", args: args{
				slice: []any{1, 2, 3, 4, 5},
				item:  11,
			},
			want: false,
		},
		{
			name: "checks for an string value returning positive",
			args: args{
				slice: []any{"luffy", "sanji", "zoro"},
				item:  "sanji",
			},
			want: true,
		},
		{
			name: "checks for string value returns false because the value entered is not in the list",
			args: args{
				slice: []any{"luffy", "sanji"},
				item:  "zoro",
			},
			want: false,
		},
		{
			name: "checks for an struct value returning positive",
			args: args{
				slice: []any{
					examploToTestCheck{
						Msg: "01",
					},
				},
				item: examploToTestCheck{Msg: "01"},
			},
			want: true,
		},
		{
			name: "checks for struct value returns false because the value entered is not in the list",
			args: args{
				slice: []any{
					examploToTestCheck{
						Msg: "02",
					},
				},
				item: examploToTestCheck{Msg: "01"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.args.slice, tt.args.item); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckMetadataKeyAndValueLength(t *testing.T) {
	type args struct {
		limit    int
		metadata map[string]any
	}
	tests := []struct {
		name    string
		args    args
		err     error
		wantErr bool
	}{
		{
			name: "exceeds the key limit",
			args: args{
				limit: 2,
				metadata: map[string]any{
					"01":  12,
					"02":  13,
					"033": 142,
				},
			},
			err:     cn.ErrMetadataKeyLengthExceeded,
			wantErr: true,
		},
		{
			name: "case parse int",
			args: args{
				limit: 2,
				metadata: map[string]any{
					"01": 12,
					"02": 13,
				},
			},
		},
		{
			name: "case parse float64",
			args: args{
				limit: 5,
				metadata: map[string]any{
					"01": 12.1,
					"02": 13.4,
				},
			},
		},
		{
			name: "case parse string",
			args: args{
				limit: 5,
				metadata: map[string]any{
					"01": "br",
					"02": "us",
				},
			},
		},
		{
			name: "case parse bool",
			args: args{
				limit: 5,
				metadata: map[string]any{
					"01": true,
					"02": false,
				},
			},
		},
		{
			name: "case parse string exeeds the limit value",
			args: args{
				limit: 5,
				metadata: map[string]any{
					"01": "br",
					"02": "us",
					"03": "Guarapa",
				},
			},
			err:     cn.ErrMetadataValueLengthExceeded,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckMetadataKeyAndValueLength(tt.args.limit, tt.args.metadata)
			if (err != nil) != tt.wantErr {
				if err != tt.err {
					t.Errorf("CheckMetadataKeyAndValueLength() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateCountryAddress(t *testing.T) {
	type args struct {
		country string
	}
	tests := []struct {
		name    string
		args    args
		err     error
		wantErr bool
	}{
		{
			name: "get item exist",
			args: args{
				country: "LY",
			},
		},
		{
			name: "failed get item no exist",
			args: args{
				country: "SOU",
			},
			err:     cn.ErrInvalidCountryCode,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateCountryAddress(tt.args.country); (err != nil) != tt.wantErr {
				t.Errorf("ValidateCountryAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAccountType(t *testing.T) {
	tests := []struct {
		name        string
		accountType string
		wantErr     bool
	}{
		{"Valid Deposit", "deposit", false},
		{"Valid Savings", "savings", false},
		{"Valid Loans", "loans", false},
		{"Valid Marketplace", "marketplace", false},
		{"Valid CreditCard", "creditCard", false},
		{"Valid External", "external", false},
		{"Invalid Account Type", "invalidType", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountType(tt.accountType)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateType(t *testing.T) {
	tests := []struct {
		input    string
		expected error
	}{
		{"crypto", nil},
		{"currency", nil},
		{"commodity", nil},
		{"others", nil},
		{"invalid", cn.ErrInvalidType},
		{"", cn.ErrInvalidType},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := ValidateType(tt.input)
			if err != tt.expected {
				t.Errorf("ValidateType(%q) = %v; want %v", tt.input, err, tt.expected)
			}
		})
	}
}

func TestValidateCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected error
	}{
		{name: "Valid Code", input: "USD", expected: nil},
		{name: "Lowercase Letters", input: "usd", expected: cn.ErrCodeUppercaseRequirement},
		{name: "Contains Number", input: "US1", expected: cn.ErrInvalidCodeFormat},
		{name: "Contains Symbol", input: "US$", expected: cn.ErrInvalidCodeFormat},
		{name: "Empty Code", input: "", expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCode(tt.input)
			if err != tt.expected {
				t.Errorf("ValidateCode(%q) = %v; want %v", tt.input, err, tt.expected)
			}
		})
	}
}

func TestValidateCurrency(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected error
	}{
		{name: "Valid Currency Code - USD", input: "USD", expected: nil},
		{name: "Valid Currency Code - EUR", input: "EUR", expected: nil},
		{name: "Valid Currency Code - JPY", input: "JPY", expected: nil},
		{name: "Invalid Currency Code", input: "ABC", expected: cn.ErrCurrencyCodeStandardCompliance},
		{name: "Empty Currency Code", input: "", expected: cn.ErrCurrencyCodeStandardCompliance},
		{name: "Lowercase Currency Code", input: "usd", expected: cn.ErrCurrencyCodeStandardCompliance},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCurrency(tt.input)
			if err != tt.expected {
				t.Errorf("ValidateCurrency(%q) = %v; want %v", tt.input, err, tt.expected)
			}
		})
	}
}

func TestSafeIntToUint64(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected uint64
	}{
		{name: "Positive Value", input: 42, expected: 42},
		{name: "Zero Value", input: 0, expected: 0},
		{name: "Negative Value", input: -1, expected: 1},
		{name: "Large Positive Value", input: 1000000, expected: 1000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeIntToUint64(tt.input)
			if result != tt.expected {
				t.Errorf("SafeIntToUint64(%d) = %d; want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "Valid UUID", input: "123e4567-e89b-12d3-a456-426614174000", expected: true},
		{name: "Invalid UUID - Missing Segments", input: "123e4567-e89b-12d3-a456", expected: false},
		{name: "Invalid UUID - Extra Characters", input: "123e4567-e89b-12d3-a456-426614174000xyz", expected: false},
		{name: "Invalid UUID - Wrong Version", input: "123e4567-e89b-62d3-a456-426614174000", expected: false},
		{name: "Invalid UUID - Wrong Variant", input: "123e4567-e89b-12d3-c456-426614174000", expected: false},
		{name: "Empty String", input: "", expected: false},
		{name: "Random String", input: "not-a-uuid", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsUUID(tt.input)
			assert.Equal(t, tt.expected, result, "IsUUID(%q) should return %v", tt.input, tt.expected)
		})
	}
}

func Test_GenerateUUIDv7(t *testing.T) {
	u := GenerateUUIDv7()
	assert.NotEqual(t, uuid.Nil, u, "Generated UUIDv7 should not be nil")
	assert.Equal(t, 7, int(u.Version()), "Generated UUID version should be 7")
	assert.Equal(t, 36, len(u.String()), "Generated UUID length should be 36")
}

func TestStructToJSONString(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		expected    string
		expectError bool
	}{
		{
			name:        "Valid Struct",
			input:       struct{ Name string }{Name: "John"},
			expected:    `{"Name":"John"}`,
			expectError: false,
		},
		{
			name:        "Nil Input",
			input:       nil,
			expected:    "null",
			expectError: false,
		},
		{
			name:        "Empty Struct",
			input:       struct{}{},
			expected:    `{}`,
			expectError: false,
		},
		{
			name: "Struct with Multiple Fields",
			input: struct {
				Name string
				Age  int
			}{Name: "Alice", Age: 30},
			expected:    `{"Name":"Alice","Age":30}`,
			expectError: false,
		},
		{
			name:        "Invalid Struct (unexported field)",
			input:       struct{ name string }{name: "Hidden"},
			expected:    `{}`, // Unexported fields are ignored in JSON serialization
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StructToJSONString(tt.input)

			if tt.expectError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Did not expect an error but got one")
				assert.JSONEq(t, tt.expected, result, "Expected JSON: %s, got: %s", tt.expected, result)
			}
		})
	}
}

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		source   map[string]any
		target   map[string]any
		expected map[string]any
	}{
		{
			name:     "Add new keys from source",
			source:   map[string]any{"key1": "value1", "key2": "value2"},
			target:   map[string]any{"key3": "value3"},
			expected: map[string]any{"key1": "value1", "key2": "value2", "key3": "value3"},
		},
		{
			name:     "Override existing keys in target",
			source:   map[string]any{"key1": "newValue1"},
			target:   map[string]any{"key1": "oldValue1", "key2": "value2"},
			expected: map[string]any{"key1": "newValue1", "key2": "value2"},
		},
		{
			name:     "Remove keys from target when source value is nil",
			source:   map[string]any{"key1": nil},
			target:   map[string]any{"key1": "value1", "key2": "value2"},
			expected: map[string]any{"key2": "value2"},
		},
		{
			name:     "Empty source map",
			source:   map[string]any{},
			target:   map[string]any{"key1": "value1"},
			expected: map[string]any{"key1": "value1"},
		},
		{
			name:     "Empty target map",
			source:   map[string]any{"key1": "value1"},
			target:   map[string]any{},
			expected: map[string]any{"key1": "value1"},
		},
		{
			name:     "Nil source map",
			source:   nil,
			target:   map[string]any{"key1": "value1"},
			expected: map[string]any{"key1": "value1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeMaps(tt.source, tt.target)
			assert.Equal(t, tt.expected, result, "Result mismatch for test case: %s", tt.name)
		})
	}
}

func TestGetCPUUsage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name         string
		mockResponse []byte
		mockError    error
		expected     int64
		expectError  bool
	}{
		{
			name:         "Valid CPU usage",
			mockResponse: []byte("12.34"),
			mockError:    nil,
			expected:     12,
			expectError:  false,
		},
		{
			name:         "Error in executing command",
			mockResponse: nil,
			mockError:    errors.New("command failed"),
			expected:     0,
			expectError:  true,
		},
		{
			name:         "Invalid output format",
			mockResponse: []byte("invalid data"),
			mockError:    nil,
			expected:     0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockSyscmdI(ctrl)
			mockExecutor.EXPECT().ExecCmd(gomock.Any(), gomock.Any()).Return(tt.mockResponse, tt.mockError)

			ctx := context.Background()
			result := GetCPUUsage(ctx, mockExecutor)

			if tt.expectError {
				assert.Equal(t, tt.expected, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetMemUsage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name         string
		mockResponse []byte
		mockError    error
		expected     int64
		expectError  bool
	}{
		{
			name:         "Valid memory usage",
			mockResponse: []byte("42.5"),
			mockError:    nil,
			expected:     42, // Esperado arredondamento para int64
			expectError:  false,
		},
		{
			name:         "Error in executing command",
			mockResponse: nil,
			mockError:    errors.New("command failed"),
			expected:     0,
			expectError:  true,
		},
		{
			name:         "Invalid output format",
			mockResponse: []byte("invalid data"),
			mockError:    nil,
			expected:     0,
			expectError:  true,
		},
		{
			name:         "Empty output",
			mockResponse: []byte(""),
			mockError:    nil,
			expected:     0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockSyscmdI(ctrl)
			mockExecutor.EXPECT().ExecCmd(gomock.Any(), gomock.Any()).Return(tt.mockResponse, tt.mockError)

			ctx := context.Background()
			result := GetMemUsage(ctx, mockExecutor)

			if tt.expectError {
				assert.Equal(t, tt.expected, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetMapNumKinds(t *testing.T) {
	expected := map[reflect.Kind]bool{
		reflect.Int:     true,
		reflect.Int8:    true,
		reflect.Int16:   true,
		reflect.Int32:   true,
		reflect.Int64:   true,
		reflect.Float32: true,
		reflect.Float64: true,
	}

	result := GetMapNumKinds()
	assert.Equal(t, expected, result)
}
