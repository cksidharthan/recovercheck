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

func sameFileRecover() func() {
	return func() {
		if r := recover(); r != nil {
			log.Println("Recovered from panic:", r)
		}
	}
}

func anyName() func() {
	return func() {
		if r := recover(); r != nil {
			log.Println("Recovered from panic:", r)
		}
	}
}

func SafeGoroutine2() {
	go func() {
		defer pkg.PanicRecover()
		panic("oh no")
	}()
}

func SafeGoroutine3() {
	go func() {
		defer sameFileRecover()
		panic("oh no")
	}()
}

func SafeGoroutine4() {
	go func() {
		defer anyName()
		panic("oh no")
	}()
}
