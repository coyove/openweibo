package driver

import (
	"fmt"
)

type Node struct {
	Name   string
	Weight int64
	KV
}

func (n *Node) String() string {
	return fmt.Sprintf("%s(w:%d,o:%d)", n.Name, n.Weight, n.Stat().ObjectCount)
}
