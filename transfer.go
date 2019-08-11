package ch

import (
	"log"
	"strings"
	"time"

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
			time.Sleep(time.Second * 5)
			continue
		}

		parts := strings.Split(string(p.Value), "@")
		if !ns.transferKey(parts[0], parts[1]) {
			ns.transferDB.PushBack(p.Value)
		}
	}
}

func (ns *Nodes) transferKey(k string, to string) bool {
	v, err := ns.get(k, false, true)
	if err != nil {
		log.Println("[transfer] lost ", k, err)
		return true
	}

	toNode := ns.GetNode(to)
	if toNode == nil {
		log.Println("[transfer] FATAL: invalid to node name:", to, k)
		return true
	}

	if err := toNode.Put(k, v); err != nil {
		log.Println("[transfer] failed to put ", k, err)
		return false
	}

	return true
}
