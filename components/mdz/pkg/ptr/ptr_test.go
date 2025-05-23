package ptr

import "testing"

func TestStringPtr(t *testing.T) {
	tests := []string{"", "hello", "test string"}

	for _, test := range tests {
		result := StringPtr(test)
		if result == nil {
			t.Errorf("StringPtr should not return nil")
		}
		if *result != test {
			t.Errorf("StringPtr(%q) = %q, expected %q", test, *result, test)
		}
	}
}

func TestBoolPtr(t *testing.T) {
	tests := []bool{true, false}

	for _, test := range tests {
		result := BoolPtr(test)
		if result == nil {
			t.Errorf("BoolPtr should not return nil")
		}
		if *result != test {
			t.Errorf("BoolPtr(%t) = %t, expected %t", test, *result, test)
		}
	}
}

func TestFloat64Ptr(t *testing.T) {
	tests := []float64{0.0, 1.0, -1.0, 3.14159, 1e10}

	for _, test := range tests {
		result := Float64Ptr(test)
		if result == nil {
			t.Errorf("Float64Ptr should not return nil")
		}
		if *result != test {
			t.Errorf("Float64Ptr(%f) = %f, expected %f", test, *result, test)
		}
	}
}

func TestIntPtr(t *testing.T) {
	tests := []int{0, 1, -1, 42, 1000000}

	for _, test := range tests {
		result := IntPtr(test)
		if result == nil {
			t.Errorf("IntPtr should not return nil")
		}
		if *result != test {
			t.Errorf("IntPtr(%d) = %d, expected %d", test, *result, test)
		}
	}
}
