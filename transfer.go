package ch

import (
	"log"
	"strings"
	"time"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/mq"
	"github.com/coyove/common/sched"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ltime | log.Ldate)
	sched.Verbose = false
}

func (ns *Nodes) StartTransferAgent(path string) {
	var err error
	ns.transferDB, err = mq.New(path)
	if err != nil {
		panic(err)
	}
	go ns.transferDaemon()
}

func (ns *Nodes) transferDaemon() {
	for {
		p, err := ns.transferDB.PopFront()
		if err == mq.ErrEmptyQueue {
			time.Sleep(time.Second * 10)
			continue
		}

		parts := strings.Split(string(p), "@")
		if !ns.transferKey(parts[0], parts[1]) {
			ns.transferDB.PushBack(p)
		}
	}
}

func (ns *Nodes) transferKey(k string, from string) bool {
	ns.mu.RLock()
	toNode := SelectNode(k, ns.nodes)
	fromNode := ns.NodeByName(from)
	ns.mu.RUnlock()

	if fromNode == nil {
		log.Println("[transfer] invalid from node name:", from, k)
		return false
	}

	log.Println("[transfer] from:", from, ", to:", toNode.Name, ", key:", k)

	v, err := fromNode.Get(k)
	if err == driver.ErrKeyNotFound {
		goto OK
	}
	if err != nil {
		log.Println("[transfer] get:", fromNode.Name, "key:", k, "err:", err)
		return false
	}
	if err := toNode.Put(k, v); err != nil {
		log.Println("[transfer] put:", toNode.Name, "key:", k, "err:", err)
		return false
	}
	if err := fromNode.Delete(k); err != nil {
		log.Println("[transfer] delete:", fromNode.Name, "key:", k, "err:", err)
		return false
	}

OK:
	return true
}
