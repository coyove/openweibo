package tfidf

import (
	"log"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/coyove/iis/dal/kv/cache"
	"github.com/garyburd/redigo/redis"
)

var client *redis.Pool

func Init(config *cache.RedisConfig) {
	options := []redis.DialOption{}

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

	client = redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", config.Addr, options...)
	}, config.MaxIdle)
}

func tf(v string) map[string]float64 {
	m := map[string]float64{}
	if len(v) == 0 {
		return m
	}

	rs := []rune(v)
	if len(rs) == 1 {
		m[v] = 1
		return m
	}

	for i := 0; i < len(rs)-1; i++ {
		m[string(rs[i:i+2])]++
	}
	for k, v := range m {
		m[k] = v / float64(len(rs))
	}
	return m
}

func simpleget(key string) (string, error) {
	c := client.Get()
	v, err := redis.String(c.Do("GET", key))
	c.Close()
	if err == redis.ErrNil {
		err = nil
	}
	return v, err
}

func Index(ns, id, doc string) {
	olddoc, err := simpleget(ns + "-" + id)
	if err != nil {
		log.Println("[Index] get old doc:", err)
	}

	if len(doc) == 0 && len(olddoc) == 0 {
		return
	}

	pipe := client.Get()
	defer func() {
		if err := pipe.Flush(); err != nil {
			log.Println("[Index] flush err:", err)
		}
		pipe.Close()
	}()

	if len(olddoc) >= 0 {
		for k := range tf(doc) {
			pipe.Send("ZREM", ns+"-"+k, id)
		}
	}
	if len(doc) >= 0 {
		for k, v := range tf(doc) {
			pipe.Send("ZADD", ns+"-"+k, v, id)
		}
		pipe.Send("SET", ns+"-"+id, doc)
	}
}

func Search(ns, query string, start, n int) ([]string, error) {
	if len(query) == 0 {
		return nil, nil
	}

	redisKeys := []string{}
	for k := range tf(query) {
		redisKeys = append(redisKeys, ns+"-"+k)
	}

	pipe := client.Get()
	defer pipe.Close()

	var err error
	sizes := []int64{}
	if func() {
		pipe := client.Get()
		defer pipe.Close()
		for _, k := range redisKeys {
			pipe.Send("ZCARD", k)
		}
		pipe.Flush()

		var s int64
		for range redisKeys {
			s, err = redis.Int64(pipe.Receive())
			sizes = append(sizes, s)
		}
	}(); err != nil && err != redis.ErrNil {
		return nil, err
	}

	tempKey := ns + strconv.FormatInt(time.Now().UnixNano()^rand.Int63(), 36)
	args := []interface{}{tempKey, 0}
	idfs := []interface{}{}
	for i := range sizes {
		if sizes[i] == 0 {
			continue
		}
		args = append(args, redisKeys[i])
		idfs = append(idfs, math.Max(math.Log2(1e8/float64(sizes[i])), 0))
		args[1] = args[1].(int) + 1
	}

	if len(idfs) == 0 {
		return nil, nil
	}

	pipe.Do("ZUNIONSTORE", append(append(args, "WEIGHTS"), idfs...)...)
	ids, err := redis.Strings(pipe.Do("ZREVRANGE", tempKey, start, start+n-1))
	pipe.Do("DEL", tempKey)
	return ids, err
}
