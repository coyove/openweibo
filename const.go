package ch

import (
	"crypto/sha1"
	"hash/crc32"
	"math"

	"github.com/coyove/ch/driver"
)

const selectCount = 3

func selectNodes(v []byte, nodes []*driver.Node) [selectCount]int32 {
	var (
		h    = crc32.NewIEEE()
		w    = sha1.Sum(v)
		list [selectCount]struct {
			node  int32
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
				list[i].node = int32(ni)
				list[i].straw = s
				break
			}
		}

	}

	res := [selectCount]int32{}
	for i, l := range list {
		res[i] = int32(l.node)
	}
	return res
}
