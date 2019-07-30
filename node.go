package node

import (
	"fmt"
	"sync"
)

var testNode = false

type Node struct {
	Name   string
	Weight int64
	kv     sync.Map
}

func (n *Node) Put(k string, v string) error {

	return nil
}

func (n *Node) Get(k string) (string, error) {
	v, ok := n.kv.Load(k)
	if ok {
		return v.(string), nil
	}
	return "", ErrKeyNotFound
}

func (n *Node) Del(k string) error {
	n.kv.Delete(k)
	return nil
}

func (n *Node) String() string {
	return fmt.Sprintf("%s(+%d)", n.Name, n.Weight)
}
