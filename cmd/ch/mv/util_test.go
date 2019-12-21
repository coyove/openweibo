package mv

import (
	"testing"
)

func TestFirstImage(t *testing.T) {
	t.Log(ExtractFirstImage("http://1.jp http://1.qjpg http://2.jpg"))
	t.Log(ExtractFirstImage("[img]http://1.jp[/img] http://1.qjpg http://2.jpg"))
}

func BenchmarkUserUnmarshal(b *testing.B) {
	buf := (User{ID: "awdasd"}).Marshal()
	for i := 0; i < b.N; i++ {
		UnmarshalUser(buf)
	}
}

func TestSearchUsers(t *testing.T) {
	UnmarshalUser([]byte(`{"ID":"aaa"}`))
	UnmarshalUser([]byte(`{"ID":"bbb"}`))
	UnmarshalUser([]byte(`{"ID":"aabb"}`))
	UnmarshalUser([]byte(`{"ID":"coyove"}`))
	t.Log(SearchUsers("coyv", 3))
}
