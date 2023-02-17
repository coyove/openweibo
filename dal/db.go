package dal

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/coyove/iis/disklru"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	//sync "github.com/sasha-s/go-deadlock"
)

var (
	Store struct {
		*bbolt.DB
		*bitmap.Manager
		S3         *s3manager.Uploader
		ImageCache *disklru.DiskLRU
		locks      [256]sync.Mutex
	}

	BBoltOptions = &bbolt.Options{
		FreelistType: bbolt.FreelistMapType,
	}

	NoteBK = "notes"
	UserBK = "users"
)

func InitDB() {
	var err error
	s3 := types.Config.S3
	sess, err := session.NewSession(&aws.Config{
		Endpoint:    &s3.Endpoint,
		Region:      &s3.Region,
		Credentials: credentials.NewStaticCredentials(s3.AccessKey, s3.SecretKey, ""),
		HTTPClient: &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				MaxConnsPerHost: 200,
			},
		},
	})
	if err != nil {
		logrus.Fatal("init s3 manager: ", err)
	}
	Store.S3 = s3manager.NewUploader(sess)

	Store.Manager, err = bitmap.NewManager("data/index",
		types.Config.Index.SwitchThreshold, bitmap.NewLRUCache(types.Config.Index.CacheSize))
	if err != nil {
		logrus.Fatal("init bitmap manager: ", err)
	}

	Store.DB, err = bbolt.Open("data/tags.db", 0777, BBoltOptions)
	if err != nil {
		logrus.Fatal("init store database: ", err)
	}
	Store.DB.Update(func(*bbolt.Tx) error { return nil })

	Metrics.DB, err = bbolt.Open("data/metrics.db", 0777, metricsDBOptions)
	if err != nil {
		logrus.Fatal("init metrics database: ", err)
	}
	Metrics.DB.Update(func(*bbolt.Tx) error { return nil })
	startMetricsBackup()

	Store.ImageCache, err = disklru.New("lru_cache",
		int(types.Config.ImageCache.MaxFiles), time.Duration(types.Config.ImageCache.PurgerInterval),
		imageS3Loader)
	if err != nil {
		logrus.Fatal("init image DiskLRU: ", err)
	}
}

type KeySortValue struct {
	Key    []byte
	Sort0  uint64
	Sort1  []byte
	Value  []byte
	NoSort bool
}

func (ksv KeySortValue) sort0Key() []byte {
	return append(types.Uint64Bytes(ksv.Sort0), ksv.Key...)
}

func (ksv KeySortValue) sort1Key() []byte {
	return append(append(append(ksv.Sort1, 0), ksv.Key...), byte(len(ksv.Key)))
}

func (ksv KeySortValue) String() string {
	return fmt.Sprintf("ksv(%x, %d, %v, %s)", ksv.Key, ksv.Sort0, ksv.Sort1, ksv.Value)
}

func KSVFromNote(tag *types.Note) KeySortValue {
	return KeySortValue{
		Key:    types.Uint64Bytes(tag.Id),
		Sort0:  uint64(tag.UpdateUnix),
		Sort1:  []byte(tag.Title),
		Value:  tag.MarshalBinary(),
		NoSort: tag.IsBio,
	}
}

func KSVUpsert(tx *bbolt.Tx, bkPrefix string, ksv KeySortValue) error {
	if len(ksv.Key) == 0 {
		return fmt.Errorf("null key not allowed")
	}
	if len(ksv.Key) >= 256 {
		return fmt.Errorf("key exceeds 255 bytes")
	}

	keyValue, err := tx.CreateBucketIfNotExists([]byte(bkPrefix + "_kv"))
	if err != nil {
		return err
	}
	sort0Key, err := tx.CreateBucketIfNotExists([]byte(bkPrefix + "_s0k"))
	if err != nil {
		return err
	}
	sort1Key, err := tx.CreateBucketIfNotExists([]byte(bkPrefix + "_s1k"))
	if err != nil {
		return err
	}
	keySortSort2, err := tx.CreateBucketIfNotExists([]byte(bkPrefix + "_kss"))
	if err != nil {
		return err
	}
	if !ksv.NoSort {
		oldSort := keySortSort2.Get(ksv.Key)
		if len(oldSort) >= 8 {
			deleteKSS(sort0Key, sort1Key, KeySortValue{
				Key:   ksv.Key,
				Sort0: types.BytesUint64(oldSort[:8]),
				Sort1: oldSort[8:],
			})
		}

		if added, err := sort0Key.TestPut(ksv.sort0Key(), nil); err != nil {
			return err
		} else if added {
			sort0Key.SetSequence(sort0Key.Sequence() + 1)
		}

		if len(ksv.Sort1) > 0 {
			if added, err := sort1Key.TestPut(ksv.sort1Key(), nil); err != nil {
				return err
			} else if added {
				sort1Key.SetSequence(sort1Key.Sequence() + 1)
			}
		}
		if err := keySortSort2.Put(ksv.Key, append(types.Uint64Bytes(ksv.Sort0), ksv.Sort1...)); err != nil {
			return err
		}
	} else {
		keyValue.FillPercent = 0.9
	}

	if added, err := keyValue.TestPut(ksv.Key, ksv.Value); err != nil {
		return err
	} else if added {
		keyValue.SetSequence(keyValue.Sequence() + 1)
	}
	return nil
}

