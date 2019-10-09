package action

import (
	"fmt"
	"log"
	"net"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/coyove/iis/cmd/ch/token"
	"github.com/gin-gonic/gin"
)

func getAuthor(g *gin.Context) string {
	a := mv.SoftTrunc(g.PostForm("author"), 32)
	if a == "" {
		a = "user/" + g.MustGet("ip").(net.IP).String()
	}
	return a
}

func hashIP(g *gin.Context) string {
	ip := append(net.IP{}, g.MustGet("ip").(net.IP)...)
	if len(ip) == net.IPv4len {
		ip[3] = 0
	} else if len(ip) == net.IPv6len {
		copy(ip[8:], "\x00\x00\x00\x00\x00\x00\x00\x00")
	}
	return ip.String()
}

func checkTokenAndCaptcha(g *gin.Context, author string) string {
	var (
		answer            = mv.SoftTrunc(g.PostForm("answer"), 6)
		uuid              = mv.SoftTrunc(g.PostForm("uuid"), 32)
		tokenbuf, tokenok = token.Parse(g, uuid)
		challengePassed   bool
	)
	if !g.GetBool("ip-ok") {
		return fmt.Sprintf("guard/cooling-down/%.1fs", float64(config.Cfg.Cooldown)-g.GetFloat64("ip-ok-remain"))
	}
	if !token.IsAdmin(author) {
		if len(answer) == 6 {
			challengePassed = true
			for i := range answer {
				if answer[i]-'0' != tokenbuf[i]%10 {
					challengePassed = false
					break
				}
			}
		}
		if !challengePassed {
			log.Println(g.MustGet("ip"), "challenge failed")
			return "guard/failed-captcha"
		}
	}
	if !tokenok {
		return "guard/token-expired"
	}
	return ""
}

func New(g *gin.Context) {
	var (
		ip       = hashIP(g)
		content  = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		title    = mv.SoftTrunc(g.PostForm("title"), 100)
		author   = getAuthor(g)
		cat      = checkCategory(mv.SoftTrunc(g.PostForm("cat"), 20))
		announce = g.PostForm("announce") != ""
		redir    = func(a, b string) {
			q := encodeQuery(a, b, "author", author, "content", content, "title", title, "cat", cat)
			g.Redirect(302, "/new?"+q)
		}
	)

	if g.PostForm("refresh") != "" {
		redir("", "")
		return
	}

	if ret := checkTokenAndCaptcha(g, author); ret != "" {
		redir("error", ret)
		return
	}

	if len(content) < int(config.Cfg.MinContent) {
		redir("error", "content/too-short")
		return
	}

	if len(title) < int(config.Cfg.MinContent) {
		redir("error", "title/too-short")
		return
	}

	a := m.NewPost(title, content, config.HashName(author), ip, cat)
	if token.IsAdmin(author) && announce {
		a.Announce = true
	}

	if _, err := m.PostPost(a); err != nil {
		log.Println(err)
		redir("error", "internal/error")
		return
	}

	g.Redirect(302, "/p/"+a.DisplayID())
}

func Reply(g *gin.Context) {
	var (
		reply   = ident.StringBytes(g.PostForm("reply"))
		ip      = hashIP(g)
		content = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		author  = getAuthor(g)
		redir   = func(a, b string) {
			g.Redirect(302, "/p/"+ident.BytesString(reply)+"?p=-1&"+encodeQuery(a, b, "author", author, "content", content)+"#paging")
		}
	)

	if g.PostForm("refresh") != "" {
		redir("refresh", "1")
		return
	}

	if ret := checkTokenAndCaptcha(g, author); ret != "" {
		redir("error", ret)
		return
	}

	if len(content) < int(config.Cfg.MinContent) {
		redir("error", "content/too-short")
		return
	}

	if _, err := m.PostReply(reply, m.NewReply(content, config.HashName(author), ip)); err != nil {
		log.Println(err)
		redir("error", "error/can-not-reply")
		return
	}

	author, content = "", ""
	redir("", "")
}
