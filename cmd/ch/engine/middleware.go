package engine

import (
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
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
)

var m *manager.Manager
var engine *gin.Engine

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

	tok, _ := g.Cookie("id")
	u, _ := m.GetUserByToken(tok)
	if u != nil {
		g.Set("user", u)
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
		uuid, _ := ident.MakeToken(g)
		engine.HTMLRender.Instance("footer.html", struct {
			UUID          string
			LoginError    string
			LoginUsername string
			LoginPassword string
			Render        int64
			Tags          []string
			CurTag        string
			User          *mv.User
		}{
			uuid,
			g.Query("login-error"),
			g.Query("username"),
			ident.ParseTempToken(g.Query("password")),
			msec,
			config.Cfg.Tags,
			g.Param("tag"),
			u,
		}).Render(g.Writer)
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

	engine = r
	return r
}
