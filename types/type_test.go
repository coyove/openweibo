package types

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/pierrec/lz4/v4"
)

func TestA(t *testing.T) {
	fmt.Println(RenderClip(`
    http://!<b%20href='b'%20style='font-size:100px'>sad</b>
    http://!<code>alert("aa") a
    `))
}

func BenchmarkB(b *testing.B) {
	data, _ := ioutil.ReadFile("doc.go")
	var sz int
	for i := 0; i < b.N; i++ {
		out := &bytes.Buffer{}
		w := gzip.NewWriter(out)
		w.Write(data)
		w.Close()
		sz = out.Len()
	}
	fmt.Println(sz)
}

func BenchmarkA(b *testing.B) {
	data, _ := ioutil.ReadFile("doc.go")
	var sz int
	for i := 0; i < b.N; i++ {
		out := &bytes.Buffer{}
		w := lz4.NewWriter(out)
		w.Apply(lz4.CompressionLevelOption(lz4.Fast))
		w.Write(data)
		w.Close()
		sz = out.Len()
	}
	fmt.Println(sz)
}
