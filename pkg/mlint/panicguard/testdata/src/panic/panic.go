package panic

import "fmt"

// Test cases for nopanicinproduction analyzer

func badPanicWithString() {
	panic("something went wrong") // want "panic\\(\\) should not be used in production code"
}

func badPanicWithFormat() {
	x := 42
	panic(fmt.Sprintf("unexpected value: %d", x)) // want "panic\\(\\) should not be used in production code"
}

func badPanicWithError() {
	err := fmt.Errorf("error")
	panic(err) // want "panic\\(\\) should not be used in production code"
}
