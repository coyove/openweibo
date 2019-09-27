package main

import "github.com/etcd-io/bbolt"

func mget(tx *bbolt.Tx, res [][2][]byte) (a []*Article) {
	main := tx.Bucket(bkPost)
	for _, r := range res {
		p := &Article{}
		if p.Unmarshal(main.Get(r[1])) == nil {
			a = append(a, p)
		}
	}
	return
}
