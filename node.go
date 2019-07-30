package node

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"sync"
)

var testNode = false

type Node struct {
	Name   string
	Weight int64
	kv     sync.Map
}

func (n *Node) Put(k string, v string) error {
	n.kv.Store(k, v)
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

func SelectNode(k string, nodes []*Node) *Node {
	straws := make([]struct {
		s float64
		n *Node
	}, len(nodes))

	for i, n := range nodes {
		h := fnv.New64()
		h.Write([]byte(n.Name))
		h.Write([]byte(k))
		straws[i].s = math.Log(float64(h.Sum64()&0xffff)/65536) / float64(n.Weight)
		straws[i].n = n
	}

	sort.Slice(straws, func(i, j int) bool {
		return straws[i].s > straws[j].s
	})

	return straws[0].n
}
