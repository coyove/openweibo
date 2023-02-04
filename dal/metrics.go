package dal

import (
	"encoding/binary"
	"os"
	"sync"
	"time"

	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

const MetricsDelta = 300

type metricsPair struct {
	ns    string
	value float64
}

var Metrics struct {
	sync.RWMutex
	*bbolt.DB
	pending chan metricsPair
}

var metricsDBOptions = &bbolt.Options{
	FreelistType: bbolt.FreelistMapType,
	NoSync:       true,
}

func startMetricsBackup() {
	Metrics.pending = make(chan metricsPair, 10000)
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

	var worker func(int)
	worker = func(c int) {
		defer func() {
			time.AfterFunc(time.Second, func() { worker(c + 1) })
		}()
		if Metrics.DB == nil {
			return
		}
		if len(Metrics.pending) > 0 {
			batchMetricsIncr()
		}
		if c%10 == 0 {
			stats := Metrics.Stats()
			Metrics.pending <- metricsPair{"dbacc:metrics:freepage", float64(stats.FreePageN)}
			Metrics.pending <- metricsPair{"dbacc:metrics:freealloc", float64(stats.FreeAlloc)}
			Metrics.pending <- metricsPair{"dbacc:metrics:freelistsize", float64(stats.FreelistInuse)}

			stats = Store.Stats()
			Metrics.pending <- metricsPair{"dbacc:data:freepage", float64(stats.FreePageN)}
			Metrics.pending <- metricsPair{"dbacc:data:freealloc", float64(stats.FreeAlloc)}
			Metrics.pending <- metricsPair{"dbacc:data:freelistsize", float64(stats.FreelistInuse)}
		}
	}
	worker(0)
}

func metricsMakeKey(ns string, typ string, delta int64) []byte {
	x := append([]byte(ns), typ...)
	x = append(x, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(x[len(x)-4:], uint32(delta))
	return x
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

func MetricsIncr(ns string, value float64) { Metrics.pending <- metricsPair{ns, value} }

func batchMetricsIncr() {
	err := Metrics.Update(func(tx *bbolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists([]byte("metrics"))
		if err != nil {
			return err
		}
		bk.FillPercent = 0.9

		round := 0
	MORE:
		select {
		case in := <-Metrics.pending:
			delta := clock.Unix() / MetricsDelta
			ns, value := in.ns, in.value
			kb := metricsMakeKey(ns, "_sum", delta)
			old := types.BytesFloat64(bk.Get(kb))
			if err := bk.Put(kb, types.Float64Bytes(old+value)); err != nil {
				return err
			}

			kb = metricsMakeKey(ns, "_ctr", delta)
			oldCtr := types.BytesUint64(bk.Get(kb))
			if err := bk.Put(kb, types.Uint64Bytes(oldCtr+1)); err != nil {
				return err
			}

			kb = metricsMakeKey(ns, "_max", delta)
			oldBuf := bk.Get(kb)
			if value > types.BytesFloat64(oldBuf) || len(oldBuf) == 0 {
				if err := bk.Put(kb, types.Float64Bytes(value)); err != nil {
					return err
				}
			}
			if round++; round < 1000 {
				goto MORE
			}
			logrus.Infof("[Metrics] batch worker too many pendings")
		default:
		}
		return nil
	})
	if err != nil {
		logrus.Errorf("[Metrics] batch worker commit error: %v", err)
	}
}

type MetricsIndex struct {
	Index int64
	Sum   float64
	Max   float64
	Avg   float64
	DAvg  float64
	Count uint64
}

func MetricsRange(ns string, start, end int64) (res []MetricsIndex) {
	Metrics.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte("metrics"))
		if bk == nil {
			return nil
		}

		for i := start; i <= end; i++ {
			total := types.BytesFloat64(bk.Get(metricsMakeKey(ns, "_sum", i)))
			num := types.BytesUint64(bk.Get(metricsMakeKey(ns, "_ctr", i)))
			mx := types.BytesFloat64(bk.Get(metricsMakeKey(ns, "_max", i)))

			avg := total / float64(num)
			if num == 0 {
				avg = 0
			}
			res = append(res, MetricsIndex{
				Index: i * MetricsDelta,
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

func MetricsCalcAccDAvg(in []MetricsIndex) []MetricsIndex {
	for i := 1; i < len(in); i++ {
		if in[i].Count == 0 {
			in[i].Avg = in[i-1].Avg
		}
		in[i].DAvg = (in[i].Avg - in[i-1].Avg) / float64(in[i].Index-in[i-1].Index)
	}
	return in
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
			k = k[:len(k)-8]
			ns = append(ns, string(k))
			k, _ = c.Seek(append(append([]byte{}, k...), 0xff))
		}
		return nil
	})
	return
}