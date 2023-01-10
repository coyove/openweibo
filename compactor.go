package main

import (
	"os"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

func compact(pCurrent, pTotal *int64) {
	oldPath := dal.TagsStore.DB.Path()
	tmpPath := oldPath + ".compacted"
	os.Remove(tmpPath)

	oldfi, err := os.Stat(oldPath)
	if err != nil {
		logrus.Fatal("[compactor] sys stat: ", err)
	}
	*pTotal = oldfi.Size()

	db, err := bbolt.Open(tmpPath, 0777, &bbolt.Options{})
	if err != nil {
		logrus.Fatal("[compactor] ", err)
	}

	exited := false
	go func() {
		for i := 0; !exited; i++ {
			time.Sleep(time.Second)
			fi, err := os.Stat(tmpPath)
			if err != nil {
				if !exited {
					logrus.Error("[compactor] daemon sys stat: ", err)
				}
				break
			}
			*pCurrent = fi.Size()
			if i%10 == 0 {
				logrus.Infof("[compactor] progress: %d / %d (%d%%)",
					*pCurrent, *pTotal, int(float64(*pCurrent)/float64(*pTotal)*100))
			}
		}
	}()

	if err := bbolt.Compact(db, dal.TagsStore.DB, 40960); err != nil {
		logrus.Fatal("[compactor] ", err)
	}

	db.Close()
	dal.TagsStore.DB.Close()

	tmpfi, err := os.Stat(tmpPath)
	if err != nil {
		logrus.Fatal("[compactor] sys stat: ", err)
	}

	if err := os.Remove(oldPath); err != nil {
		logrus.Fatal("[compactor] remove old: ", err)
	}

	if err := os.Rename(tmpPath, oldPath); err != nil {
		logrus.Fatal("[compactor] rename: ", err)
	}

	logrus.Infof("[compactor] original size: %d", *pTotal)
	logrus.Infof("[compactor] compacted size: %d", tmpfi.Size())

	db, err = bbolt.Open(oldPath, 0777, dal.BBoltOptions)
	if err != nil {
		logrus.Fatal("[compactor] rename: ", err)
	}

	dal.TagsStore.DB = db
	exited = true
}
