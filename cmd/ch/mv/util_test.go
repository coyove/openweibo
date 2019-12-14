package mv

import "testing"

func TestFirstImage(t *testing.T) {
	t.Log(ExtractFirstImage("http://1.jp http://1.qjpg http://2.jpg"))
}
