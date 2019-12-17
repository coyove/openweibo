package mv

import (
	"testing"
)

func TestFirstImage(t *testing.T) {
	t.Log(ExtractFirstImage("http://1.jp http://1.qjpg http://2.jpg"))
}

func BenchmarkUserUnmarshal(b *testing.B) {
	buf := (User{ID: "awdasd"}).Marshal()
	for i := 0; i < b.N; i++ {
		UnmarshalUser(buf)
	}
}
