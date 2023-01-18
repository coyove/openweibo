package dal

import (
	"fmt"

	"github.com/coyove/sdss/contrib/ngram"
)

func LockKey(key interface{}) {
	k := fmt.Sprint(key)
	Store.locks[ngram.StrHash(k)%uint64(len(Store.locks))].Lock()
}

func UnlockKey(key interface{}) {
	k := fmt.Sprint(key)
	Store.locks[ngram.StrHash(k)%uint64(len(Store.locks))].Unlock()
}

func idivceil(a, b int) int {
	c := a / b
	if c*b != a {
		c++
	}
	return c
}
