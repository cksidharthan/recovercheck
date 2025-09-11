package example

import "time"

func UnsafeFunction() {
	// This should be flagged - unsafe goroutine
	go func() {
		panic("This will crash the program")
	}()

	// Another unsafe goroutine
	go func() {
		time.Sleep(1 * time.Second)
		panic("Another unrecovered panic")
	}()
}

func SafeFunction() {
	// This should NOT be flagged - has recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Handle panic
			}
		}()
		panic("This will be recovered")
	}()
}
