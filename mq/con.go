package mq

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/etcd-io/bbolt"
)

func (mq *SimpleMessageQueue) addToConsumed(key []byte) error {
	mq.consumed = append(mq.consumed, key)
	_, err := mq.f.Write(key)
	return err
}

func (mq *SimpleMessageQueue) clearConsumed() error {
	mq.consumed = mq.consumed[:0]
	if _, err := mq.f.Seek(0, 0); err != nil {
		return err
	}
	return mq.f.Truncate(0)
}

func (mq *SimpleMessageQueue) delConsumed(key []byte) error {
	for i, k := range mq.consumed {
		if bytes.Equal(key, k) {
			mq.consumed = append(mq.consumed[:i], mq.consumed[i+1:]...)
			_, err := mq.f.Write(key) // double write key, meaning this key should not be deleted
			return err
		}
	}
	return nil
}

func (mq *SimpleMessageQueue) checkConFile() error {
	buf := make([]byte, 8)
	m := map[uint64]bool{}
	for {
		n, err := io.ReadAtLeast(mq.f, buf, 8)
		if err == io.EOF {
			break
		}
		if err != nil || n != 8 {
			return fmt.Errorf("corrupted con file, read %v bytes, err: %v", n, err)
		}
		x := binary.BigEndian.Uint64(buf)
		m[x] = !m[x]
	}

	err := mq.db.Update(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkName)
		for key, del := range m {
			if del {
				binary.BigEndian.PutUint64(buf, key)
				if err := bk.Delete(buf); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return mq.clearConsumed()
}
