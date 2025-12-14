package goroutine

import "fmt"

// Test cases for norawgoroutine analyzer

func badRawGoroutineAnonymous() {
	go func() { // want "raw goroutine detected"
		fmt.Println("bad")
	}()
}

func badRawGoroutineNamed() {
	go doSomething() // want "raw goroutine detected"
}

func doSomething() {
	fmt.Println("doing something")
}

func badRawGoroutineWithArgs() {
	x := 1
	go func(n int) { // want "raw goroutine detected"
		fmt.Println(n)
	}(x)
}
