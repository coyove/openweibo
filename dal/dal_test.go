package dal

import (
	"testing"
)

func TestSet(t *testing.T) {
	a := 1
	var b *int
	setIfValid(&a, b)
	t.Log(a)

	b = &a
	*b++
	setIfValid(&a, b)
	t.Log(a)
}
