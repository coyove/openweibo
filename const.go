package node

import (
	"errors"
	"hash/fnv"
	"math"
	"sort"
)

var (
	ErrKeyNotFound        = errors.New("key not found")
	ErrServiceUnavailable = errors.New("service unavailable")
)

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

	sort.SliceStable(straws, func(i, j int) bool {
		return straws[i].s > straws[j].s
	})

	return straws[0].n
}
