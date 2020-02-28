package goforget

import (
	"log"

	"github.com/coyove/iis/dal/kv/cache"
)

const rate = 0.5

var updateChan chan *Distribution
var redisServer *RedisServer

func Incr(dist string, fields ...string) error {
	if redisServer == nil {
		return nil
	}

	err := IncrField(dist, fields, 1)
	if err != nil {
		return err
	}
	updateChan <- &Distribution{Name: dist}
	return nil
}

func TopN(dist string, n int) Distribution {
	if redisServer == nil {
		return Distribution{}
	}

	if n == 0 {
		n = 10
	}

	result := Distribution{
		Name:  dist,
		Rate:  rate,
		Prune: true,
	}
	result.GetNMostProbable(n)
	result.Decay()

	updateChan <- &result

	return result
}

func Init(config *cache.RedisConfig) {
	redisServer = NewRedisServer(config)
	// create the connection pool
	redisServer.Connect(config.BatchWorkers * 2)

	log.Printf("Starting %d update worker(s)", config.BatchWorkers)

	updateChan = make(chan *Distribution, config.BatchWorkers)
	for i := 0; i < config.BatchWorkers; i++ {
		go func(idx int) {
			UpdateRedis(updateChan, idx)
		}(i)
	}
}
