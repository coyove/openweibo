package main

import (
	"time"

	"github.com/gin-gonic/gin"
)

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
