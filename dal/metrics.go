package dal

import (
	"bytes"
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

func MetricsIncr(ns string, delta int64, value float64) (new float64, err error) {
	err = Metrics.Update(func(tx *bbolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists([]byte("metrics"))
		if err != nil {
			return err
		}

		kb := strconv.AppendInt(append([]byte(ns), "_acc_"...), delta, 10)
		old := types.BytesFloat64(bk.Get(kb))

		added, err := bk.TestPut(kb, types.Float64Bytes(old+value))
		if err != nil {
			return err
		}

		a := 0
		if added {
			a = 1
		}

		{
			kb := strconv.AppendInt(append([]byte(ns), "_ctr_"...), delta, 10)
			oldCtr := types.BytesUint64(bk.Get(kb))
			if err := bk.Put(kb, types.Uint64Bytes(oldCtr+1)); err != nil {
				return err
			}
		}

		{
			kb := strconv.AppendInt(append([]byte(ns), "_max_"...), delta, 10)
			old := bk.Get(kb)
			if value > types.BytesFloat64(old) || len(old) == 0 {
				if err := bk.Put(kb, types.Float64Bytes(value)); err != nil {
					return err
				}
			}
		}

		new = old + value
		return bk.SetSequence(bk.Sequence() + uint64(a))
	})
	return
}

type MetricsIndex struct {
	Index int64
	Sum   float64
	Max   float64
	Avg   float64
	Count uint64
}

func MetricsRange(ns string, start, end int64) (res []MetricsIndex) {
	Metrics.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("metrics"))
		if bk == nil {
			return nil
		}

		for i := start; i <= end; i++ {
			k := append([]byte(ns), "_acc_"...)
			total := types.BytesFloat64(bk.Get(strconv.AppendInt(k, i, 10)))

			c := append([]byte(ns), "_ctr_"...)
			num := types.BytesUint64(bk.Get(strconv.AppendInt(c, i, 10)))

			m := append([]byte(ns), "_max_"...)
			mx := types.BytesFloat64(bk.Get(strconv.AppendInt(m, i, 10)))

			avg := total / float64(num)
			if num == 0 {
				avg = 0
			}
			res = append(res, MetricsIndex{
				Index: i,
				Sum:   total,
				Avg:   avg,
				Max:   mx,
				Count: num,
			})
		}
		return nil
	})
	return
}

func MetricsListNamespaces() (ns []string) {
	Metrics.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("metrics"))
		if bk == nil {
			return nil
		}

		c := bk.Cursor()
		k, _ := c.First()

		for len(k) > 0 {
			idx := bytes.LastIndexByte(k, '_')
			k = k[:idx]
			idx = bytes.LastIndexByte(k, '_')
			k = k[:idx]

			ns = append(ns, string(k))

			k, _ = c.Seek(append(append([]byte{}, k...), 0xff))
		}
		return nil
	})
	return
}
