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

func mget2(ids []int64) (a []*Article) {
	m.db.View(func(tx *bbolt.Tx) error {
		main := tx.Bucket(bkPost)
		for _, id := range ids {
			p := &Article{}
			if p.unmarshal(main.Get(idBytes(id))) == nil {
				a = append(a, p)
			}
		}
		return nil
	})
	return
}
