package mq

import (
	"log"
	"os"
	"strconv"
	"testing"
)

func TestMQ(t *testing.T) {
	os.RemoveAll("1.db")
	db, err := New("1.db")
	if err != nil {
		panic(err)
	}
	for i := 0; i < 10000; i++ {
		if err := db.PushBack([]byte(strconv.Itoa(i))); err != nil {
			panic(err)
		}

	}

	//db.db.View(func(tx *bolt.Tx) error {
	//	bk := tx.Bucket(bkName)
	//	return bk.ForEach(func(k []byte, v []byte) error {
	//		log.Println(k, v)
	//		return nil
	//	})
	//})

	for {
		b, err := db.PopFront()
		if err != nil || len(b) == 0 {
			log.Println(err)
			break
		}
		log.Println(string(b))
	}

	db.Close()
	os.RemoveAll("1.db")
}
