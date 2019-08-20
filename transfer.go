package ch

import (
	"fmt"
	"log"
	"time"

	"github.com/coyove/common/sched"
	"github.com/coyove/iis/mq"
)

func init() {
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

		var k string
		var i int
		fmt.Sscanf("%s@%d", string(p.Value), &k, &i)

		if !ns.transferKey(k, i) {
			ns.transferDB.PushBack(p.Value)
		}
	}
}

func (ns *Nodes) transferKey(k string, to int) bool {
	v, err := ns.get(k, 'g', true)
	if err != nil {
		log.Println("[transfer] lost ", k, err)
		return true
	}

	toNode := ns.GetNode(to)
	if toNode == nil {
		log.Println("[transfer] FATAL: invalid to node:", to, k)
		return true
	}

	if err := toNode.Put(k, v); err != nil {
		log.Println("[transfer] failed to put ", k, err)
		return false
	}

	return true
}
