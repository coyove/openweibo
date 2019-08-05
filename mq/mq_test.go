package mq

import (
	"log"
	"os"
	"strconv"
	"testing"
)

func TestMQ(t *testing.T) {
	os.Remove("1.db")
	db, err := New("1.db")
	if err != nil {
		panic(err)
	}
	for i := 0; i < 1000; i++ {
		if err := db.PushBack([]byte(strconv.Itoa(i))); err != nil {
			panic(err)
		}
		if i%100 == 0 {
			log.Println(i)
		}
	}

	//db.db.View(func(tx *bolt.Tx) error {
	//	bk := tx.Bucket(bkName)
	//	return bk.ForEach(func(k []byte, v []byte) error {
	//		log.Println(k, v)
	//		return nil
	//	})
	//})
	log.Println(db.FirstN(1))

	i := 0
	for {
		b, _, err := db.PopFront()
		if err != nil || len(b) == 0 {
			log.Println(err)
			break
		}
		i++
		log.Println(string(b))
	}

	log.Println("===", i)

	db.Close()
	os.RemoveAll("1.db")
}
