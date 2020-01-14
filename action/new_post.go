package action

import (
	"fmt"
	"log"
	"net"
	"net/url"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/engine"
	"github.com/coyove/iis/model"
	"github.com/coyove/iis/view"
	"github.com/gin-gonic/gin"
)

func getAuthor(g *gin.Context) string {
	a := common.SoftTrunc(g.PostForm("author"), 32)
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
		content = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
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
		if image, err = writeImage(u, g.PostForm("image_name"), image); err != nil {
			g.String(200, err.Error())
			return
		}
		image = "IMG:" + image
	}

	a := &model.Article{
		Content: content,
		Media:   image,
		IP:      ip,
		NSFW:    nsfw,
	}

	a2, err := m.Post(a, u, g.PostForm("no_master") == "1")
	if err != nil {
		log.Println(a2, err)
		g.String(200, "internal/error")
		return
	}

	g.Writer.Header().Add("X-Result", url.PathEscape(engine.RenderTemplateString("row_content.html",
		view.NewTopArticleView(a2, u))))
	g.String(200, "ok")
}

func doReply(g *gin.Context) {
	var (
		reply   = g.PostForm("parent")
		ip      = hashIP(g)
		content = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
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
		err := m.UpdateArticle(reply, func(a *model.Article) error {
			if u.ID != a.Author && !u.IsMod() {
				return fmt.Errorf("user/can-not-delete")
			}
			a.Content = model.DeletionMarker
			a.Media = ""
			return nil
		})
		if err != nil {
			g.String(200, err.Error())
		} else {
			g.String(200, "ok")
		}
		return
	}

	if g.PostForm("makensfw") != "" {
		err := m.UpdateArticle(reply, func(a *model.Article) error {
			if u.ID != a.Author && !u.IsMod() {
				return fmt.Errorf("user/can-not-delete")
			}
			a.NSFW = !a.NSFW
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
		image, err = writeImage(u, g.PostForm("image_name"), image)
		if err != nil {
			g.String(200, err.Error())
			return
		}
		image = "IMG:" + image
	}

	a2, err := m.PostReply(reply, content, image, u, ip, nsfw, g.PostForm("no_timeline") == "1")
	if err != nil {
		log.Println(a2, err)
		g.String(200, "error/can-not-reply")
		return
	}

	g.Writer.Header().Add("X-Result", url.PathEscape(engine.RenderTemplateString("row_content.html",
		view.NewReplyArticleView(a2, u))))
	g.String(200, "ok")
}
