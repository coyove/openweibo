package kv

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
)

type batchGetTask struct {
	key    string
	rValue []byte
	rOk    bool
	rDone  chan struct{}
}

type GlobalCache struct {
	Pool  *redis.Pool
	batch chan *batchGetTask
}

type RedisConfig struct {
	Addr         string        `yaml:"Addr"`
	Timeout      time.Duration `yaml:"Timeout"`
	MaxIdle      int           `yaml:"MaxIdle"`
	BatchWorkers int
}

func NewGlobalCache(config *RedisConfig) *GlobalCache {
	gc := &GlobalCache{}

	var options []redis.DialOption

	if config.Timeout == 0 {
		config.Timeout = time.Millisecond * 100
	}

	options = append(options, redis.DialConnectTimeout(config.Timeout))
	options = append(options, redis.DialReadTimeout(config.Timeout))
	options = append(options, redis.DialWriteTimeout(config.Timeout))

	if config.MaxIdle == 0 {
		config.MaxIdle = 10
	}

	if config.BatchWorkers == 0 {
		config.BatchWorkers = 1
	}

	gc.Pool = redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", config.Addr, options...)
	}, config.MaxIdle)

	gc.batch = make(chan *batchGetTask, 1024)

	for i := 0; i < config.BatchWorkers; i++ {
		go func() {
			tasks := []*batchGetTask{}
			blocking := false

			for {

				if blocking {
					t := <-gc.batch
					tasks = append(tasks, t)
				} else {
					for exit := false; !exit; {
						select {
						case t := <-gc.batch:
							tasks = append(tasks, t)
							if len(tasks) >= 16 {
								exit = true
							}
						default:
							exit = true
						}
					}
				}

				if len(tasks) == 0 {
					blocking = true
					continue
				}

				blocking = false

				keys := make([]interface{}, len(tasks))
				for i := range tasks {
					keys[i] = tasks[i].key
				}

				c := gc.Pool.Get()
				res, err := redis.Strings(c.Do("MGET", keys...))
				c.Close()

				if err != nil {
					log.Println("[GlobalCache_redis] batch get:", keys, "error:", err)
					for _, t := range tasks {
						t.rOk = false
						t.rDone <- struct{}{}
					}
				} else {
					for i, t := range tasks {
						t.rValue = []byte(res[i])

						if bytes.HasSuffix(t.rValue, []byte("$")) {
							t.rValue = t.rValue[:len(t.rValue)-1]
							t.rOk = true
						} else {
							t.rValue = nil
							t.rOk = false
						}

						t.rDone <- struct{}{}
					}
				}

				tasks = tasks[:0]
			}
		}()
	}

	return gc
}

func (gc *GlobalCache) Get(k string) ([]byte, bool) {
	// defer func(a time.Time) {
	// 	log.Println(time.Since(a))
	// }(time.Now())
	task := &batchGetTask{
		key:   k,
		rDone: make(chan struct{}, 1),
	}

	gc.batch <- task
	<-task.rDone
	return task.rValue, task.rOk
}

func (gc *GlobalCache) Add(k string, v []byte) error {
	c := gc.Pool.Get()
	defer c.Close()

	if _, err := c.Do("SET", k, append(v, '$')); err != nil {
		log.Println("[GlobalCache_redis] set:", k, "value:", string(v), "error:", err)
		return fmt.Errorf("cache error")
	}
	return nil
}

func (gc *GlobalCache) MGet(keys ...string) map[string][]byte {
	m := map[string][]byte{}
	c := gc.Pool.Get()
	defer c.Close()

	var args []interface{}
	for _, k := range keys {
		args = append(args, k)
	}

	res, err := redis.Strings(c.Do("MGET", args...))
	if err != nil {
		log.Println("[GlobalCache_redis] mget:", keys, "error:", err)
		return m
	}

	for i, r := range res {
		if strings.HasSuffix(r, "$") {
			m[keys[i]] = []byte(r[:len(r)-1])
		}
	}
	return m
}
