package common

import (
	"sort"
)

type usCache [65536][16]rune

var (
	userCache usCache
	tagCache  usCache
)

func AddUserToSearch(id string) {
	addImpl(&userCache, id)
}

func AddTagToSearch(id string) {
	addImpl(&tagCache, id)
}

func addImpl(cache *usCache, id string) {
	hash := Hash32(id)
	bs := [16]rune{}
	copy(bs[:], []rune(id))
	(*cache)[hash%uint32(len(*cache))] = bs
}

func SearchUsers(id string, n int) []string {
	return searchImpl(&userCache, id, n)
}

func SearchTags(id string, n int) []string {
	return searchImpl(&tagCache, id, n)
}

func searchImpl(cache *usCache, id string, n int) []string {
	m := map[uint32]bool{}
	idr := []rune(id)
	rtos := func(r [16]rune) string {
		for j, c := range r {
			if c == 0 {
				return string(r[:j])
			}
		}
		return string(r[:])
	}

	if len(idr) == 1 {
		res := make([]string, 0, n)
		for _, id := range cache {
			if id == [16]rune{} {
				continue
			}
			for _, r := range id {
				if r == idr[0] {
					res = append(res, rtos(id))
					break
				}
			}
			if len(res) == n {
				break
			}
		}
		return res
	}

	if len(idr) < 2 {
		return nil
	}

	lower := func(r rune) uint32 {
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		return uint32(uint16(r))
	}

	for i := 0; i < len(idr)-1; i++ {
		a, b := lower(idr[i]), lower(idr[i+1])
		m[a<<16|b] = true
	}

	bigram := func(a [16]rune) int {
		s := 0
		for i := 0; i < len(a)-1; i++ {
			v := lower(a[i])<<16 | lower(a[i+1])
			if v == 0 {
				break
			}
			if m[v] {
				s++
			}
		}
		return s
	}

	scores := make([]struct {
		res   [16]rune
		score int
	}, n)

	for _, id := range cache {
		if id == [16]rune{} {
			continue
		}
		s := bigram(id)
		i := sort.Search(n, func(i int) bool {
			return scores[i].score >= s
		})
		if i < len(scores) {
			scores = append(scores[:i+1], scores[i:]...)
		} else {
			scores = append(scores, scores[0])
		}
		scores[i].res = id
		scores[i].score = s
		scores = scores[1:]
	}

	res := make([]string, 0, n)
	for i := len(scores) - 1; i >= 0; i-- {
		if scores[i].score == 0 || scores[i].res == [16]rune{} {
			break
		}
		res = append(res, rtos(scores[i].res))
	}
	return res
}
