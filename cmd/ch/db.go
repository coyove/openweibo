package main

import (
	"bytes"

	"github.com/etcd-io/bbolt"
)

func substractCursorN(bk *bbolt.Bucket, cursor []byte, n int) (newCursor []byte) {
	c := bk.Cursor()

	if cursor == nil || len(bytes.Trim(cursor, "\x00")) == 0 {
		cursor, _ = c.Last()
	}

	k, _ := c.Seek(cursor)
	if k == nil {
		return nil
	}

	i := 0
	for {
		oldk := k
		k, _ = c.Next()
		if k == nil {
			k = oldk
			break
		}

		if i++; i >= n {
			break
		}
	}

	if i == 0 {
		return nil
	}

	newCursor = k
	return
}

func ScanBucketDesc(bk *bbolt.Bucket, cursor []byte, n int, reverse bool) (keyvalues [][2][]byte, next []byte) {
	var (
		c    = bk.Cursor()
		k, v []byte
	)

	if cursor == nil || len(bytes.TrimRight(cursor, "\x00")) == 0 {
		k, v = c.Last()
	} else {
		k, v = c.Seek(cursor)
	}

	for ; k != nil; k, v = c.Prev() {
		keyvalues = append(keyvalues, [2][]byte{k, v})
		if len(keyvalues) >= n+1 {
			break
		}
	}

	if len(keyvalues) == n+1 {
		next = keyvalues[len(keyvalues)-1][0]
		keyvalues = keyvalues[:len(keyvalues)-1]
	}

	if reverse {
		for left, right := 0, len(keyvalues)-1; left < right; left, right = left+1, right-1 {
			keyvalues[left], keyvalues[right] = keyvalues[right], keyvalues[left]
		}
	}

	return
}
