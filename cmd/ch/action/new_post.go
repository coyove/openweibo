package action

import (
	"fmt"
	"log"
	"net"
	"net/url"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

func getAuthor(g *gin.Context) string {
	a := mv.SoftTrunc(g.PostForm("author"), 32)
	//if a == "" {
	//	a = "user/" + hashIP(g)
	//}
	return a
}

func hashIP(g *gin.Context) string {
	ip := append(net.IP{}, g.MustGet("ip").(net.IP)...)
	if len(ip) == net.IPv4len {
		ip[3] = 0 // \24
	} else if len(ip) == net.IPv6len {
		copy(ip[8:], "\x00\x00\x00\x00\x00\x00\x00\x00") // \64
	}
	return ip.String()
}

func checkTokenAndCaptcha(g *gin.Context, author string) string {
	var (
		answer            = mv.SoftTrunc(g.PostForm("answer"), 6)
		uuid              = mv.SoftTrunc(g.PostForm("uuid"), 32)
		tokenbuf, tokenok = ident.ParseToken(g, uuid)
		challengePassed   bool
	)

	if !g.GetBool("ip-ok") {
		return fmt.Sprintf("guard/cooling-down/%.1fs", float64(config.Cfg.Cooldown)-g.GetFloat64("ip-ok-remain"))
	}

	if m.IsBanned(config.HashName(author)) {
		return "guard/id-not-existed"
	}

	if !ident.IsAdmin(author) {
		if len(answer) == 4 {
			challengePassed = true
			for i := range answer {
				a := answer[i]
				if a >= 'A' && a <= 'Z' {
					a = a - 'A' + 'a'
				}

				if a != "0123456789acefhijklmnpqrtuvwxyz"[tokenbuf[i]%31] &&
					a != "oiz3asg7b9acefhijklmnpqrtuvwxyz"[tokenbuf[i]%31] {
					challengePassed = false
					break
				}
			}
		}
		if !challengePassed {
			log.Println(g.MustGet("ip"), "challenge failed")
			return "guard/failed-captcha"
		}

		if config.Cfg.NeedID && !m.UserExisted(config.HashName(author)) {
			return "guard/id-not-existed"
		}
	}

	// Admin still needs token verification
	if !tokenok {
		return "guard/token-expired"
	}

	return ""
}

func New(g *gin.Context) {
	var (
		ip      = hashIP(g)
		content = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		title   = mv.SoftTrunc(g.PostForm("title"), 100)
		author  = getAuthor(g)
		cat     = checkCategory(mv.SoftTrunc(g.PostForm("cat"), 20))
		redir   = func(a, b string) {
			q := EncodeQuery(a, b, "author", author, "content", content, "title", title, "cat", cat)
			g.Redirect(302, "/new"+q)
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
	a.Saged = g.PostForm("saged") != ""

	if _, err := m.Post(a); err != nil {
		log.Println(a, err)
		redir("error", "internal/error")
		return
	}

	g.Redirect(302, "/p/"+ident.ParseID(a.ID).String())
}

func Reply(g *gin.Context) {
	var (
		reply   = ident.ParseID(g.PostForm("reply"))
		ip      = hashIP(g)
		content = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		author  = getAuthor(g)
		redir   = func(a, b string) {
			g.Redirect(302, "/p/"+reply.String()+EncodeQuery(a, b, "author", author, "content", content)+"&p=-1#paging")
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

	if _, err := m.PostReply(reply.String(), m.NewReply(content, config.HashName(author), ip)); err != nil {
		log.Println(err)
		redir("error", "error/can-not-reply")
		return
	}

	author, content = "", ""
	redir("", "")
}

func EncodeQuery(a ...string) string {
	query := url.Values{}
	for i := 0; i < len(a); i += 2 {
		if a[i] != "" {
			query.Add(a[i], a[i+1])
		}
	}
	return "?" + query.Encode()
}
