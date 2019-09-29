package main

import (
	"encoding/binary"

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

func mgetReplies(pid []byte, ids []int64) (a []*Article) {
	m.db.View(func(tx *bbolt.Tx) error {
		main := tx.Bucket(bkPost)
		buf := make([]byte, 1+len(pid)+2)
		copy(buf[1:], pid)

		for _, id := range ids {
			p := &Article{}
			binary.BigEndian.PutUint16(buf[len(buf)-2:], uint16(id))

			if p.unmarshal(main.Get(buf)) == nil {
				a = append(a, p)
			}
		}
		return nil
	})
	return
}
