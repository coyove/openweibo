package node

import (
	"hash/fnv"
	"math"
)

// Straw2
func SelectNode(k string, nodes []*Node) *Node {
	var maxNode *Node = nodes[0]
	var maxStraw float64 = -math.MaxFloat64

	for _, n := range nodes {
		h := fnv.New64()
		h.Write([]byte(n.Name))
		h.Write([]byte(k))
		s := math.Log(float64(h.Sum64()&0xffff)/65536) / float64(n.Weight)
		if s >= maxStraw {
			maxNode = n
			maxStraw = s
		}
	}

	return maxNode
}
