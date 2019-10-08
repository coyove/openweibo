package main

import (
	"github.com/coyove/iis/cmd/ch/id"
	"github.com/etcd-io/bbolt"
)

func (m *Manager) mget(tx *bbolt.Tx, noGet bool, res [][2][]byte) (a []*Article) {
	main := tx.Bucket(bkPost)
	for _, r := range res {
		p := &Article{}
		if noGet {
			if len(r[1]) > 30 && p.unmarshal(r[1]) == nil {
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

func (m *Manager) mgetReplies(parent []byte, start, end int) (a []*Article) {
	m.db.View(func(tx *bbolt.Tx) error {
		main := tx.Bucket(bkPost)

		for i := start; i < end; i++ {
			if i <= 0 {
				continue
			}

			pid := id.ParseID(parent)
			pid.RIndexAppend(int16(i))
			pid.SetHeader(id.HeaderReply)

			p := &Article{}
			if p.unmarshal(main.Get(pid.Marshal())) != nil || p.ID == nil {
				p.NotFound = true
				p.Index = int64(i)
			}
			a = append(a, p)
		}
		return nil
	})
	return
}
