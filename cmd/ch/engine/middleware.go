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
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/manager/logs"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
)

var m *manager.Manager

func SetManager(mgr *manager.Manager) {
	m = mgr
}

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

	userid := `<a href='/user'><svg class="vcenter" height="24" version="1.1" width="24"><g transform="translate(0 -1028.4)"><path d="m4 1034.4c-1.1046 0-2 0.9-2 2v10c0 1.1 0.8954 2 2 2h16c1.105 0 2-0.9 2-2v-10c0-1.1-0.895-2-2-2h-16z" fill="#95a5a6"/><path d="m4 5c-1.1046 0-2 0.8954-2 2v10c0 1.105 0.8954 2 2 2h16c1.105 0 2-0.895 2-2v-10c0-1.1046-0.895-2-2-2h-16z" fill="#bdc3c7" transform="translate(0 1028.4)"/><rect fill="#ecf0f1" height="10" width="7" x="4" y="1035.4"/><path d="m7.3125 8a2.5 2.5 0 0 0 -2.3125 2.5 2.5 2.5 0 0 0 0.8125 1.844c-0.7184 0.297-1.3395 0.769-1.8125 1.375v2.281 1h7v-1-2.312c-0.474-0.592-1.1053-1.052-1.8125-1.344a2.5 2.5 0 0 0 0.8125 -1.844 2.5 2.5 0 0 0 -2.6875 -2.5z" fill="#2c3e50" transform="translate(0 1028.4)"/><path d="m13 8v1h4v-1h-4zm0 2v1h7v-1h-7zm0 2v1h7v-1h-7z" fill="#7f8c8d" transform="translate(0 1028.4)"/></g></svg></a>`
	tok, _ := g.Cookie("id")
	if u, _ := m.GetUserByToken(tok); u != nil {
		g.Set("user", u)
		userid += "&nbsp;<a class='vcenter' href='/user'>" + u.ID + "</a>"
	}

	g.Set("ip", ip)
	g.Set("req-start", start)
	g.Next()
	msec := time.Since(start).Nanoseconds() / 1e6

	if msec > Survey.Max {
		Survey.Max = msec
	}
	atomic.AddInt64(&Survey.Written, int64(g.Writer.Size()))

	x := g.Writer.Header().Get("Content-Type")
	if strings.HasPrefix(x, "text/html") {
		g.Writer.Write([]byte(userid))

		tag := g.Param("tag")
		for _, t := range config.Cfg.Tags {
			if tag == t {
				g.Writer.Write([]byte(fmt.Sprintf(" / <b class='vcenter'>%v</b>", t)))
			} else {
				g.Writer.Write([]byte(fmt.Sprintf(" / <a class='vcenter' href='/cat/%v'>%v</a>", t, t)))
			}
		}
		g.Writer.Write([]byte(fmt.Sprintf("<span class='vcenter' style='float:right'>%dms</span>", msec)))
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

func New(prod bool) *gin.Engine {
	os.MkdirAll("tmp/logs", 0700)
	logf, err := rotatelogs.New("tmp/logs/access_log.%Y%m%d%H%M", rotatelogs.WithLinkName("tmp/logs/access_log"), rotatelogs.WithMaxAge(7*24*time.Hour))
	//logerrf, err := rotatelogs.New("tmp/logs/error_log.%Y%m%d%H%M", rotatelogs.WithLinkName("error_log"), rotatelogs.WithMaxAge(7*24*time.Hour))
	if err != nil {
		panic(err)
	}

	mwLoggerConfig := gin.LoggerConfig{
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
	}

	if prod {
		gin.SetMode(gin.ReleaseMode)

		mwLoggerConfig.Output = logf
		gin.DefaultErrorWriter = logs.New(config.Cfg.CwRegion, config.Cfg.DyAccessKey, config.Cfg.DySecretKey, "iis", "gin")
	} else {
		mwLoggerConfig.Output = os.Stdout
		gin.DefaultErrorWriter = io.MultiWriter(logs.New(config.Cfg.CwRegion, config.Cfg.DyAccessKey, config.Cfg.DySecretKey, "iis", "gin"), os.Stdout)
	}

	log.SetOutput(gin.DefaultErrorWriter)
	log.SetFlags(log.Lshortfile | log.Ltime | log.Ldate)

	r := gin.New()
	r.Use(gin.Recovery(), gzip.Gzip(gzip.BestSpeed), gin.LoggerWithConfig(mwLoggerConfig), mwRenderPerf, mwIPThrot)

	loadTrafficCounter()
	return r
}