func deleteKSS(sort0Key, sort1Key *bbolt.Bucket, old KeySortValue) error {
	if deleted, err := sort0Key.TestDelete(old.sort0Key()); err != nil {
		return err
	} else if deleted {
		sort0Key.SetSequence(sort0Key.Sequence() - 1)
	}
	if deleted, err := sort1Key.TestDelete(old.sort1Key()); err != nil {
		return err
	} else if deleted {
		sort1Key.SetSequence(sort1Key.Sequence() - 1)
	}
	return nil
}

func KSVGet(tx *bbolt.Tx, bkPrefix string, key []byte) (value []byte, err error) {
	keyValue := tx.Bucket([]byte(bkPrefix + "_kv"))
	if keyValue == nil {
		return nil, nil
	}
	return append([]byte{}, keyValue.Get(key)...), nil
}

func KSVExist(tx *bbolt.Tx, bkPrefix string, key []byte) (ok bool) {
	if tx == nil {
		Store.View(func(tx *bbolt.Tx) error {
			ok = KSVExist(tx, bkPrefix, key)
			return nil
		})
		return
	}
	keyValue := tx.Bucket([]byte(bkPrefix + "_kv"))
	if keyValue == nil {
		return false
	}
	k, _ := keyValue.Cursor().Seek(key)
	return bytes.Equal(k, key)
}

func KSVDelete(tx *bbolt.Tx, bkPrefix string, key []byte) error {
	keyValue := tx.Bucket([]byte(bkPrefix + "_kv"))
	sort0Key := tx.Bucket([]byte(bkPrefix + "_s0k"))
	sort1Key := tx.Bucket([]byte(bkPrefix + "_s1k"))
	keySortSort2 := tx.Bucket([]byte(bkPrefix + "_kss"))
	if keyValue == nil || sort0Key == nil || sort1Key == nil || keySortSort2 == nil {
		return nil
	}

	oldSort := keySortSort2.Get(key)
	if len(oldSort) >= 8 {
		deleteKSS(sort0Key, sort1Key, KeySortValue{
			Key:   key,
			Sort0: types.BytesUint64(oldSort[:8]),
			Sort1: oldSort[8:],
		})
	}
	if err := keySortSort2.Delete(key); err != nil {
		return err
	}

	if deleted, err := keyValue.TestDelete(key); err != nil {
		return err
	} else if deleted {
		keyValue.SetSequence(keyValue.Sequence() - 1)
	}
	return nil
}

func KSVDeleteSort0(tx *bbolt.Tx, bkPrefix string, key []byte) error {
	sort0Key := tx.Bucket([]byte(bkPrefix + "_s0k"))
	keySortSort2 := tx.Bucket([]byte(bkPrefix + "_kss"))
	if sort0Key == nil || keySortSort2 == nil {
		return nil
	}

	oldSort := keySortSort2.Get(key)
	if len(oldSort) >= 8 {
		if deleted, err := sort0Key.TestDelete((KeySortValue{
			Key:   key,
			Sort0: types.BytesUint64(oldSort[:8]),
			Sort1: oldSort[8:],
		}).sort0Key()); err != nil {
			return err
		} else if deleted {
			sort0Key.SetSequence(sort0Key.Sequence() - 1)
		}
	}
	return nil
}

