package assert

import (
	"testing"

	"github.com/shopspring/decimal"
)

// Benchmarks verify assertions are lightweight enough for always-on usage.
// Target: < 100ns for hot path (condition is true), zero allocations.

// --- Core Assertion Benchmarks (Hot Path) ---

func BenchmarkThat_True(b *testing.B) {
	for i := 0; i < b.N; i++ {
		That(true, "benchmark test")
	}
}

func BenchmarkThat_TrueWithContext(b *testing.B) {
	for i := 0; i < b.N; i++ {
		That(true, "benchmark test", "key1", "value1", "key2", 42)
	}
}

func BenchmarkNotNil_NonNil(b *testing.B) {
	v := "test"
	for i := 0; i < b.N; i++ {
		NotNil(v, "benchmark test")
	}
}

func BenchmarkNotNil_NonNilPointer(b *testing.B) {
	x := 42
	ptr := &x
	for i := 0; i < b.N; i++ {
		NotNil(ptr, "benchmark test")
	}
}

func BenchmarkNotEmpty_NonEmpty(b *testing.B) {
	s := "test"
	for i := 0; i < b.N; i++ {
		NotEmpty(s, "benchmark test")
	}
}

func BenchmarkNoError_NilError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NoError(nil, "benchmark test")
	}
}

// --- Predicate Benchmarks ---

func BenchmarkPositive(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Positive(int64(i + 1))
	}
}

func BenchmarkNonNegative(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NonNegative(int64(i))
	}
}

func BenchmarkInRange(b *testing.B) {
	for i := 0; i < b.N; i++ {
		InRange(5, 0, 10)
	}
}

func BenchmarkValidUUID(b *testing.B) {
	uuid := "123e4567-e89b-12d3-a456-426614174000"
	for i := 0; i < b.N; i++ {
		ValidUUID(uuid)
	}
}

func BenchmarkValidAmount(b *testing.B) {
	amount := decimal.NewFromFloat(1234.56)
	for i := 0; i < b.N; i++ {
		ValidAmount(amount)
	}
}

func BenchmarkPositiveDecimal(b *testing.B) {
	amount := decimal.NewFromFloat(1234.56)
	for i := 0; i < b.N; i++ {
		PositiveDecimal(amount)
	}
}

func BenchmarkValidScale(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidScale(8)
	}
}

// --- Helper Function Benchmarks ---

func BenchmarkIsNil_NonNil(b *testing.B) {
	v := "test"
	for i := 0; i < b.N; i++ {
		isNil(v)
	}
}

func BenchmarkIsNil_TypedNilPointer(b *testing.B) {
	var ptr *int = nil
	for i := 0; i < b.N; i++ {
		isNil(ptr)
	}
}

// --- Combined Usage Benchmarks ---

// BenchmarkTypicalAssertion simulates a typical assertion pattern.
func BenchmarkTypicalAssertion(b *testing.B) {
	id := "123e4567-e89b-12d3-a456-426614174000"
	amount := decimal.NewFromFloat(100.50)

	for i := 0; i < b.N; i++ {
		That(ValidUUID(id), "invalid id", "id", id)
		That(PositiveDecimal(amount), "invalid amount", "amount", amount)
	}
}
