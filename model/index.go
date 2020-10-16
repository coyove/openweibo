package model

import (
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coyove/common/lru"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal/kv"

	"github.com/gomodule/redigo/redis"
)

var (
	client         *redis.Pool
	bleveFastCache *lru.Cache
	searchCache    *lru.Cache
	bleveWorker    chan *Article
)

func Init(config *kv.RedisConfig) {
	client = kv.NewGlobalCache(config).Pool
	bleveFastCache = lru.NewCache(10240)
	searchCache = lru.NewCache(100)
	bleveWorker = make(chan *Article, 1024)
	go indexArticleWorker()
}

func indexArticle(a *Article) {
	if a.PostOptions&PostOptionNoMasterTimeline != 0 {
		return
	}
	if a.PostOptions&PostOptionNoSearch != 0 {
		return
	}
	if !(len(a.ID) == 12 && a.ID[0] == 'S') {
		return
	}

	select {
	case bleveWorker <- a:
	default:
	}
}

func indexArticleWorker() {
	for a := range bleveWorker {
		if _, ok := bleveFastCache.Get(a.ID); ok {
			continue
		}
		Index("art", a.ID, a.Content, a.Author)
		bleveFastCache.Add(a.ID, true)
	}
}

func SearchArticle(q string, timeout time.Duration, start, limit int) ([]string, int, error) {
	return Search("art", q, start, limit)
}

func tf(v string) (m map[string]float64) {
	if len(v) == 0 {
		return m
	}

	m = map[string]float64{}
	r, l := utf8.DecodeRuneInString(v)
	if r != utf8.RuneError && l == len(v) {
		m[v] = 1
		return m
	}

	for len(m) < 512 {
		r, l1 := utf8.DecodeRuneInString(v)
		if r == utf8.RuneError || l1 == 0 || l1 == len(v) {
			break
		}
		_, l2 := utf8.DecodeRuneInString(v[l1:])
		m[strings.ToLower(v[:l1+l2])]++
		v = v[l1:]
	}

	for k, v := range m {
		m[k] = v / float64(len(m))
	}
	return m
}

func Index(ns, id string, docs ...string) {
	if len(docs) == 0 || len(docs[0]) == 0 {
		return
	}

	pipe := client.Get()
	defer func() {
		if err := pipe.Flush(); err != nil {
			log.Println("[Index] flush err:", err)
		}
		pipe.Close()
	}()

	for _, doc := range docs {
		for k, v := range tf(doc) {
			redisKey := ns + "-" + k
			pipe.Send("ZREMRANGEBYRANK", redisKey, 0, -100)
			pipe.Send("ZADD", redisKey, v, id)
		}
	}
}

func Search(ns, query string, start, n int) ([]string, int, error) {
	if len(query) == 0 {
		return nil, 0, nil
	}
	query = common.SoftTrunc(query, 32)

	ret := func(ids []string) []string {
		defer func() { recover() }()
		max := start + n
		if max > len(ids) {
			max = len(ids)
		}
		return ids[start:max]
	}

	if cached, ok := searchCache.Get(ns + query); ok {
		x := cached.([]string)
		return ret(x), len(x), nil
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
		return nil, 0, err
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
		return nil, 0, nil
	}

	pipe.Do("ZUNIONSTORE", append(append(args, "WEIGHTS"), idfs...)...)
	ids, err := redis.Strings(pipe.Do("ZREVRANGE", tempKey, 0, -1))
	// total, _ := redis.Int(pipe.Do("ZCARD", tempKey))
	pipe.Do("DEL", tempKey)
	searchCache.Add(ns+query, ids)
	return ret(ids), len(ids), err
}
