package ch

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/mq"
)

type Key [15]byte

func (k Key) String() string {
	return base32.HexEncoding.EncodeToString(k[:])
}

func (k *Key) FromString(key string) error {
	buf, _ := base32.HexEncoding.DecodeString(key)
	if len(buf) != len(*k) {
		return fmt.Errorf("invalid key: %v", key)
	}
	copy((*k)[:], buf)
	return nil
}

func (k Key) Replicas() (r []int16) {
	for i := 0; i < 3; i++ {
		x := int16(binary.BigEndian.Uint16(k[6+i*2:]))
		if x < 0 {
			continue
		}
		r = append(r, x)
	}
	return
}

func (k *Key) init(r [3]int16) {
	binary.BigEndian.PutUint16(k[6:], uint16(r[0]))
	binary.BigEndian.PutUint16(k[8:], uint16(r[1]))
	binary.BigEndian.PutUint16(k[10:], uint16(r[2]))
}

func (ns *Nodes) MakeKey(v []byte) Key {
	x := sha1.Sum(v)
	key := Key{}
	copy(key[:], x[:])
	key.init(selectNodes(v, ns.Nodes()))
	return key
}

type Nodes struct {
	mu         sync.RWMutex
	nodes      []*driver.Node
	transferDB *mq.SimpleMessageQueue
}

func (ns *Nodes) LoadNodes(nodes []*driver.Node) {
	if len(nodes) >= math.MaxInt16 {
		panic("too many nodes")
	}
	ns.mu.Lock()
	ns.nodes = nodes
	ns.mu.Unlock()
}

func (ns *Nodes) GetNode(i int) *driver.Node {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	if i >= len(ns.nodes) {
		return nil
	}
	return ns.nodes[i]
}

func (ns *Nodes) Put(v []byte) (string, error) {
	var (
		nodes   = ns.Nodes()
		key     = ns.MakeKey(v)
		k, r    = key.String(), key.Replicas()
		wg      sync.WaitGroup
		success bool
		failure error
	)

	for i := range r {
		if int(r[i]) >= len(nodes) {
			continue
		}
		wg.Add(1)
		go func(node *driver.Node) {
			if err := node.Put(k, v); err != nil {
				failure = err
			} else {
				success = true
			}
			wg.Done()
		}(nodes[r[i]])
	}
	wg.Wait()
	if success {
		return k, nil
	}
	return "", fmt.Errorf("no node available, last error: %v", failure)
}

func (ns *Nodes) Get(k interface{}) ([]byte, error) {
	v, err := ns.get(k, 'g', false)
	return v, err
}

func (ns *Nodes) Delete(k interface{}) error {
	_, err := ns.get(k, 'd', false)
	return err
}

func (ns *Nodes) get(key interface{}, del byte, noTransfer bool) (v []byte, err error) {
	k, ok := key.(Key)
	if !ok {
		if err := k.FromString(key.(string)); err != nil {
			return nil, err
		}
	} else {
		key = k.String()
	}

	nodes, r := ns.Nodes(), k.Replicas()
	if len(r) != 3 {
		return nil, fmt.Errorf("invalid key: %s, no replicas", key)
	}

	if del == 'd' {
		for i := range r {
			if int(r[i]) >= len(nodes) {
				continue
			}
			// TODO: del
			nodes[r[i]].Delete(key.(string))
		}
	} else {
		for i := range rand.Perm(len(r)) {
			if int(r[i]) >= len(nodes) {
				err = driver.ErrKeyNotFound
				continue
			}
			if v, err = nodes[r[i]].Get(key.(string)); err == nil {
				return
			}
			if err == driver.ErrKeyNotFound && !noTransfer {
				tkey := fmt.Sprintf("%v@%d", key, r[i])
				if err := ns.transferDB.PushBack([]byte(tkey)); err != nil {
					log.Println("[nodes.get] push to transfer.db:", err)
				}
			}
		}
	}
	// If we reach here, the last error will be returned
	return
}

func (ns *Nodes) Nodes() []*driver.Node {
	ns.mu.RLock()
	n := ns.nodes
	ns.mu.RUnlock()
	return n
}
