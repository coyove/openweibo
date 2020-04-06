package action

import (
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/coyove/iis/view"
	"github.com/gin-gonic/gin"
)

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
		rlm, _  = strconv.Atoi(g.PostForm("reply_lock"))
		err     error
	)

	u := dal.GetUserByContext(g)
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
		Content:       content,
		Media:         image,
		IP:            ip,
		NSFW:          g.PostForm("nsfw") != "",
		ReplyLockMode: byte(rlm),
	}
	a.SetStickOnTop(g.PostForm("stick_on_top") != "")

	a2, err := dal.Post(a, u, g.PostForm("no_master") == "1")
	if err != nil {
		if err.Error() == "multiple/stick-on-top" {
			g.String(200, err.Error())
			return
		}
		log.Println(a2, err)
		g.String(200, "internal/error")
		return
	}

	g.String(200, "ok:"+url.PathEscape(middleware.RenderTemplateString("row_content.html",
		view.NewTopArticleView(a2, u))))
}

func doReply(g *gin.Context) {
	var (
		reply   = g.PostForm("parent")
		ip      = hashIP(g)
		content = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
		image   = g.PostForm("image64")
		nsfw    = g.PostForm("nsfw") != ""
		rlm, _  = strconv.Atoi(g.PostForm("reply_lock"))
		err     error
	)

	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/not-logged-in")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
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

	noTimeline := g.PostForm("no_timeline") == "1" || strings.Contains(content, "#ReportThis")

	a := &model.Article{
		Content:       content,
		Media:         image,
		NSFW:          nsfw,
		Author:        u.ID,
		IP:            ip,
		ReplyLockMode: byte(rlm),
	}

	a2, err := dal.PostReply(reply, a, u, noTimeline)
	if err != nil {
		log.Println(a2, err)
		g.String(200, "error/can-not-reply")
		return
	}

	g.String(200, "ok:"+url.PathEscape(middleware.RenderTemplateString("row_content.html",
		view.NewReplyArticleView(a2, u))))
}

func APIDeleteArticle(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/not-logged-in")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	if _, err := dal.DoUpdateArticle(&dal.UpdateArticleRequest{ID: g.PostForm("id"), DeleteBy: u}); err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

func APIToggleNSFWArticle(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/not-logged-in")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	if _, err := dal.DoUpdateArticle(&dal.UpdateArticleRequest{ID: g.PostForm("id"), ToggleNSFWBy: u}); err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

func APIToggleLockArticle(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/not-logged-in")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	v, _ := strconv.Atoi(g.PostForm("mode"))
	if _, err := dal.DoUpdateArticle(&dal.UpdateArticleRequest{
		ID:                g.PostForm("id"),
		UpdateReplyLockBy: u,
		UpdateReplyLock:   aws.Uint8(byte(v)),
	}); err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

func APIDropTop(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/not-logged-in")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	if _, err := dal.DoUpdateArticleExtra(&dal.UpdateArticleExtraRequest{
		ID:            ik.NewID(ik.IDAuthor, u.ID).String(),
		SetExtraKey:   "stick_on_top",
		SetExtraValue: "",
	}); err != nil {
		g.String(200, err.Error())
		return
	}

	g.String(200, "ok")
}
