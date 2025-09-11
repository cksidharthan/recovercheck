package testdata

import "log"

func ExampleUnsafeGoroutine() {
	// This should be flagged - no recovery
	go func() {
		panic("This will crash")
	}()
}

func ExampleSafeGoroutine() {
	// This should NOT be flagged - has recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("Recovered:", r)
			}
		}()
		panic("This will be recovered")
	}()
}

func ExampleNamedFunction() {
	// This should NOT be flagged - named function assumed safe
	go someFunction()
}

func someFunction() {
	// Implementation details...
}
