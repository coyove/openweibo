package model

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coyove/common/lru"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
)

var (
	mtSize     = 100
	candSize   = 100
	pool       = lru.NewCache(1e4)
	dedupSec   = 60.0
	dedupCache = lru.NewCache(100)
)

// IDAuthor and IDTag do not have real meanings, they are just used to construct a valid ik.ID
func IndexUser(u *User) { Index("su", ik.NewID(ik.IDAuthor, u.ID), u.ID, u.CustomName) }
func IndexTag(t string) { Index("st", ik.NewID(ik.IDTag, t), t) }

func indexArticle(a *Article) {
	if a.PostOptions&PostOptionNoMasterTimeline != 0 ||
		a.PostOptions&PostOptionNoSearch != 0 {
		return
	}
	if !(len(a.ID) == 12 && a.ID[0] == 'S') {
		return
	}
	if a.Anonymous {
		Index("art", ik.ParseID(a.ID), a.Content)
	} else {
		Index("art", ik.ParseID(a.ID), a.Content, a.Author)
	}
}

func tf(v string) (m map[string]float64) {
	if len(v) == 0 {
		return m
	}

	m = map[string]float64{}
	r, l := utf8.DecodeRuneInString(v)
	if r != utf8.RuneError && l == len(v) {
		m[v] = 1
		return m
	}

	for len(m) < 512 {
		r, l1 := utf8.DecodeRuneInString(v)
		if r == utf8.RuneError || l1 == 0 || l1 == len(v) {
			break
		}
		_, l2 := utf8.DecodeRuneInString(v[l1:])
		m[strings.ToLower(v[:l1+l2])]++
		v = v[l1:]
	}

	for k, v := range m {
		m[k] = v / float64(len(m))
	}
	return m
}

func Index(ns string, id ik.ID, docs ...string) bool {
	if lastTime, ok := dedupCache.Get(id); ok && time.Since(lastTime.(time.Time)).Seconds() < dedupSec {
		return false
	}
	for _, doc := range docs {
		for k, score := range tf(doc) {
			key := ns + "-" + k
			if m, ok := pool.Get(key); ok {
				m.(*lru.Cache).Add(id, score)
			} else {
				c := lru.NewCache(int64(mtSize))
				c.Add(id, score)
				pool.Add(key, c)
			}
		}
	}
	dedupCache.Add(id, time.Now())
	return true
}

func Search(ns, query string, start, n int) ([]ik.ID, int) {
	// 	defer func(start time.Time) {
	// 		log.Println(time.Since(start))
	// 	}(time.Now())

	query = common.SoftTrunc(query, 32)
	if len(query) == 0 {
		return nil, 0
	}

	type pair struct {
		id    ik.ID
		score float64
	}

	slice := func(a []pair) []ik.ID {
		defer func() { recover() }()
		end := start + n
		if end > len(a) {
			end = len(a)
		}
		b := make([]ik.ID, end-start)
		for i := range b {
			b[i] = a[i+start].id
		}
		return b
	}

	iters := []*lru.Cache{}
	allKeys := map[ik.ID]struct{}{}
	for k := range tf(query) {
		if m, ok := pool.Get(ns + "-" + k); ok {
			mmt := m.(*lru.Cache)
			mmt.Info(func(k lru.Key, v interface{}, a, b int64) {
				allKeys[k.(ik.ID)] = struct{}{}
			})
			iters = append(iters, mmt)
		}
	}

	sorting := []pair{}
	for key := range allKeys {
		sorting = append(sorting, pair{key, 0})
	}

	if ns == "art" {
		sort.Slice(sorting, func(i, j int) bool { return sorting[i].id.Time().After(sorting[j].id.Time()) })
	} else {
		sort.Slice(sorting, func(i, j int) bool { return sorting[i].id.Less(sorting[j].id) })
	}

	if len(sorting) > candSize {
		sorting = sorting[:candSize]
	}

	for ii := range sorting {
		for _, i := range iters {
			if s, ok := i.Get(sorting[ii].id); ok {
				sorting[ii].score += s.(float64) * math.Max(math.Log2(float64(pool.Len())/float64(i.Len())), 0)
			}
		}
	}
	sort.Slice(sorting, func(i, j int) bool { return sorting[i].score > sorting[j].score })
	return slice(sorting), len(sorting)
}
