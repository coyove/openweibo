package ch

import (
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sync"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/mq"
)

type Key [20]byte

func (k Key) String() string {
	return base32.HexEncoding.EncodeToString(k[:])
}

func (k Key) Replicas() (r []int32) {
	p := [4]byte{}
	r = make([]int32, 0)
	for i := 0; i < 3; i++ {
		copy(p[1:], k[i*3:i*3+3])
		x := int32(binary.BigEndian.Uint32(p[:]))
		if x > -1 {
			r = append(r, x)
		}
	}
	return
}

func (k *Key) SetReplicas(r [3]int32) {
	p := [4]byte{}
	for i, r := range r {
		binary.BigEndian.PutUint32(p[:], uint32(r))
		copy((*k)[i*3:i*3+3], p[1:])
	}
}

type Nodes struct {
	mu         sync.RWMutex
	nodes      []*driver.Node
	transferDB *mq.SimpleMessageQueue
}

func (ns *Nodes) LoadNodes(nodes []*driver.Node) {
	ns.mu.Lock()
	ns.nodes = nodes
	ns.mu.Unlock()
}

func (ns *Nodes) GetNode(name string) *driver.Node {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	for _, n := range ns.nodes {
		if n.Name == name {
			return n
		}
	}
	return nil
}

func (ns *Nodes) Put(v []byte) (string, error) {
	nodes := ns.Nodes()
	r := selectNodes(v, nodes)

	key := Key{}
	rand.Read(key[:])
	key.SetReplicas(r)
	k := key.String()

	for i := range r {
		if r[i] == -1 {
			continue
		}
		if err := nodes[r[i]].Put(k, v); err != nil {
			if i == len(r)-1 {
				return "", err // All nodes failed, so we have to return an error
			}
			continue // Try next node
		}

		// Replicate to the rest nodes, we don't care the result
		// If a node failed to put the key, it will eventually be transferred anyway
		for j := i + 1; j < len(r); j++ {
			if r[j] == -1 {
				continue
			}
			go nodes[r[j]].Put(k, v)
		}
		return k, nil
	}
	return "", fmt.Errorf("no node available")
}

func (ns *Nodes) Get(k string) ([]byte, error) {
	v, err := ns.get(k, false, false)
	return v, err
}

func (ns *Nodes) Delete(k string) error {
	_, err := ns.get(k, true, false)
	return err
}

func (ns *Nodes) get(key string, del bool, noTransfer bool) (v []byte, err error) {
	buf, _ := base32.HexEncoding.DecodeString(key)
	k := Key{}
	if len(buf) != len(k) {
		return nil, fmt.Errorf("invalid key: %s", key)
	}
	copy(k[:], buf)
	r := k.Replicas()
	if len(r) == 0 {
		return nil, fmt.Errorf("invalid key: %s, no replicas", key)
	}

	nodes := ns.Nodes()
	if del {
		for i := range r {
			// TODO: del
			nodes[r[i]].Delete(key)
		}
	} else {
		for i := range rand.Perm(len(r)) {
			if v, err = nodes[r[i]].Get(key); err == nil {
				return
			}
			if err == driver.ErrKeyNotFound && !noTransfer {
				if err := ns.transferDB.PushBack([]byte(fmt.Sprintf("%s@%s", key, nodes[r[i]].Name))); err != nil {
					log.Println("[nodes.get] push to transfer.db:", err)
				}
			}
		}
	}
	return
}

func (ns *Nodes) Nodes() []*driver.Node {
	ns.mu.RLock()
	n := make([]*driver.Node, len(ns.nodes))
	copy(n, ns.nodes)
	ns.mu.RUnlock()
	return n
}
