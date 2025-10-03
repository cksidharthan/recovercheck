package recovercheck

import "testing"

func TestUnsafeGoroutine(t *testing.T) {
	// This goroutine should be flagged as unsafe
	go func() { // want "goroutine created without panic recovery"
		panic("test panic")
	}()
}

func TestAnotherUnsafeGoroutine(t *testing.T) {
	// Another unsafe goroutine
	go func() { // want "goroutine created without panic recovery"
		// No recovery logic here
		println("unsafe goroutine")
	}()
}
