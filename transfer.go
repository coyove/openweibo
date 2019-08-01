package ch

import (
	"log"
	"time"

	"github.com/boltdb/bolt"
	"github.com/coyove/ch/driver"
	"github.com/coyove/common/sched"
)

var (
	transferDB *bolt.DB
	tbkName    = []byte("transfer")
)

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ltime | log.Ldate)
	sched.Verbose = false

	var err error
	transferDB, err = bolt.Open("transfer.db", 0644, nil)
	if err != nil {
		panic(err)
	}

	transferDaemon()
}

func transferDaemon() {
	//	err := transferDB.Update(func(tx *bolt.Tx) error {
	//		bk, err := tx.CreateBucketIfNotExists(tbkName)
	//		if err != nil {
	//			return err
	//		}
	//		return bk.ForEach(func(k, v []byte) error {
	//
	//		})
	//	})
	//	if err != nil {
	//		log.Println("[transferDaemon.DB]", err)
	//	}
	//
	sched.Schedule(transferDaemon, time.Minute)
}

func transferKey(fromNode, toNode *driver.Node, k string, addToDBIfFailed bool) bool {
	delFailed := false
	errx := func(err error) bool {
		log.Println("[transfer]", err)

		if addToDBIfFailed {
			err = transferDB.Update(func(tx *bolt.Tx) error {
				bk, err := tx.CreateBucketIfNotExists(tbkName)
				if err != nil {
					return err
				}

				if delFailed {
					// No need to transfer, just delete the key in fromNode
					return bk.Put([]byte(k), []byte("delonly|"+fromNode.Name))
				}

				return bk.Put([]byte(k), []byte(fromNode.Name+"|"+toNode.Name))

			})
			if err != nil {
				log.Println("[transfer.DB]", err)
			}
		}

		return false
	}

	v, err := fromNode.Get(k)
	if err == driver.ErrKeyNotFound {
		return true
	}
	if err != nil {
		return errx(err)
	}
	if err := toNode.Put(k, v); err != nil {
		return errx(err)
	}
	if err := fromNode.Delete(k); err != nil {
		delFailed = true
		return errx(err)
	}
	return true
}
