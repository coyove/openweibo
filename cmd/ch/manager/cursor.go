package manager

import (
	"bytes"
	"log"

	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/etcd-io/bbolt"
)

func cursorMoveToLast(c *bbolt.Cursor, tag string) (k, v []byte) {
	if tag == "" {
		k, v = c.Last()
	} else {
		kid := ident.NewID(ident.HeaderAuthorTag, tag)
		kid.Maximize()
		c.Seek(kid.Marshal())
		k, v = c.Prev()
		if ident.ParseID(k).Tag() == tag {
			return
		}
		return nil, nil
	}
	return
}

func substractCursorN(c *bbolt.Cursor, tag string, cursor []byte, n int) (newCursor []byte) {

	if cursor == nil || len(bytes.Trim(cursor, "\x00")) == 0 {
		cursor, _ = cursorMoveToLast(c, tag)
	}

	validK := func(k []byte) bool {
		if k == nil {
			return false
		}
		if tag != "" {
			kid := ident.ParseID(k)
			return kid.Header() == ident.HeaderAuthorTag && kid.Tag() == tag
		}
		return true
	}

	k, _ := c.Seek(cursor)
	if !validK(k) {
		return nil
	}

	i := 0
	for {
		oldk := k
		k, _ = c.Next()
		if !validK(k) {
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

func scanBucketDesc(bk *bbolt.Bucket, tag string, cursor []byte, n int) (keyvalues [][2][]byte, prev, next []byte) {
	var (
		c    = bk.Cursor()
		k, v []byte
	)

	if cursor == nil || len(bytes.TrimRight(cursor, "\x00")) == 0 {
		k, v = cursorMoveToLast(c, tag)
	} else {
		k, v = c.Seek(cursor)
	}

	for ; k != nil; k, v = c.Prev() {
		if tag != "" {
			if kid := ident.ParseID(k); kid.Header() != ident.HeaderAuthorTag || kid.Tag() != tag {
				break
			}
		}

		keyvalues = append(keyvalues, [2][]byte{k, v})
		if len(keyvalues) >= n+1 {
			break
		}
	}

	if len(keyvalues) == n+1 {
		next = keyvalues[len(keyvalues)-1][0]
		keyvalues = keyvalues[:len(keyvalues)-1]
	}

	prev = substractCursorN(c, tag, cursor, n)
	return
}

func (m *Manager) UserExisted(id string) (ok bool) {
	m.db.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket(bkPost).Cursor()
		k, _ := cursorMoveToLast(c, id)

		if k == nil {
			return nil
		}

		kid := ident.ParseID(k)
		log.Println(kid.Tag(), id)

		ok = kid.Header() == ident.HeaderAuthorTag && kid.Tag() == id
		return nil
	})
	return
}
