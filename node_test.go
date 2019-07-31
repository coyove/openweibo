package ch

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chmemory"
)

func TestNodesFuzzy(t *testing.T) {
	rand.Seed(time.Now().Unix())
	testNode = true

	nodes := []*driver.Node{
		chmemory.NewNode("aa", 10),
		chmemory.NewNode("bb", 25),
		chmemory.NewNode("cc", 10),
		chmemory.NewNode("dd", 5),
	}

	mgr := &Nodes{}
	mgr.LoadNodes(nodes)

	m := sync.Map{}

	for i := 0; i < 1e3; i++ {
		wg := sync.WaitGroup{}
		for j := 0; j < 100; j++ {
			wg.Add(1)

			if rand.Intn(10000) == 0 {
				//if i == 1 && j == 1 {
				nodes = append(nodes, chmemory.NewNode(strconv.Itoa(i*200000+j), int64(rand.Intn(10)+10)))
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

	Retries = len(nodes)

	for i := 0; i < 2; i++ {
		count := 0
		avgTries := 0

		m.Range(func(k, v interface{}) bool {
			v2, err := mgr.Get(k.(string))
			if err != nil {
				panic(err)
			}
			if !bytes.Equal(v.([]byte), v2) {
				t.Fatal(v, v2)
			}
			count++
			//log.Println(count)
			avgTries += testRetries
			return true
		})

		log.Println(avgTries, count, float64(avgTries)/float64(count))
	}

	log.Println(nodes)
}
