package dal

import (
	"net"

	"github.com/tidwall/gjson"
)

func CheckIP(ip net.IP) (ok bool) {
	ok = true
	blacklist, _ := GetJsonizedNote("ns:blacklist")
	blacklist.ForEach(func(k, v gjson.Result) bool {
		_, ra, _ := net.ParseCIDR(k.Str)
		if ra != nil && ra.Contains(ip) {
			ok = false
			return false
		}
		return true
	})
	return
}
