package storage

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"sort"
	"unsafe"
)

const (
	lrLeft  = 1
	lrRight = 2

	blockSize = 256
)

type lr3 uint64

func lr3_push(v lr3, l byte) lr3 {
	if v > 1<<60 {
		panic("too deep")
	}
	return v*3 + lr3(l)
}

func lr3_last(v lr3) byte {
	return byte(v % 3)
}

func lr3_pop(v lr3) lr3 {
	return v / 3
}

type Node struct {
	Used  bool
	lr    lr3
	Start uint32
	Size  uint32
}

func (n Node) String() string {
	return fmt.Sprintf("[%d-%d]", n.Start, n.Start+n.Size)
}

type BinaryTree struct {
	s   []Node
	m   Bitmap
	buf []byte
}

func NewBlock(n int) *BinaryTree {
	sz := uint32(math.Pow(2, float64(n)))
	return &BinaryTree{
		s:   []Node{{Size: sz}},
		m:   NewBitmap(int(sz)),
		buf: make([]byte, sz*blockSize),
	}
}

func (t *BinaryTree) Alloc(n int) []byte {
	x := n / blockSize
	if x*blockSize != n {
		x++
	}

	start := t.m.FirstUnset(0)
	if start == -1 {
		return nil
	}

	buf := t.allocAt(uint32(x), start)
	if buf != nil {
		return buf[:n]
	}
	return buf
}

func (t *BinaryTree) allocAt(n uint32, i int) []byte {
	if n == 0 {
		return []byte{}
	}

	for i < len(t.s) {
		b := t.s[i]
		if b.Used || b.Size < uint32(n) {
			i = t.m.FirstUnset(i + 1)
			if i == -1 {
				break
			}
			continue
		}
		if b.Size == n || (b.Size > n && b.Size/2 < n) {
			t.s[i].Used = true

			offset := t.s[i].Start
			t.m.Set(int(offset), int(offset+n))
			return t.buf[offset*blockSize : (offset+n)*blockSize]
		}
		t.splitAt(i)
		return t.allocAt(n, i)
	}
	// No free space available
	return nil
}

func (t *BinaryTree) splitAt(i int) {
	n := t.s[i]
	if n.Size == 1 {
		panic("shouldn't happen")
	}

	t.s = append(t.s, Node{})
	copy(t.s[i+2:], t.s[i+1:])

	t.s[i] = Node{
		Size:  n.Size / 2,
		Start: n.Start,
		lr:    lr3_push(n.lr, lrLeft),
	}
	t.s[i+1] = Node{
		Size:  n.Size / 2,
		Start: n.Start + n.Size/2,
		lr:    lr3_push(n.lr, lrRight),
	}
}

func (t *BinaryTree) Free(buf []byte) {
	offset := uint32(((*reflect.SliceHeader)(unsafe.Pointer(&buf)).Data -
		(*reflect.SliceHeader)(unsafe.Pointer(&t.buf)).Data) / blockSize)

	i := sort.Search(len(t.s), func(i int) bool {
		return t.s[i].Start >= offset
	})

	if i >= len(t.s) {
		log.Println(t.s, offset)
		panic(fmt.Sprintf("not my buffer: %d", offset))
	}

	if t.s[i].Start != offset {
		panic(fmt.Sprintf("not my buffer: %x %x", offset, t.s[i].Start))
	}

	t.m.Set(int(t.s[i].Start), int(t.s[i].Start+t.s[i].Size))
	t.s[i].Used = false
	t.mergeAt(i)
}

func (t *BinaryTree) mergeAt(i int) bool {
	x := func(i int) {
		t.s[i].Size *= 2
		t.s[i].lr = lr3_pop(t.s[i].lr)
	}

	switch lr3_last(t.s[i].lr) {
	case lrLeft:
		if i == len(t.s)-1 {
			panic("shouldn't happen")
		}
		if !t.s[i+1].Used && t.s[i+1].Size == t.s[i].Size {
			t.s = append(t.s[:i+1], t.s[i+2:]...)
			x(i)
			return t.mergeAt(i)
		}
	case lrRight:
		if i == 0 {
			panic("shouldn't happen")
		}
		if !t.s[i-1].Used && t.s[i].Size == t.s[i-1].Size {
			t.s = append(t.s[:i], t.s[i+1:]...)
			x(i - 1)
			return t.mergeAt(i - 1)
		}
	}
	return false
}
