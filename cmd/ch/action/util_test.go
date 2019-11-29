package action

import "testing"

func TestSanUsername(t *testing.T) {
	t.Log(sanUsername("一二三12四"))
}
