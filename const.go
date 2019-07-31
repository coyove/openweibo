package node

import (
	"hash/fnv"
	"math"

	"github.com/coyove/ch/driver"
)

// Straw2
func SelectNode(k string, nodes []*driver.Node) *driver.Node {
	var maxNode = nodes[0]
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
