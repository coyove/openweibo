package dal

import (
	"encoding/binary"
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
	sleep := time.Hour * 6
	// if os.Getenv("DEBUG") == "1" {
	// 	sleep = time.Second * 5
	// }

	var do func()
	do = func() {
		defer func() {
			time.AfterFunc(sleep, do)
		}()

		start := time.Now()
		p := Metrics.Path() + ".bak"
		os.Remove(p)

		bak, err := os.Create(p)
		if err != nil {
			logrus.Errorf("[Metrics] failed to open backup database: %v", err)
			return
		}
		defer bak.Close()

		err = Metrics.View(func(tx *bbolt.Tx) error {
			_, err := tx.WriteTo(bak)
			return err
		})
		if err != nil {
			logrus.Errorf("[Metrics] failed to write metrics database: %v", err)
			return
		}
		logrus.Infof("[Metrics] backup metrics data in %v", time.Since(start))
	}
	time.AfterFunc(sleep, do)
}

type MetricsKeyValue struct {
	Key   string
	Value float64
}

func MetricsSetAdd(set string, key string) (count int64, err error) {
	err = Metrics.Update(func(tx *bbolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists([]byte("set"))
		if err != nil {
			return err
		}

		h := uint32(ngram.StrHash(key))

		hll := types.HyperLogLog(bk.Get([]byte(set)))
		if len(hll) == types.HLLSize {
			hll = append([]uint8{}, hll...)
		} else if len(hll) < types.HLLSize {
			count = int64(len(hll) / 4)
			for i := 0; i < len(hll); i += 4 {
				if h == binary.BigEndian.Uint32(hll[i:]) {
					return nil
				}
			}

			if count >= 128 {
				tmp := types.NewHyperLogLog()
				for i := 0; i < len(hll); i += 4 {
					tmp.Add(binary.BigEndian.Uint32(hll[i:]))
				}
				hll = tmp
				goto ADD
			}

			hll = append(hll, 0, 0, 0, 0)
			binary.BigEndian.PutUint32(hll[len(hll)-4:], h)
			count++
			return bk.Put([]byte(set), hll)
		} else {
			hll = types.NewHyperLogLog()
		}

	ADD:
		hll.Add(h)
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
		} else {
			count = int64(len(hll) / 4)
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
