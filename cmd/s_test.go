package main

import (
	"strconv"
	"testing"

	"github.com/etcd-io/bbolt"
)

func TestZZZ(t *testing.T) {
	db, _ := bbolt.Open("1.db", 0777, nil)
	db.Update(func(tx *bbolt.Tx) error {
		bk, _ := tx.CreateBucketIfNotExists([]byte("zzz"))
		for i := 0; i < 10000; i++ {
			bk.CreateBucketIfNotExists([]byte(strconv.Itoa(i)))
		}
		for i := 0; i < 10000; i++ {
			bk.Put([]byte("k"+strconv.Itoa(i)), []byte{})
		}
		return nil
	})
	db.Close()
}

func BenchmarkZZZ(b *testing.B) {
	db, _ := bbolt.Open("1.db", 0777, nil)
	for i := 0; i < b.N; i++ {
		db.View(func(tx *bbolt.Tx) error {
			bk := tx.Bucket([]byte("zzz"))
			bk.Cursor().Seek([]byte("k10"))
			return nil
		})
	}
	db.Close()
}

func BenchmarkZZZ2(b *testing.B) {
	db, _ := bbolt.Open("1.db", 0777, nil)
	for i := 0; i < b.N; i++ {
		db.View(func(tx *bbolt.Tx) error {
			bk := tx.Bucket([]byte("zzz"))
			bk.Cursor().Seek([]byte("k1000"))
			return nil
		})
	}
	db.Close()
}
