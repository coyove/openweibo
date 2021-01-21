package avatar

import (
	"bytes"
	"io/ioutil"
	"testing"
)

func TestGen(t *testing.T) {
	p := &bytes.Buffer{}
	Create(p, "test")
	ioutil.WriteFile("test.jpg", p.Bytes(), 0777)
}
