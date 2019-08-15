package ch

import (
	"crypto/sha1"
	"hash/crc32"
	"math"

	"github.com/coyove/iis/driver"
)

const (
	selectCount = 3
)

func selectNodes(v []byte, nodes []*driver.Node) [selectCount]int16 {
	var (
		h    = crc32.NewIEEE()
		w    = sha1.Sum(v)
		list [selectCount]struct {
			node  int16
			straw float64
		}
	)

	for i := range list {
		list[i].node = -1
	}

	for i := range list {
		list[i].straw = -math.MaxFloat64
	}

	for ni, n := range nodes {
		offline, total, used := n.Space()
		if int64(len(v)) > total-used || offline {
			continue
		}

		h.Reset()
		h.Write([]byte(n.Name))
		h.Write(w[:])

		s := math.Log(float64(h.Sum32()&0xffff)/65536) * 1024 * 1024 / float64(total-used)

		for i := range list {
			if s > list[i].straw {
				copy(list[i+1:], list[i:])
				list[i].node = int16(ni)
				list[i].straw = s
				break
			}
		}

	}

	res := [selectCount]int16{}
	next := len(nodes)

	for i, l := range list {
		res[i] = l.node
		if l.node == -1 {
			res[i] = int16(next)
			next++
		}
	}
	return res
}
