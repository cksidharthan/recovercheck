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
