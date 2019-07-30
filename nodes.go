package node

import (
	"log"
	"sync"
	"time"

	"github.com/coyove/ch/driver"
)

type Nodes struct {
	mu    sync.RWMutex
	nodes []*Node
}

func dupNodes(nodes []*Node) []*Node {
	n := make([]*Node, len(nodes))
	copy(n, nodes)
	return n
}

func removeFromNodes(nodes *[]*Node, node *Node) {
	for i := len(*nodes) - 1; i >= 0; i-- {
		if (*nodes)[i] == node {
			*nodes = append((*nodes)[:i], (*nodes)[i+1:]...)
			break
		}
	}
}

func (ns *Nodes) LoadNodes(nodes []*Node) {
	ns.mu.Lock()
	ns.nodes = dupNodes(nodes)
	ns.mu.Unlock()
}

func (ns *Nodes) Put(k string, v []byte) error {
	ns.mu.RLock()
	node := SelectNode(k, ns.nodes)
	ns.mu.RUnlock()
	return node.Put(k, v)
}

func (ns *Nodes) Get(k string) ([]byte, error) {
	v, err := ns.get(k, false)
	return v, err
}

func (ns *Nodes) Del(k string) error {
	_, err := ns.get(k, true)
	return err
}

func (ns *Nodes) get(k string, del bool) ([]byte, error) {
	ns.mu.RLock()
	nodes := dupNodes(ns.nodes)
	ns.mu.RUnlock()

	startNode := SelectNode(k, nodes)

	for i := 0; i < 10; i++ { // Retry 10 times at max
		node := SelectNode(k, nodes)
		v, err := node.Get(k)
		if err == driver.ErrKeyNotFound {
			if removeFromNodes(&nodes, node); len(nodes) == 0 {
				break
			}

			if testNode {
				log.Println("retry", node, nodes)
				time.Sleep(time.Millisecond * 300)
			}

			continue
		}

		if del && err == nil {
			return nil, node.Del(k)
		}

		if node != startNode {
			go transferKey(node, startNode, k, true)
		}

		return v, err
	}

	return nil, driver.ErrKeyNotFound
}