func KSVPaging(tx *bbolt.Tx, bkPrefix string, bySort int, desc bool, page, pageSize int) (res []KeySortValue, total, pages int) {
	if tx == nil {
		Store.View(func(tx *bbolt.Tx) error {
			res, total, pages = KSVPaging(tx, bkPrefix, bySort, desc, page, pageSize)
			return nil
		})
		return
	}
	keyValue := tx.Bucket([]byte(bkPrefix + "_kv"))
	sort0Key := tx.Bucket([]byte(bkPrefix + "_s0k"))
	sort1Key := tx.Bucket([]byte(bkPrefix + "_s1k"))

	var c *bbolt.Cursor
	switch bySort {
	case 0:
		if keyValue == nil || sort0Key == nil {
			return
		}
		c = sort0Key.Cursor()
		total = int(sort0Key.Sequence())
	case 1:
		if keyValue == nil || sort1Key == nil {
			return
		}
		c = sort1Key.Cursor()
		total = int(sort1Key.Sequence())
	default:
		if keyValue == nil {
			return
		}
		c = keyValue.Cursor()
		total = int(keyValue.Sequence())
	}

	i := 0
	a, b := c.First()
	if desc {
		a, b = c.Last()
	}

	for start := clock.UnixNano(); len(a) > 0; {
		if i >= (page+1)*pageSize {
			break
		}
		if clock.UnixNano()-start > types.Config.PagingTimeout {
			return nil, -1, -1
		}
		if i/pageSize == page {
			var ksv KeySortValue
			switch bySort {
			case 0:
				ksv.Key = append([]byte{}, a[8:]...)
				ksv.Value = append([]byte{}, keyValue.Get(a[8:])...)
			case 1:
				ln := int(a[len(a)-1])
				k := a[len(a)-1-ln : len(a)-1]
				ksv.Key = append([]byte{}, k...)
				ksv.Value = append([]byte{}, keyValue.Get(k)...)
			default:
				ksv.Key = append([]byte{}, a...)
				ksv.Value = append([]byte{}, b...)
			}
			res = append(res, ksv)
		}
		i++
		if desc {
			a, b = c.Prev()
		} else {
			a, b = c.Next()
		}
	}

	pages = idivceil(total, pageSize)
	return
}

func KSVPagingSort0FindKey(bkPrefix string, key []byte, desc bool, pageSize int) (page int) {
	i := 0
	Store.View(func(tx *bbolt.Tx) error {
		sort0Key := tx.Bucket([]byte(bkPrefix + "_s0k"))
		if sort0Key == nil {
			return nil
		}
		c := sort0Key.Cursor()

		a, _ := c.First()
		if desc {
			a, _ = c.Last()
		}

		for start := clock.UnixNano(); len(a) > 0; {
			if clock.UnixNano()-start > types.Config.PagingTimeout {
				i = 0
				return nil
			}

			if sort1 := a[8:]; desc {
				if bytes.Compare(sort1, key) <= 0 {
					break
				}
			} else {
				if bytes.Compare(sort1, key) >= 0 {
					break
				}
			}
			i++
			if desc {
				a, _ = c.Prev()
			} else {
				a, _ = c.Next()
			}
		}
		return nil
	})
	return i/pageSize + 1
}

func KSVFirstKeyOfSort1(tx *bbolt.Tx, bkPrefix string, sort1 []byte) (key []byte, found bool) {
	sort1Key := tx.Bucket([]byte(bkPrefix + "_s1k"))
	if sort1Key == nil {
		return
	}
	c := sort1Key.Cursor()
	a, _ := c.Seek(sort1)
	if len(a) > 0 {
		ln := int(a[len(a)-1])
		if bytes.Equal(sort1, a[:len(a)-1-ln-1]) {
			key = append([]byte{}, a[len(a)-1-ln:len(a)-1]...)
			found = true
		}
	}
	return
}

func KSVCount(tx *bbolt.Tx, bkPrefix string) (sz int) {
	if tx == nil {
		Store.DB.View(func(tx *bbolt.Tx) error {
			sz = KSVCount(tx, bkPrefix)
			return nil
		})
		return
	}
	keyValue := tx.Bucket([]byte(bkPrefix + "_kv"))
	if keyValue == nil {
		return 0
	}
	return int(keyValue.Sequence())
}
