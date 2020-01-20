package dal

import (
	"testing"
)

func TestNewRequest(t *testing.T) {
	t.Log(*NewRequest("Test", "A", 1).TestRequest.A)
}

func BenchmarkNewRequest(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewRequest("UpdateUserKimochi", "ID", "zzz", "Kimochi", byte(12))
	}
}
