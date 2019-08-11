package ch

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/mq"
)

type Key [20]byte

func (k Key) String() string {
	return base32.HexEncoding.EncodeToString(k[:])
}

func (k Key) Replicas() (r []int32) {
	x := binary.BigEndian.Uint64(k[:])
	for i := 1; i <= 43; i += 21 {
		r = append(r, int32(x<<uint(i)>>43))
	}
	return
}

func (k *Key) SetReplicas(r [3]int32) {
	x := uint64(r[0]&0x1fffff)<<42 + uint64(r[1]&0x1fffff)<<21 + uint64(r[2]&0x1fffff)
	binary.BigEndian.PutUint64(k[:], x)
}

func (k Key) Time() time.Time {
	return time.Unix(int64(binary.BigEndian.Uint32(k[8:])), 0)
}

func (k *Key) SetTime() {
	binary.BigEndian.PutUint32((*k)[8:], uint32(time.Now().Unix()))
}

type Nodes struct {
	mu         sync.RWMutex
	nodes      []*driver.Node
	transferDB *mq.SimpleMessageQueue
	Cooldown   time.Duration
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

func (ns *Nodes) MakeKey(v []byte) string {
	nodes := ns.Nodes()
	r := selectNodes(v, nodes)

	key := Key(sha1.Sum(v))
	key.SetReplicas(r)
	key.SetTime()
	return key.String()
}

func (ns *Nodes) Put(v []byte) (string, error) {
	var (
		nodes   = ns.Nodes()
		r       = selectNodes(v, nodes)
		k       = ns.MakeKey(v)
		wg      sync.WaitGroup
		success bool
		failure error
	)

	for i := range r {
		if r[i] == -1 {
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
	if ns.Cooldown != 0 && time.Since(k.Time()) < ns.Cooldown {
		return nil, fmt.Errorf("invalid key: %s, cooldown", key)
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
