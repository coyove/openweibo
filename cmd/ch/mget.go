package main

import (
	"github.com/etcd-io/bbolt"
)

func mget(tx *bbolt.Tx, noGet bool, res [][2][]byte) (a []*Article) {
	main := tx.Bucket(bkPost)
	for _, r := range res {
		p := &Article{}
		if noGet {
			if p.unmarshal(r[1]) == nil {
				a = append(a, p)
			}
		} else {
			if p.unmarshal(main.Get(r[1])) == nil {
				a = append(a, p)
			}
		}
	}
	return
}

func mgetReplies(pid []byte, start, end int) (a []*Article) {
	m.db.View(func(tx *bbolt.Tx) error {
		main := tx.Bucket(bkPost)
		buf := make([]byte, len(pid)+2)

		for id := start; id < end; id++ {
			p := &Article{}
			newReplyID(pid, uint16(id), buf)
			if p.unmarshal(main.Get(buf)) != nil || p.ID == nil {
				p.NotFound = true
				p.Index = int64(id)
			}
			a = append(a, p)
		}
		return nil
	})
	return
}
