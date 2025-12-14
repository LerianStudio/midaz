package recover

import "fmt"

// Test cases for norecoveroutsideboundary analyzer

func badRecoverInBusinessLogic() {
	defer func() {
		if r := recover(); r != nil { // want "recover\\(\\) is only allowed in boundary packages"
			fmt.Println("recovered:", r)
		}
	}()
	mayPanic()
}

func badRecoverInHelper() {
	defer func() {
		recover() // want "recover\\(\\) is only allowed in boundary packages"
	}()
}

func mayPanic() {
	// placeholder
}
