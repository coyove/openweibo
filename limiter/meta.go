package limiter

import (
	"net"
	"sync"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/tidwall/gjson"
	"golang.org/x/time/rate"
)

var cdMap sync.Map

type limiter struct {
	*rate.Limiter
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
		if rl.Allow() {
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
		sec = 30
	}

	burst := r.Config().Get("ip_cooldown_burst").Int()
	if burst <= 0 {
		burst = 10
	}

	rl := &limiter{
		Limiter: rate.NewLimiter(rate.Limit(1.0/float64(sec)), int(burst)),
		freeAt:  clock.Unix() + sec,
	}

	cdMap.Store(ipStr, rl)
	time.AfterFunc(time.Second*time.Duration(sec), func() {
		cdMap.Delete(ipStr)
	})
}
