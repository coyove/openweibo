package bbolt

import "bytes"

func (db *DB) Size() (sz int64) {
	db.View(func(tx *Tx) error {
		sz = tx.Size()
		return nil
	})
	return
}

// KeyN is a shortcut of Bucket.Stats().KeyN, which assumes there is no nested buckets
func (b *Bucket) KeyN() (n int) {
	b.forEachPage(func(p *page, depth int) {
		if (p.flags & leafPageFlag) != 0 {
			n += int(p.count)
		}
	})
	return
}

func (b *Bucket) TestDelete(key []byte) (bool, error) {
	if b.tx.db == nil {
		return false, ErrTxClosed
	} else if !b.Writable() {
		return false, ErrTxNotWritable
	}

	// Move cursor to correct position.
	c := b.Cursor()
	k, _, flags := c.seek(key)

	// Return nil if the key doesn't exist.
	if !bytes.Equal(key, k) {
		return false, nil
	}

	// Return an error if there is already existing bucket value.
	if (flags & bucketLeafFlag) != 0 {
		return false, ErrIncompatibleValue
	}

	// Delete the node if we have a matching key.
	c.node().del(key)

	return true, nil
}

func (b *Bucket) TestPut(key []byte, value []byte) (bool, error) {
	if b.tx.db == nil {
		return false, ErrTxClosed
	} else if !b.Writable() {
		return false, ErrTxNotWritable
	} else if len(key) == 0 {
		return false, ErrKeyRequired
	} else if len(key) > MaxKeySize {
		return false, ErrKeyTooLarge
	} else if int64(len(value)) > MaxValueSize {
		return false, ErrValueTooLarge
	}

	// Move cursor to correct position.
	c := b.Cursor()
	k, _, flags := c.seek(key)

	// Return an error if there is an existing key with a bucket value.
	if bytes.Equal(key, k) && (flags&bucketLeafFlag) != 0 {
		return false, ErrIncompatibleValue
	}

	// Insert into node.
	key = cloneBytes(key)
	c.node().put(key, key, value, 0, 0)

	return !bytes.Equal(key, k), nil
}
