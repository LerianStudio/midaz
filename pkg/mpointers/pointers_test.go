package mpointers

import (
	"testing"
	"time"
)

func TestString(t *testing.T) {
	s := "test"
	result := String(s)
	if *result != s {
		t.Errorf("String() = %v, want %v", *result, s)
	}
}

func TestBool(t *testing.T) {
	b := true
	result := Bool(b)
	if *result != b {
		t.Errorf("Bool() = %v, want %v", *result, b)
	}
}

func TestTime(t *testing.T) {
	t1 := time.Now()
	result := Time(t1)
	if !result.Equal(t1) {
		t.Errorf("Time() = %v, want %v", result, t1)
	}
}

func TestInt64(t *testing.T) {
	num := int64(42)
	result := Int64(num)
	if *result != num {
		t.Errorf("Int64() = %v, want %v", *result, num)
	}
}

func TestInt(t *testing.T) {
	num := 42
	result := Int(num)
	if *result != num {
		t.Errorf("Int() = %v, want %v", *result, num)
	}
}
