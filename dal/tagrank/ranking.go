package tagrank

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/coyove/iis/dal/kv"

	"github.com/gomodule/redigo/redis"
)

var p *redis.Pool
var TooOld = time.Hour * 24 * 7

func Init(redisConfig *kv.RedisConfig) {
	p = kv.NewGlobalCache(redisConfig).Pool
}

func Update(tag string, createTime time.Time, totalCount int) {
	c := p.Get()
	defer c.Close()

	if time.Since(createTime) > TooOld {
		return
	}

	score := float64(createTime.Unix())/45000 + math.Log10(float64(totalCount+1))

	if _, err := c.Do("ZADD", "tagrank", score, tag); err != nil {
		log.Println("[tagrank] set:", tag, "score:", score, "error:", err)
	}
}

func TopN(n int) []string {
	c := p.Get()
	defer c.Close()

	curMark := float64(time.Now().Unix()) / 45000
	c.Do("ZREMRANGEBYSCORE", "tagrank", "-inf", fmt.Sprintf("(%f", curMark-TooOld.Seconds()/45000))
	c.Do("ZREMRANGEBYRANK", "tagrank", "0", "-1000")

	res, err := redis.Strings(c.Do("ZREVRANGE", "tagrank", 0, n-1))
	if err != nil {
		log.Println("[tagrank] topn error:", err)
		return nil
	}

	return res
}
