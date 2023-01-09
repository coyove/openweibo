package dal

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/types"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	//sync "github.com/sasha-s/go-deadlock"
)

var (
	db        *dynamodb.DynamoDB
	tableFTS  = "fts"
	TagsStore struct {
		*bbolt.DB
		*bitmap.Manager
		locks [256]sync.Mutex
	}
)

func InitDB() {
	ddb := types.Config.DynamoDB
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(ddb.Region),
		Credentials: credentials.NewStaticCredentials(ddb.AccessKey, ddb.SecretKey, ""),
		HTTPClient: &http.Client{
			Timeout: time.Second,
			Transport: &http.Transport{
				MaxConnsPerHost: 200,
			},
		},
	})
	if err != nil {
		logrus.Fatal("init DB: ", err)
	}

	db = dynamodb.New(sess)
	info, err := db.DescribeEndpoints(&dynamodb.DescribeEndpointsInput{})
	if err != nil {
		logrus.Fatal("init DB, describe: ", err)
	}
	for _, ep := range info.Endpoints {
		logrus.Info("dynamodb endpoint: ", strings.Replace(ep.String(), "\n", " ", -1))
	}

	TagsStore.Manager, err = bitmap.NewManager("bitmap_cache/tags", 1024000, 1*1024*1024*1024)
	if err != nil {
		logrus.Fatal("init bitmap manager: ", err)
	}

	TagsStore.DB, err = bbolt.Open("bitmap_cache/tags.db", 0777, &bbolt.Options{FreelistType: bbolt.FreelistMapType})
	if err != nil {
		logrus.Fatal("init tags db: ", err)
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

func KSVFromTag(tag *types.Tag) KeySortValue {
	k := bitmap.Uint64Key(tag.Id)
	return KeySortValue{
		Key:   k[:],
		Sort0: uint64(tag.UpdateUnix),
		Sort1: []byte(tag.Name),
		Value: tag.MarshalBinary(),
	}
}

func KSVUpsert(tx *bbolt.Tx, bkPrefix string, ksv KeySortValue) error {
	if len(ksv.Key) == 0 {
		return fmt.Errorf("null key not allowed")
	}
	if len(ksv.Key) >= 256 {
		return fmt.Errorf("key exceeds 255 bytes")
	}

	keyValue, err := tx.CreateBucketIfNotExists([]byte(bkPrefix))
	if err != nil {
		return err
	}
	sortKey, err := tx.CreateBucketIfNotExists([]byte(bkPrefix + "_s0k"))
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
			old := KeySortValue{
				Key:   ksv.Key,
				Sort0: types.BytesUint64(oldSort[:8]),
				Sort1: oldSort[8:],
			}
			if err := sortKey.Delete(old.sort0Key()); err != nil {
				return err
			}
			if err := sort1Key.Delete(old.sort1Key()); err != nil {
				return err
			}
		}
		if err := sortKey.Put(ksv.sort0Key(), nil); err != nil {
			return err
		}
		if err := sort1Key.Put(ksv.sort1Key(), nil); err != nil {
			return err
		}
		if err := keySortSort2.Put(ksv.Key, append(types.Uint64Bytes(ksv.Sort0), ksv.Sort1...)); err != nil {
			return err
		}
	} else {
		keyValue.FillPercent = 0.9
	}
	return keyValue.Put(ksv.Key, ksv.Value)
}

func KSVDelete(tx *bbolt.Tx, bkPrefix string, key []byte) error {
	keyValue := tx.Bucket([]byte(bkPrefix))
	sortKey := tx.Bucket([]byte(bkPrefix + "_s0k"))
	sort1Key := tx.Bucket([]byte(bkPrefix + "_s1k"))
	keySortSort2 := tx.Bucket([]byte(bkPrefix + "_kss"))
	if keyValue == nil || sortKey == nil || sort1Key == nil || keySortSort2 == nil {
		return nil
	}

	oldSort := keySortSort2.Get(key)
	if len(oldSort) >= 8 {
		old := KeySortValue{
			Key:   key,
			Sort0: types.BytesUint64(oldSort[:8]),
			Sort1: oldSort[8:],
		}
		if err := sortKey.Delete(old.sort0Key()); err != nil {
			return err
		}
		if err := sort1Key.Delete(old.sort1Key()); err != nil {
			return err
		}
	}
	if err := keySortSort2.Delete(key); err != nil {
		return err
	}
	if err := keyValue.Delete(key); err != nil {
		return err
	}
	return nil
}

func KSVPaging(tx *bbolt.Tx, bkPrefix string, bySort int, desc bool, page, pageSize int) (res []KeySortValue, total, pages int) {
	if tx == nil {
		TagsStore.View(func(tx *bbolt.Tx) error {
			res, total, pages = KSVPaging(tx, bkPrefix, bySort, desc, page, pageSize)
			return nil
		})
		return
	}
	keyValue := tx.Bucket([]byte(bkPrefix))
	sort0Key := tx.Bucket([]byte(bkPrefix + "_s0k"))
	sort1Key := tx.Bucket([]byte(bkPrefix + "_s1k"))

	var c *bbolt.Cursor
	switch bySort {
	case 0:
		if keyValue == nil || sort0Key == nil {
			return
		}
		c = sort0Key.Cursor()
	case 1:
		if keyValue == nil || sort1Key == nil {
			return
		}
		c = sort1Key.Cursor()
	default:
		if keyValue == nil {
			return
		}
		c = keyValue.Cursor()
	}

	i := 0
	a, b := c.First()
	if desc {
		a, b = c.Last()
	}

	for len(a) > 0 {
		if i >= (page+1)*pageSize {
			break
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

	total = keyValue.Stats().KeyN
	pages = total / pageSize
	if pages*pageSize != total {
		pages++
	}
	return
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

func LockKey(key interface{}) {
	k := fmt.Sprint(key)
	TagsStore.locks[types.StrHash(k)%uint64(len(TagsStore.locks))].Lock()
}

func UnlockKey(key interface{}) {
	k := fmt.Sprint(key)
	TagsStore.locks[types.StrHash(k)%uint64(len(TagsStore.locks))].Unlock()
}
