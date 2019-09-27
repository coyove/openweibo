package main

import (
	"bytes"

	"github.com/etcd-io/bbolt"
)

var (
	db         *bbolt.DB
	bkPost     = []byte("post")
	bkTimeline = []byte("timeline")
	bkTag      = []byte("tag")
	bkReply    = []byte("reply")
	bkAuthor   = []byte("author")
	bkNames    = [][]byte{
		bkPost,
		bkTimeline,
		bkTag,
		bkReply,
		bkAuthor,
	}
)

func ScanBucketAsc(bk *bbolt.Bucket, cursor []byte, n int, reverse bool) (keyvalues [][2][]byte, next []byte) {
	var (
		c    = bk.Cursor()
		k, v []byte
	)

	if cursor == nil || len(bytes.TrimRight(cursor, "\x00")) == 0 {
		k, v = c.First()
	} else {
		k, v = c.Seek(cursor)
	}

	for ; k != nil; k, v = c.Next() {
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

func ScanBucketDesc(bk *bbolt.Bucket, cursor []byte, n int, reverse bool) (keyvalues [][2][]byte, next []byte) {
	var (
		c    = bk.Cursor()
		k, v []byte
	)

	if cursor == nil || len(bytes.TrimRight(cursor, "\x00")) == 0 {
		k, v = c.Last()
	} else {
		k, v = c.Seek(cursor)
		k, v = c.Prev()
		if bytes.Equal(cursor, k) {
			k, v = c.Prev()
		}
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
