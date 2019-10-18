package engine

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager/logs"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func mwRenderPerf(g *gin.Context) {
	ip := net.ParseIP(g.ClientIP())
	if ip == nil {
		g.String(403, "Invalid IP: ["+g.ClientIP()+"]")
		return
	}

	if ip.To4() != nil {
		ip = ip.To4()
	}

	for _, subnet := range config.Cfg.IPBlacklistParsed {
		if subnet.Contains(ip) {
			g.AbortWithStatus(403)
			return
		}
	}

	start := time.Now()
	g.Set("ip", ip)
	g.Set("req-start", start)
	g.Next()
	msec := time.Since(start).Nanoseconds() / 1e6

	if msec > survey.max {
		survey.max = msec
	}
	atomic.AddInt64(&survey.written, int64(g.Writer.Size()))

	x := g.Writer.Header().Get("Content-Type")
	if strings.HasPrefix(x, "text/html") {
		g.Writer.Write([]byte(fmt.Sprintf(
			"Render %dms | Max %dms | Out %.2fG | <a href='https://github.com/coyove/iis'>IIS</a>",
			msec, survey.max, float64(survey.written)/1024/1024/1024)))
	}
}

func mwIPThrot(g *gin.Context) {
	if ident.IsAdmin(g) {
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	ip := g.MustGet("ip").(net.IP).String()
	lastaccess, ok := ident.Dedup.Get(ip)

	if !ok {
		ident.Dedup.Add(ip, time.Now())
		g.Set("ip-ok", true)
		g.Next()
		return
	}

	t, _ := lastaccess.(time.Time)
	diff := time.Since(t).Seconds()

	if diff > float64(config.Cfg.Cooldown) {
		ident.Dedup.Add(ip, time.Now())
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
			buf.WriteByte('[')
			buf.WriteString(strconv.FormatFloat(float64(params.BodySize)/1024, 'f', 3, 64))
			buf.WriteByte(']')
			buf.WriteByte(' ')
			buf.WriteString(params.ErrorMessage)
			buf.WriteByte('\n')
			return buf.String()
		},
	})
}

func New(prod bool) *gin.Engine {
	//os.MkdirAll("tmp/logs", 0700)
	//logf, err := rotatelogs.New("tmp/logs/access_log.%Y%m%d%H%M", rotatelogs.WithLinkName("access_log"), rotatelogs.WithMaxAge(7*24*time.Hour))
	//logerrf, err := rotatelogs.New("tmp/logs/error_log.%Y%m%d%H%M", rotatelogs.WithLinkName("error_log"), rotatelogs.WithMaxAge(7*24*time.Hour))
	//if err != nil {
	//	panic(err)
	//}

	if prod {
		gin.SetMode(gin.ReleaseMode)
		mwLoggerOutput = logs.New(config.Cfg.CwRegion, config.Cfg.DyAccessKey, config.Cfg.DySecretKey, "iis", "business")
		gin.DefaultErrorWriter = logs.New(config.Cfg.CwRegion, config.Cfg.DyAccessKey, config.Cfg.DySecretKey, "iis", "gin")
	} else {
		mwLoggerOutput, gin.DefaultErrorWriter = os.Stdout, os.Stdout // io.MultiWriter(logf, os.Stdout, cw), io.MultiWriter(logerrf, os.Stdout, cw)
	}

	log.SetOutput(mwLoggerOutput)
	log.SetFlags(log.Lshortfile | log.Ltime | log.Ldate)

	r := gin.New()
	r.Use(gin.Recovery(), gzip.Gzip(gzip.BestSpeed), mwLogger(), mwRenderPerf, mwIPThrot)

	loadTrafficCounter()
	return r
}
