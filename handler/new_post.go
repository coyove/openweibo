package handler

import (
	"bufio"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func hashIP(g *gin.Context) string {
	ip := append(net.IP{}, g.MustGet("ip").(net.IP)...)
	if len(ip) == net.IPv4len {
		ip[3] = 0 // \24
	} else if len(ip) == net.IPv6len {
		ip4 := ip.To4()
		if ip4 != nil {
			ip = ip4
			ip[3] = 0
		} else {
			copy(ip[10:], "\x00\x00\x00\x00\x00\x00") // \80
		}
	}
	return ip.String() + "/" + strconv.FormatInt(time.Now().Unix(), 36)
}

func APINew(g *gin.Context) {
	if g.PostForm("parent") != "" {
		doReply(g)
		return
	}

	var (
		ip      = hashIP(g)
		content = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
		image   = common.SanMedia(g.PostForm("media"))
		rlm, _  = strconv.Atoi(g.PostForm("reply_lock"))
		err     error
	)

	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
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
		image = "IMG:" + image
	}

	a := &model.Article{
		Content:       content,
		Media:         image,
		IP:            ip,
		NSFW:          g.PostForm("nsfw") != "",
		Anonymous:     g.PostForm("anon") != "",
		ReplyLockMode: byte(rlm),
	}
	a.SetStickOnTop(g.PostForm("stick_on_top") != "")

	if g.PostForm("no_master") == "1" {
		a.PostOptions |= model.PostOptionNoMasterTimeline
	}

	if u.Settings().DoFollowerNeedsAcceptance() {
		a.PostOptions |= model.PostOptionNoSearch
	}

	a2, err := dal.Post(a, u)
	if err != nil {
		if err.Error() == "multiple/stick-on-top" {
			g.String(200, err.Error())
			return
		}
		g.String(200, "internal/error")
		return
	}

	g.String(200, "ok:"+url.PathEscape(middleware.RenderTemplateString("row_content.html",
		NewTopArticleView(a2, u))))
}

func doReply(g *gin.Context) {
	var (
		reply   = g.PostForm("parent")
		ip      = hashIP(g)
		content = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
		image   = common.SanMedia(g.PostForm("media"))
		nsfw    = g.PostForm("nsfw") != ""
		rlm, _  = strconv.Atoi(g.PostForm("reply_lock"))
		err     error
	)

	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
		return
	}

	if ret := checkIP(g); ret != "" {
		g.String(200, ret)
		return
	}

	if image != "" {
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

	if noTimeline || u.Settings().DoFollowerNeedsAcceptance() {
		a.PostOptions |= model.PostOptionNoSearch
	}

	a2, err := dal.PostReply(reply, a, u, noTimeline)
	if err != nil {
		log.Println(a2, err)
		g.String(200, "error/can-not-reply")
		return
	}

	g.String(200, "ok:"+url.PathEscape(middleware.RenderTemplateString("row_content.html",
		NewReplyArticleView(a2, u))))
}

func APIDeleteArticle(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil {
		g.String(200, "user/404")
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
		g.String(200, "user/404")
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
		g.String(200, "user/404")
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
		g.String(200, "user/404")
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

func APIUpload(g *gin.Context) {
	u2, _ := g.Get("user")
	u, _ := u2.(*model.User)
	if u == nil {
		g.String(500, "user/404")
		return
	}

	if !ik.BAdd(u.ID) {
		g.String(500, "cooldown")
		return
	}

	d, params, err := mime.ParseMediaType(g.GetHeader("Content-Type"))
	if err != nil || !(d == "multipart/form-data" || d == "multipart/mixed") {
		g.String(500, "error-not-multipart")
		return
	}

	boundary, ok := params["boundary"]
	if !ok {
		g.String(500, "error-no-boundary")
		return
	}

	mprd := multipart.NewReader(g.Request.Body, boundary)
	part, err := mprd.NextPart()
	if err != nil {
		log.Println("Upload:", err)
		g.String(500, "error-no-part")
	}
	defer part.Close()

	cl, _ := strconv.ParseInt(g.GetHeader("Content-Length"), 10, 64)
	large := cl > 1*1024*1024

	rd := bufio.NewReader(part)
	tmp, _ := rd.Peek(1024)
	switch ct := http.DetectContentType(tmp); ct {
	case "image/png", "image/jpeg", "image/gif":
		hash := uint64(0)
		for _, v := range tmp {
			hash = hash*31 + uint64(v)
		}
		v, err := writeImageReader(u, part.FileName(), hash, rd, ct, large)
		if err != nil {
			log.Println("image api:", err)
			g.String(500, "server-error")
			return
		}
		g.String(200, v)
	default:
		g.String(500, "invalid-type")
	}

}
