package middleware

import "testing"

func TestPanic(t *testing.T) {
	foo()
}

func foo() {
	ThrowIf(true, "", "")
}
