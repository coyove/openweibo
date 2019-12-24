package action

import (
	"fmt"
	"log"
	"net"

	"github.com/coyove/iis/cmd/ch/config"
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

func APINew(g *gin.Context) {
	if g.PostForm("parent") != "" {
		doReply(g)
		return
	}

	var (
		ip      = hashIP(g)
		content = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		image   = g.PostForm("image64")
		nsfw    = g.PostForm("nsfw") != ""
		err     error
	)

	u := m.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/not-logged-in")
		return
	}

	if len(content) < 3 && image == "" {
		g.String(200, "content/too-short")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	if image != "" {
		if image, err = writeImage(image); err != nil {
			g.String(200, err.Error())
			return
		}
		image = "IMG:" + image
	}

	a := &mv.Article{
		Content: content,
		Media:   image,
		IP:      ip,
		NSFW:    nsfw,
	}

	aid, err := m.Post(a, u)
	if err != nil {
		log.Println(aid, err)
		g.String(200, "internal/error")
		return
	}

	content = ""
	g.String(200, "ok")
}

func doReply(g *gin.Context) {
	var (
		reply   = g.PostForm("parent")
		ip      = hashIP(g)
		content = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		image   = g.PostForm("image64")
		nsfw    = g.PostForm("nsfw") != ""
		err     error
	)

	u := m.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/not-logged-in")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	if g.PostForm("delete") != "" {
		if g.PostForm("delete-confirm") != "" {
			err := m.UpdateArticle(reply, func(a *mv.Article) error {
				if u.ID != a.Author && !u.IsMod() {
					return fmt.Errorf("user/can-not-delete")
				}
				a.Content = mv.DeletionMarker
				a.Media = ""
				return nil
			})
			if err != nil {
				g.String(200, err.Error())
			} else {
				g.String(200, "ok")
			}
		} else {
			g.String(200, "ok")
		}
		return
	}

	if g.PostForm("make-nsfw") != "" {
		err := m.UpdateArticle(reply, func(a *mv.Article) error {
			if u.ID != a.Author && !u.IsMod() {
				return fmt.Errorf("user/can-not-delete")
			}
			a.NSFW = g.PostForm("nsfw-confirm") != ""
			return nil
		})
		if err != nil {
			g.String(200, err.Error())
		} else {
			g.String(200, "ok")
		}
		return
	}

	if image != "" {
		image, err = writeImage(image)
		if err != nil {
			g.String(200, err.Error())
			return
		}
		image = "IMG:" + image
	}

	if _, err := m.PostReply(reply, content, image, u, ip, nsfw); err != nil {
		log.Println(err)
		g.String(200, "error/can-not-reply")
		return
	}

	g.String(200, "ok")
}
