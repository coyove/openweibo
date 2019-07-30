package node

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coyove/ch/driver/chmemory"
)

func TestNodesFuzzy(t *testing.T) {
	rand.Seed(time.Now().Unix())
	testNode = true

	nodes := []*Node{
		&Node{kv: &chmemory.Storage{}, Name: "aa", Weight: 10},
		&Node{kv: &chmemory.Storage{}, Name: "bb", Weight: 25},
		&Node{kv: &chmemory.Storage{}, Name: "cc", Weight: 10},
		&Node{kv: &chmemory.Storage{}, Name: "dd", Weight: 5},
	}

	mgr := &Nodes{}
	mgr.LoadNodes(nodes)

	m := sync.Map{}

	for i := 0; i < 1e3; i++ {
		wg := sync.WaitGroup{}
		for j := 0; j < 100; j++ {
			wg.Add(1)

			if rand.Intn(1000000) == 0 {
				nodes = append(nodes, &Node{
					Name:   strconv.Itoa(i*200000 + j),
					Weight: int64(rand.Intn(10) + 10),
				})
				mgr.LoadNodes(nodes)
			}

			go func() {
				k, v := fmt.Sprintf("%x", rand.Uint64()), fmt.Sprintf("%x", rand.Uint64())
				mgr.Put(k, []byte(v))
				m.Store(k, []byte(v))
				wg.Done()
			}()
		}
		wg.Wait()
		//log.Println(i)
	}

	for i := 0; i < 2; i++ {
		count := 0
		m.Range(func(k, v interface{}) bool {
			v2, err := mgr.Get(k.(string))
			if err != nil {
				panic(err)
			}
			if !bytes.Equal(v.([]byte), v2) {
				t.Fatal(v, v2)
			}
			count++
			log.Println(count)
			return true
		})
	}
}
