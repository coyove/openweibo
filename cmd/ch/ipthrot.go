package main

import (
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

func mwRenderPerf(g *gin.Context) {
	start := time.Now()
	g.Next()
	msec := time.Since(start).Nanoseconds() / 1e6

	for {
		x := atomic.LoadInt64(&survey.render.avg)
		x2 := atomic.LoadInt64(&survey.render.max)
		y := (x + msec) / 2
		y2 := x2
		if msec > y2 {
			y2 = msec
		}

		if atomic.CompareAndSwapInt64(&survey.render.avg, x, y) {
			survey.render.max = y2
			break
		}
	}

}

func mwIPThrot(g *gin.Context) {
	if g.Request.Method != "POST" {
		g.Next()
		return
	}

	if g.PostForm("author") == config.AdminName {
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	ip := g.ClientIP()
	lastaccess, ok := dedup.Get(ip)
	if !ok {
		dedup.Add(ip, time.Now())
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	t, _ := lastaccess.(time.Time)
	if time.Since(t).Seconds() > 5 {
		dedup.Add(ip, time.Now())
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	g.Set("ip-ok", false)
	g.Next()
}
