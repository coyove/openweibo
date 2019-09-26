package main

import (
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

func mwRenderPerf(g *gin.Context) {
	ip := net.ParseIP(g.ClientIP())
	if ip == nil {
		g.String(403, "Invalid IP: "+g.ClientIP())
		return
	}

	if ip.To4() != nil {
		ip = ip.To4()
	}

	for _, subnet := range config.ipblacklist {
		if subnet.Contains(ip) {
			g.AbortWithStatus(403)
			return
		}
	}

	g.Set("ip", ip)

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

	atomic.AddInt64(&survey.written, int64(g.Writer.Size()))
}

func mwIPThrot(g *gin.Context) {
	if isAdmin(g) {
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	ip := g.MustGet("ip").(net.IP).String()
	lastaccess, ok := dedup.Get(ip)

	if !ok {
		dedup.Add(ip, time.Now())
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	t, _ := lastaccess.(time.Time)
	diff := time.Since(t).Seconds()

	if diff > float64(config.Cooldown) {
		dedup.Add(ip, time.Now())
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	g.Set("ip-ok", false)
	g.Set("ip-ok-remain", diff)
	g.Next()
}

var mwLoggerOutput io.Writer

func mwLogger() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		Output: mwLoggerOutput,
		Formatter: func(params gin.LogFormatterParams) string {
			buf := strings.Builder{}
			itoa := func(i int, wid int) {
				var b [20]byte
				bp := len(b) - 1
				for i >= 10 || wid > 1 {
					wid--
					q := i / 10
					b[bp] = byte('0' + i - q*10)
					bp--
					i = q
				}
				// i < 10
				b[bp] = byte('0' + i)
				buf.Write(b[bp:])
			}

			itoa(params.TimeStamp.Year(), 4)
			buf.WriteByte('/')
			itoa(int(params.TimeStamp.Month()), 2)
			buf.WriteByte('/')
			itoa(params.TimeStamp.Day(), 2)
			buf.WriteByte(' ')
			itoa(params.TimeStamp.Hour(), 2)
			buf.WriteByte(':')
			itoa(params.TimeStamp.Minute(), 2)
			buf.WriteByte(':')
			itoa(params.TimeStamp.Second(), 2)
			buf.WriteByte(' ')
			buf.WriteString("gin-stub:1:")
			buf.WriteByte(' ')
			buf.WriteString(params.ClientIP)
			buf.WriteByte(' ')
			buf.WriteString(params.Method)
			buf.WriteByte(' ')
			buf.WriteByte('[')
			if params.StatusCode >= 400 {
				buf.WriteString(strconv.Itoa(params.StatusCode))
			}
			buf.WriteByte(']')
			buf.WriteByte(' ')
			buf.WriteString(params.Path)
			buf.WriteByte(' ')
			buf.WriteString(strconv.FormatFloat(float64(params.BodySize)/1024, 'f', 3, 64))
			buf.WriteByte('K')
			buf.WriteByte(' ')
			buf.WriteString(params.ErrorMessage)
			buf.WriteByte('\n')
			return buf.String()
		},
	})
}
