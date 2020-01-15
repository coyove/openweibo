package common

import "sync"

var _keylocks [65536]sync.Mutex

func LockKey(key string) {
	_keylocks[Hash16(key)].Lock()
}

func UnlockKey(key string) {
	_keylocks[Hash16(key)].Unlock()
}
