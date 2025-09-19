package recovercheck

import aliaspkg "recovercheck/pkg"

// SafeGoroutineWithAlias uses a recovery function from another package with import alias
func SafeGoroutineWithAlias() {
	go func() {
		defer aliaspkg.PanicRecover()
		panic("oh no")
	}()
}
