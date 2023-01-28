package dal

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

var Metrics struct {
	sync.RWMutex
	*bbolt.DB
}

var metricsDBOptions = &bbolt.Options{
	FreelistType: bbolt.FreelistMapType,
	NoSync:       true,
}

func startMetricsBackup() {
	sleep := time.Hour
	if os.Getenv("DEBUG") == "1" {
		sleep = time.Second * 5
	}

	go func() {
		for db := Metrics.DB; ; time.Sleep(sleep) {
			start := time.Now()
			func() {
				p := db.Path() + ".bak"
				os.Remove(p)

				bak, err := os.Create(p)
				if err != nil {
					logrus.Errorf("[Metrics] failed to open backup database: %v", err)
					return
				}
				defer bak.Close()

				err = db.View(func(tx *bbolt.Tx) error {
					_, err := tx.WriteTo(bak)
					return err
				})
				if err != nil {
					logrus.Errorf("[Metrics] failed to write metrics database: %v", err)
					return
				}
			}()
			logrus.Infof("[Metrics] backup metrics data in %v", time.Since(start))
		}
	}()
}

type MetricsKeyValue struct {
	Key   string
	Value float64
}

func MetricsSetAdd(set string, keys ...string) (count int64, err error) {
	err = Metrics.Update(func(tx *bbolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists([]byte("set"))
		if err != nil {
			return err
		}

		hll := types.HyperLogLog(bk.Get([]byte(set)))
		if len(hll) != types.HLLSize {
			hll = types.NewHyperLogLog()
		} else {
			hll = append([]uint8{}, hll...)
		}

		for _, k := range keys {
			hll.Add(uint32(ngram.StrHash(k)))
		}

		count = int64(hll.Count())
		return bk.Put([]byte(set), hll)
	})
	return
}

func MetricsSetCount(set string) (count int64) {
	Metrics.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("set"))
		if bk == nil {
			return nil
		}

		hll := types.HyperLogLog(bk.Get([]byte(set)))
		if len(hll) == types.HLLSize {
			count = int64(hll.Count())
		}
		return nil
	})
	return
}

func MetricsIncr(ns string, delta int64, data []MetricsKeyValue) error {
	return Metrics.Update(func(tx *bbolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists(strconv.AppendInt([]byte(ns), delta, 10))
		if err != nil {
			return err
		}

		a := 0
		for i, d := range data {
			kb := []byte(d.Key)
			old := types.BytesFloat64(bk.Get(kb))

			added, err := bk.TestPut(kb, types.Float64Bytes(old+d.Value))
			if err != nil {
				return err
			}

			if added {
				a++
			}

			data[i].Value = old + d.Value
		}
		return bk.SetSequence(bk.Sequence() + uint64(a))
	})
}

func MetricsView(ns string, delta int64, keys []string) (data []MetricsKeyValue) {
	Metrics.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(strconv.AppendInt([]byte(ns), delta, 10))
		if bk == nil {
			return nil
		}

		for _, k := range keys {
			kb := []byte(k)
			old := types.BytesFloat64(bk.Get(kb))
			data = append(data, MetricsKeyValue{k, old})
		}
		return nil
	})
	return
}

func MetricsSum(ns string, delta int64) (total float64) {
	Metrics.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(strconv.AppendInt([]byte(ns), delta, 10))
		if bk == nil {
			return nil
		}

		c := bk.Cursor()
		for k, v := c.First(); len(k) > 0; k, v = c.Next() {
			total += types.BytesFloat64(v)
		}
		return nil
	})
	return
}
