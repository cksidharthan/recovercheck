package example

import "log"

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

func panicRecover() func() {
	return func() {
		if r := recover(); r != nil {
			log.Println("Recovered from panic:", r)
		}
	}
}

func SafeGoroutine2() {
	go func() {
		defer panicRecover()
		panic("oh no")
	}()
}
