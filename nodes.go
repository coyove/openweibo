package ch

import (
	"log"
	"sync"

	"github.com/coyove/ch/driver"
)

var (
	testNode    = false
	testRetries = 0
	Retries     = 4
)

type Nodes struct {
	mu         sync.RWMutex
	nodes      []*driver.Node
	transferDB *driver.Node
}

func dupNodes(nodes []*driver.Node) []*driver.Node {
	n := make([]*driver.Node, len(nodes))
	copy(n, nodes)
	return n
}

func removeFromNodes(nodes *[]*driver.Node, node *driver.Node) {
	for i := len(*nodes) - 1; i >= 0; i-- {
		if (*nodes)[i] == node {
			*nodes = append((*nodes)[:i], (*nodes)[i+1:]...)
			break
		}
	}
}

func (ns *Nodes) LoadNodes(nodes []*driver.Node) {
	ns.mu.Lock()
	ns.nodes = dupNodes(nodes)
	ns.mu.Unlock()
}

func (ns *Nodes) NodeByName(name string) *driver.Node {
	for _, n := range ns.nodes {
		if n.Name == name {
			return n
		}
	}
	return nil
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

func (ns *Nodes) Delete(k string) error {
	_, err := ns.get(k, true)
	return err
}

func (ns *Nodes) get(k string, del bool) ([]byte, error) {
	ns.mu.RLock()
	nodes := dupNodes(ns.nodes)
	ns.mu.RUnlock()

	startNode := SelectNode(k, nodes)
	retriedNodes := []*driver.Node{}

	if testNode {
		defer func() {
			testRetries = len(retriedNodes) + 1
		}()
	}

	for i := 0; i < Retries; i++ {
		node := SelectNode(k, nodes)
		v, err := node.Get(k)
		if err == driver.ErrKeyNotFound {
			if removeFromNodes(&nodes, node); len(nodes) == 0 {
				break
			}

			if testNode {
				retriedNodes = append(retriedNodes, node)
				log.Println("retry", retriedNodes)
				//time.Sleep(time.Millisecond * 300)
			}

			continue
		}

		if err == nil {
			if del {
				return nil, node.Delete(k)
			}
			if node != startNode {
				go ns.transferKey(node, startNode, k)
			}
		}
		return v, err
	}

	return nil, driver.ErrKeyNotFound
}
