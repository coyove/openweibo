package limiter

import (
	"net"
	"sync"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/tidwall/gjson"
)

var cdMap sync.Map

func CheckIP(r *types.Request) (ok bool, remains int64) {
	ipm := r.RemoteIPv4Masked()
	dal.LockKey(ipm)
	defer dal.UnlockKey(ipm)

	ip := r.RemoteIPv4
	ok = true
	blacklist, _ := dal.GetJsonizedNote("ns:blacklist")
	blacklist.ForEach(func(k, v gjson.Result) bool {
		_, ra, _ := net.ParseCIDR(k.Str)
		if ra != nil && ra.Contains(ip) {
			ok = false
			return false
		}
		return true
	})

	if !ok {
		return false, -1
	}

	if r.User.IsMod() {
		return true, 0
	}

	if v, ok := cdMap.Load(r.RemoteIPv4Masked().String()); ok {
		return false, v.(int64) - clock.Unix()
	}
	return true, 0
}

func AddIP(r *types.Request) {
	if r.User.IsMod() {
		return
	}
	sec := r.Config().Get("ip_cooldown_sec").Int()
	if sec <= 0 {
		sec = 30
	}
	ipStr := r.RemoteIPv4Masked().String()
	cdMap.Store(ipStr, clock.Unix()+sec)
	time.AfterFunc(time.Second*time.Duration(sec), func() {
		cdMap.Delete(ipStr)
	})
}
