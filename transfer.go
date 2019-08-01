package ch

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chbolt"
	"github.com/coyove/common/sched"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ltime | log.Ldate)
	sched.Verbose = false
}

func (ns *Nodes) StartTransferAgent(path string) {
	ns.transferDB = chbolt.NewNode("transfer", path, 1)
	ns.transferDaemon()
}

func (ns *Nodes) transferDaemon() {
	type item struct {
		k        string
		from, to *driver.Node
	}

	items := []*item{}
	ns.transferDB.KV.(*chbolt.Storage).DB().View(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte("transfer"))
		if bk == nil {
			return nil
		}
		return bk.ForEach(func(k, v []byte) error {
			names := strings.Split(string(v), "|")
			items = append(items, &item{
				k:    string(k),
				from: ns.NodeByName(names[0]),
				to:   ns.NodeByName(names[1]),
			})
			if len(items) > 100 {
				return errors.New("abort")
			}
			return nil
		})
	})

	if len(items) > 0 {
		log.Println("[transfer.daemon] transfer:", len(items))
		for _, item := range items {
			ns.transferKey(item.from, item.to, item.k)
		}
	}

	sched.Schedule(func() {
		go ns.transferDaemon()
	}, time.Minute)
}

func (ns *Nodes) transferKey(fromNode, toNode *driver.Node, k string) bool {
	errx := func(err error) bool {
		log.Println("[transfer]", fromNode.Name, "->", toNode.Name, "key:", k, "err:", err)
		if err := ns.transferDB.Put(k, []byte(fromNode.Name+"|"+toNode.Name)); err != nil {
			log.Println("[transfer.DB]", err)
		}
		return false
	}

	v, err := fromNode.Get(k)
	if err == driver.ErrKeyNotFound {
		goto OK
	}
	if err != nil {
		return errx(err)
	}
	if err := toNode.Put(k, v); err != nil {
		return errx(err)
	}
	if err := fromNode.Delete(k); err != nil {
		return errx(err)
	}

OK:
	ns.transferDB.Delete(k)
	return true
}
