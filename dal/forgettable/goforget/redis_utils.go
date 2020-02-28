package goforget

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/coyove/iis/dal/kv/cache"
	"github.com/gomodule/redigo/redis"
)

var (
	DistributionEmpty = fmt.Errorf("Distribution already empty, not updating")
)

type RedisServer struct {
	addr string
	pool *redis.Pool
}

func NewRedisServer(config *cache.RedisConfig) *RedisServer {
	rs := &RedisServer{
		addr: config.Addr,
	}
	return rs
}

func (rs *RedisServer) GetConnection() redis.Conn {
	return rs.pool.Get()
}

func (rs *RedisServer) Connect(maxIdle int) {
	// set up the connection pool
	rs.connectPool(maxIdle)

	// verify the connection pool is valid before allowing program to continue
	conn := rs.GetConnection()
	_, err := conn.Do("PING")
	if err != nil {
		log.Fatal("Could not connect to Redis!")
	}
	conn.Close()

}

func (rs *RedisServer) connectPool(maxIdle int) {
	rs.pool = &redis.Pool{
		MaxIdle:     maxIdle,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", rs.addr)
			if err != nil {
				return nil, err
			}
			if _, err := c.Do("SELECT", 1); err != nil {
				c.Close()
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func UpdateRedis(readChan chan *Distribution, id int) error {
	var redisConn redis.Conn
	for dist := range readChan {
		// log.Printf("[%d] Updating distribution: %s", id, dist.Name)

		redisConn = redisServer.GetConnection()
		err := UpdateDistribution(redisConn, dist)
		if err != nil && err != DistributionEmpty {
			log.Printf("[%d] Failed to update: %s: %v: %s", id, dist.Name, redisConn.Err(), err.Error())
		}
		redisConn.Close()
	}
	return nil
}

func UpdateDistribution(rconn redis.Conn, dist *Distribution) error {
	ZName := fmt.Sprintf("%s.%s", dist.Name, "_Z")
	TName := fmt.Sprintf("%s.%s", dist.Name, "_T")

	rconn.Send("WATCH", ZName)
	defer rconn.Send("UNWATCH")

	if dist.Full() == false {
		err := dist.Fill()
		if err != nil {
			return fmt.Errorf("Could not fill: %s", err)
		}
		dist.Decay()
		dist.Normalize()
	}

	maxCount := 0
	rconn.Send("MULTI")
	if dist.HasDecayed() == true {
		if dist.Z == 0 {
			rconn.Send("DISCARD")
			return DistributionEmpty
		}

		for k, v := range dist.Data {
			if v.Count == 0 {
				rconn.Send("ZREM", dist.Name, k)
			} else {
				rconn.Send("ZADD", dist.Name, v.Count, k)
				if v.Count > maxCount {
					maxCount = v.Count
				}
			}
		}

		rconn.Send("SET", ZName, dist.Z)
		rconn.Send("SET", TName, dist.T)
	} else {
		for _, v := range dist.Data {
			if v.Count != 0 && v.Count > maxCount {
				maxCount = v.Count
			}
		}
	}

	eta := math.Sqrt(float64(maxCount) / dist.Rate)
	if eta < 300 { // ~1 day
		eta = 300
	}

	const expirSigma = 2
	expTime := int((expirSigma + eta) * eta)

	rconn.Send("EXPIRE", dist.Name, expTime)
	rconn.Send("EXPIRE", ZName, expTime)
	rconn.Send("EXPIRE", TName, expTime)

	_, err := rconn.Do("EXEC")
	if err != nil {
		return fmt.Errorf("Could not update %s: %s", dist.Name, err)
	}
	return nil
}

func GetField(distribution string, fields ...string) ([]interface{}, error) {
	rdb := redisServer.GetConnection()

	rdb.Send("MULTI")
	for _, field := range fields {
		rdb.Send("ZSCORE", distribution, field)
	}
	rdb.Send("GET", fmt.Sprintf("%s.%s", distribution, "_Z"))
	rdb.Send("GET", fmt.Sprintf("%s.%s", distribution, "_T"))
	data, err := redis.MultiBulk(rdb.Do("EXEC"))
	return data, err
}

func GetNMostProbable(distribution string, N int) ([]interface{}, error) {
	rdb := redisServer.GetConnection()

	rdb.Send("MULTI")
	rdb.Send("ZREVRANGEBYSCORE", distribution, "+INF", "-INF", "WITHSCORES", "LIMIT", 0, N)
	rdb.Send("GET", fmt.Sprintf("%s.%s", distribution, "_Z"))
	rdb.Send("GET", fmt.Sprintf("%s.%s", distribution, "_T"))
	data, err := redis.MultiBulk(rdb.Do("EXEC"))
	return data, err
}

func IncrField(distribution string, fields []string, N int) error {
	rdb := redisServer.GetConnection()

	rdb.Send("MULTI")
	for _, field := range fields {
		rdb.Send("ZINCRBY", distribution, N, field)
	}
	rdb.Send("INCRBY", fmt.Sprintf("%s.%s", distribution, "_Z"), N*len(fields))
	rdb.Send("SETNX", fmt.Sprintf("%s.%s", distribution, "_T"), int(time.Now().Unix()))
	_, err := rdb.Do("EXEC")
	return err
}

func GetDistribution(distribution string) ([]interface{}, error) {
	rdb := redisServer.GetConnection()

	rdb.Send("MULTI")
	rdb.Send("GET", fmt.Sprintf("%s.%s", distribution, "_T"))
	rdb.Send("ZRANGE", distribution, 0, -1, "WITHSCORES")
	data, err := redis.MultiBulk(rdb.Do("EXEC"))
	return data, err
}

func DBSize() (int, error) {
	rdb := redisServer.GetConnection()

	data, err := redis.Int(rdb.Do("DBSIZE"))
	return data, err
}
