package example

import (
	"log"

	"github.com/cksidharthan/recovercheck/example/pkg"
)

func SafeGoroutine() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("Recovered from panic:", r)
			}
		}()
		panic("oh no")
	}()
}

func SafeGoroutine2() {
	go func() {
		defer pkg.PanicRecover()
		panic("oh no")
	}()
}
