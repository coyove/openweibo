package dal

import (
	"flag"
	"os"
	"sync"

	"go.etcd.io/bbolt"
)

var metrics struct {
	sync.RWMutex
	*bbolt.DB
}

var metricsDBOptions = &bbolt.Options{
	FreelistType:   bbolt.FreelistMapType,
	NoFreelistSync: true,
	NoSync:         true,
}

var metricsSizeLimit = flag.Int64("metrics-size-limit", 100, "")

func MetricsView(f func(tx *bbolt.Tx) error) error {
	metrics.RLock()
	defer metrics.RUnlock()
	if metrics.DB == nil {
		return bbolt.ErrDatabaseNotOpen
	}
	return metrics.View(f)
}

func MetricsUpdate(f func(tx *bbolt.Tx) error) error {
	metrics.Lock()
	defer metrics.Unlock()
	if metrics.DB == nil {
		return bbolt.ErrDatabaseNotOpen
	}

	var size int64
	err := metrics.Update(func(tx *bbolt.Tx) error {
		size = tx.Size()
		return f(tx)
	})
	if size >= *metricsSizeLimit*1024*1024 {
		p := metrics.Path()
		metrics.Close()
		os.Remove(p)
		metrics.DB, err = bbolt.Open(p, 0777, metricsDBOptions)
	}
	return err
}
