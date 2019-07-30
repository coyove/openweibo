package node

import (
	"fmt"

	"github.com/coyove/ch/driver"
)

var testNode = false

type Node struct {
	Name   string
	Weight int64
	kv     driver.KV
}

func (n *Node) Put(k string, v []byte) error {
	return n.kv.Put(k, v)
}

func (n *Node) Get(k string) ([]byte, error) {
	return n.kv.Get(k)
}

func (n *Node) Del(k string) error {
	return n.kv.Delete(k)
}

func (n *Node) String() string {
	return fmt.Sprintf("%s(+%d)", n.Name, n.Weight)
}
