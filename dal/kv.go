package dal

import (
	"github.com/coyove/iis/types"
	"go.etcd.io/bbolt"
)

func KVIncr(tx *bbolt.Tx, key string, value int64) error {
	if tx == nil {
		return Store.Update(func(tx *bbolt.Tx) error { return KVIncr(tx, key, value) })
	}

	kv, err := tx.CreateBucketIfNotExists([]byte("kv"))
	if err != nil {
		return err
	}

	kb := []byte(key)
	c := int64(types.BytesUint64(kv.Get(kb)))
	c += value
	return kv.Put(kb, types.Uint64Bytes(uint64(c)))
}

func KVGet(tx *bbolt.Tx, keys []string) (values [][]byte, err error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if tx == nil {
		err = Store.View(func(tx *bbolt.Tx) error {
			values, err = KVGet(tx, keys)
			return err
		})
		return
	}

	values = make([][]byte, len(keys))
	kv := tx.Bucket([]byte("kv"))
	if kv == nil {
		return values, nil
	}
	for i := range keys {
		values[i] = kv.Get([]byte(keys[i]))
	}
	return values, nil
}

func KVGetInt64(tx *bbolt.Tx, key string) (value int64, err error) {
	v, err := KVGet(tx, []string{key})
	if err != nil {
		return 0, err
	}
	return int64(types.BytesUint64(v[0])), nil
}

func KVGetInt64s(tx *bbolt.Tx, keys []string) (values []int64, err error) {
	v, err := KVGet(tx, keys)
	if err != nil {
		return nil, err
	}
	values = make([]int64, len(v))
	for i, v := range v {
		values[i] = int64(types.BytesUint64(v))
	}
	return values, nil
}
