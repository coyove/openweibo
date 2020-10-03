package handler

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
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
		content  = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
		image    = common.SanMedia(g.PostForm("media"))
		rlm, _   = strconv.Atoi(g.PostForm("reply_lock"))
		err      error
		pastebin = false
	)

	u := dal.GetUserByContext(g)
	if u == nil {
		u = &model.User{ID: "pastebin" + strconv.Itoa(rand.Intn(10))}
		image, rlm = "", 0
		pastebin = true
		if content == "" {
			// curl -F 'content=@file'
			f, err := g.FormFile("content")
			throw(err, "multipart_error")
			rd, err := f.Open()
			throw(err, "multipart_error")
			defer rd.Close()
			tmp, _ := ioutil.ReadAll(rd)
			content = string(tmp)
		}
	}

	throw(len(content) < 3 && image == "", "content_too_short")
	throw(checkIP(g), "")

	if image != "" {
		image = "IMG:" + image
	}

	a := &model.Article{
		Content:       content,
		Media:         image,
		IP:            hashIP(g),
		NSFW:          g.PostForm("nsfw") != "",
		Anonymous:     g.PostForm("anon") != "",
		ReplyLockMode: byte(rlm),
	}

	a.SetStickOnTop(g.PostForm("stick_on_top") != "")
	a.AID, err = dal.Ctr.Get()
	throw(err, "")

	if pastebin {
		// Pastebin won't go into master timeline
		a.PostOptions |= model.PostOptionNoMasterTimeline
		a.History = fmt.Sprintf("{pastebin_by:%q}", g.ClientIP())
		throw(err2(dal.Post(a, u)), "")
		shortID, _ := ik.StringifyShortId(a.AID)
		g.String(200, shortID)
		return
	}

	if g.PostForm("no_master") == "1" {
		a.PostOptions |= model.PostOptionNoMasterTimeline
	}

	if u.Settings().DoFollowerNeedsAcceptance() {
		a.PostOptions |= model.PostOptionNoSearch
	}

	a2, err := dal.Post(a, u)
	throw(err, "")
	okok(g, url.PathEscape(middleware.RenderTemplateString("row_content.html", NewTopArticleView(a2, u))))
}

func doReply(g *gin.Context) {
	var (
		reply   = g.PostForm("parent")
		content = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
		image   = common.SanMedia(g.PostForm("media"))
		nsfw    = g.PostForm("nsfw") != ""
		rlm, _  = strconv.Atoi(g.PostForm("reply_lock"))
		err     error
	)

	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")

	if image != "" {
		image = "IMG:" + image
	}

	noTimeline := g.PostForm("no_timeline") == "1" || strings.Contains(content, "#ReportThis")

	a := &model.Article{
		Content:       content,
		Media:         image,
		NSFW:          nsfw,
		Author:        u.ID,
		IP:            hashIP(g),
		ReplyLockMode: byte(rlm),
	}
	a.AID, err = dal.Ctr.Get()
	if err != nil {
		log.Println("AID", err)
	}

	if noTimeline || u.Settings().DoFollowerNeedsAcceptance() {
		a.PostOptions |= model.PostOptionNoSearch
	}

	a2, err := dal.PostReply(reply, a, u, noTimeline)
	throw(err, "cannot_reply")
	okok(g, url.PathEscape(middleware.RenderTemplateString("row_content.html", NewReplyArticleView(a2, u))))
}

func APIDeleteArticle(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")
	throw(err2(dal.DoUpdateArticle(&dal.UpdateArticleRequest{ID: g.PostForm("id"), DeleteBy: u})), "")
	okok(g)
}

func APIToggleNSFWArticle(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")
	throw(err2(dal.DoUpdateArticle(&dal.UpdateArticleRequest{ID: g.PostForm("id"), ToggleNSFWBy: u})), "")
	okok(g)
}

func APIToggleLockArticle(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")

	v, _ := strconv.Atoi(g.PostForm("mode"))
	throw(err2(dal.DoUpdateArticle(&dal.UpdateArticleRequest{
		ID:                g.PostForm("id"),
		UpdateReplyLockBy: u,
		UpdateReplyLock:   aws.Uint8(byte(v)),
	})), "")
	okok(g)
}

func APIDropTop(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")
	throw(err2(dal.DoUpdateArticleExtra(&dal.UpdateArticleExtraRequest{
		ID:            ik.NewID(ik.IDAuthor, u.ID).String(),
		SetExtraKey:   "stick_on_top",
		SetExtraValue: "",
	})), "")
	okok(g)
}

func APIUpload(g *gin.Context) {
	const IR = "invalid/request"
	g.Set("error-as-500", true) // failure will be returned using 500 status code

	u := getUser(g)
	throw(u, "")
	throw(!ik.BAdd(u.ID), IR)

	d, params, err := mime.ParseMediaType(g.GetHeader("Content-Type"))
	throw(err, IR)
	throw(!(d == "multipart/form-data" || d == "multipart/mixed"), IR)

	boundary, ok := params["boundary"]
	throw(!ok, IR)

	mprd := multipart.NewReader(g.Request.Body, boundary)
	part, err := mprd.NextPart()
	throw(err, IR)

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
		throw(err, IR)
		g.String(200, v)
	default:
		throw(true, IR)
	}
}
