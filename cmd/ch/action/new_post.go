package action

import (
	"log"
	"net"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/mv"
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
		copy(ip[10:], "\x00\x00\x00\x00\x00\x00") // \80
	}
	return ip.String()
}

func New(g *gin.Context) {
	var (
		ip      = hashIP(g)
		content = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		title   = mv.SoftTrunc(g.PostForm("title"), 100)
		cat     = checkCategory(mv.SoftTrunc(g.PostForm("cat"), 20))
		redir   = func(a, b string) {
			q := EncodeQuery(a, b, "content", content, "title", title, "cat", cat)
			g.Redirect(302, "/new"+q)
		}
	)

	if emoji := g.PostForm("emoji"); emoji != "" {
		content += emoji
		redir("", "")
		return
	}

	u, ok := g.Get("user")
	if !ok {
		redir("login-error", "user/not-logged-in")
		return
	}

	if ret := checkToken(g); ret != "" {
		redir("error", ret)
		return
	}

	if len(title) < int(config.Cfg.MinContent) {
		redir("error", "title/too-short")
		return
	}

	aid, err := m.Post(content, u.(*mv.User).ID, ip)
	if err != nil {
		log.Println(aid, err)
		redir("error", "internal/error")
		return
	}

	g.Redirect(302, "/p/"+aid)
}

func Reply(g *gin.Context) {
	var (
		reply   = ident.ParseID(g.PostForm("reply"))
		ip      = hashIP(g)
		content = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		redir   = func(a, b string) {
			g.Redirect(302, "/p/"+reply.String()+EncodeQuery(a, b, "content", content)+"&p=-1#paging")
		}
	)

	if emoji := g.PostForm("emoji"); emoji != "" {
		content += emoji
		redir("refresh", "1")
		return
	}

	if reply.Reply() > 0 {
		redir("", "")
		return
	}

	u, ok := g.Get("user")
	if !ok {
		redir("error", "user/not-logged-in")
		return
	}

	if ret := checkToken(g); ret != "" {
		redir("error", ret)
		return
	}

	if _, err := m.PostReply(reply.String(), content, u.(*mv.User).ID, ip); err != nil {
		log.Println(err)
		redir("error", "error/can-not-reply")
		return
	}

	content = ""
	redir("", "")
}
