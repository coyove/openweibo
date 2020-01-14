package kv

var randomError = 0
var CacheSize int64 = 10000

func hashString(s string) (h uint16) {
	for _, r := range s {
		h = 31*h + uint16(r)
	}
	return
}
