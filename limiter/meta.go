package limiter

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/tidwall/gjson"
)

var cdMap sync.Map

type limiter struct {
	ctr    int64
	freeAt int64
}

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
		rl := v.(*limiter)
		if atomic.AddInt64(&rl.ctr, -1) > 0 {
			return true, 0
		}
		return false, rl.freeAt - clock.Unix()
	}
	return true, 0
}

func AddIP(r *types.Request) {
	if r.User.IsMod() {
		return
	}

	ipStr := r.RemoteIPv4Masked().String()
	if _, ok := cdMap.Load(ipStr); ok {
		return
	}

	sec := r.Config().Get("ip_cooldown_sec").Int()
	if sec <= 0 {
		sec = 60
	}

	ctr := r.Config().Get("ip_cooldown_counter").Int()
	if ctr <= 0 {
		ctr = 10
	}

	cdMap.Store(ipStr, &limiter{
		ctr:    ctr,
		freeAt: clock.Unix() + sec,
	})

	time.AfterFunc(time.Second*time.Duration(sec), func() {
		cdMap.Delete(ipStr)
	})
}
