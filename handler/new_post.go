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
	throw(checkIP(g), "")
	g.Set("allow-api", true)

	var (
		replyTo      = g.PostForm("parent")
		content      = common.SoftTrunc(g.PostForm("content"), int(common.Cfg.MaxContent))
		image        = common.DetectMedia(g.PostForm("media"))
		replyLock, _ = strconv.Atoi(g.PostForm("reply_lock"))
		pastebin     = false
	)

	u := dal.GetUserByContext(g)
	if u == nil {
		throw(g.PostForm("api2_uid") != "", "user_not_found")
		u = &model.User{ID: "pastebin" + strconv.Itoa(rand.Intn(10))}
		image, replyLock, replyTo, pastebin = "", 0, "", true

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

	a := &model.Article{
		Author:        u.ID,
		Content:       content,
		Media:         image,
		IP:            hashIP(g),
		NSFW:          g.PostForm("nsfw") != "",
		Anonymous:     replyTo == "" && g.PostForm("anon") != "", // anonymous replies are meaningless
		ReplyLockMode: byte(replyLock),
		CreateTime:    time.Now(),
		Extras:        map[string]string{},
		T_StickOnTop:  g.PostForm("stick_on_top") != "",
	}
	if u.IsAPI() {
		a.Extras["is_bot"], a.Extras["bot"] = "1", g.PostForm("bot")
	}

	if pastebin {
		// Pastebin won't go into master timeline
		a.PostOptions |= model.PostOptionNoMasterTimeline
		a.History = fmt.Sprintf("{pastebin_by:%q}", g.ClientIP())
		throw(common.Err2(dal.Post(a, u)), "")
		g.String(200, a.ID)
		return
	}

	if u.FollowApply != 0 {
		// If I want to control & filter my followers, then I would definitly not want my feeds found by non-followers
		a.PostOptions |= model.PostOptionNoSearch
	}

	if replyTo == "" {
		if g.PostForm("poll") == "1" {
			handlePollContent(a)
		}

		if g.PostForm("no_master") == "1" {
			a.PostOptions |= model.PostOptionNoMasterTimeline
		}

		a2, err := dal.Post(a, u)
		throw(err, "")
		okok(g, url.PathEscape(middleware.RenderTemplateString("row_content.html", NewTopArticleView(a2, u))))
	} else {
		if g.PostForm("no_timeline") == "1" || strings.Contains(content, "#ReportThis") {
			a.PostOptions |= model.PostOptionNoSearch
			a.PostOptions |= model.PostOptionNoTimeline
		}

		a2, err := dal.PostReply(replyTo, a, u)
		throw(err, "cannot_reply")
		okok(g, url.PathEscape(middleware.RenderTemplateString("row_content.html", NewReplyArticleView(a2, u))))
	}
}

func APIDeleteArticle(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")
	throw(common.Err2(dal.DoUpdateArticle(g.PostForm("id"), func(a *model.Article) error {
		if u.ID != a.Author && !u.IsMod() {
			return fmt.Errorf("e:user_not_permitted")
		}
		a.Content = model.DeletionMarker
		a.Media = ""
		a.History += fmt.Sprintf("{delete_by:%s:%v}", u.ID, time.Now().Unix())
		if a.Parent != "" {
			go dal.DoUpdateArticle(a.Parent, func(a *model.Article) { a.Replies-- })
		}
		return nil
	})), "")
	okok(g)
}

func APIToggleNSFWArticle(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")
	throw(common.Err2(dal.DoUpdateArticle(g.PostForm("id"), func(a *model.Article) error {
		if u.ID != a.Author && !u.IsMod() {
			return fmt.Errorf("e:user_not_permitted")
		}
		a.NSFW = !a.NSFW
		a.History += fmt.Sprintf("{nsfw_by:%s:%v}", u.ID, time.Now().Unix())
		return nil
	})), "")
	okok(g)
}

func APIToggleLockArticle(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")

	v, _ := strconv.Atoi(g.PostForm("mode"))
	throw(common.Err2(dal.DoUpdateArticle(g.PostForm("id"), func(a *model.Article) error {
		if u.ID != a.Author && !u.IsMod() {
			return fmt.Errorf("e:user_not_permitted")
		}
		a.ReplyLockMode = byte(v)
		a.History += fmt.Sprintf("{lock_by:%s:%v}", u.ID, time.Now().Unix())
		return nil
	})), "")
	okok(g)
}

func APIDropTop(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")
	throw(common.Err2(dal.DoUpdateArticle(ik.NewID(ik.IDAuthor, u.ID).String(), func(a *model.Article) {
		a.Extras["stick_on_top"] = ""
	})), "")
	okok(g)
}

func APIPoll(g *gin.Context) {
	u := throw(dal.GetUserByContext(g), "").(*model.User)
	throw(checkIP(g), "")
	a, err := dal.GetArticle(g.PostForm("id"))
	throw(err, "")
	throw(a.Extras["poll_title"] == "", "")

	pollID := "u/" + u.ID + "/poll/" + a.ID
	uc, err := dal.GetArticle(pollID)
	throw(err != nil && err != model.ErrNotExisted, "")

	if uc != nil && uc.Extras["choice"] != "" {
		// Clear old poll votes
		throw(a.Extras["poll_nochange"] != "", "poll_nochange")
		throw(common.Err2(dal.DoUpdateArticle(pollID, func(a *model.Article) {
			a.Extras["choice"] = ""
		})), "")

		oldChoices := strings.Split(uc.Extras["choice"], ",")
		throw(common.Err2(dal.DoUpdateArticle(a.ID, func(a *model.Article) {
			for _, key := range append(oldChoices, "poll_total") {
				v, _ := strconv.ParseInt(a.Extras[key], 10, 64)
				if v--; v < 0 {
					v = 0
				}
				a.Extras[key] = strconv.FormatInt(v, 10)
			}
		})), "")

		okok(g, common.RevRenderTemplateString("poll.html", []interface{}{a.ID, a.Extras}))
		return
	}

	// Cast new poll votes
	options, _ := strconv.Atoi(a.Extras["poll_options"])
	maxChoices, _ := strconv.Atoi(a.Extras["poll_max"])
	ttl := common.ParseDuration(a.Extras["poll_live"])
	newChoices := strings.Split(g.PostForm("choice"), ",")
	throw(len(newChoices) <= 0, "")
	throw(a.Extras["poll_multiple"] == "" && len(newChoices) > 1, "too_many_choices")
	throw(maxChoices != 0 && len(newChoices) > maxChoices, "too_many_choices")
	throw(ttl > 0 && a.CreateTime.Add(ttl).Before(time.Now()), "poll_closed")

	for _, c := range newChoices {
		if !strings.HasPrefix(c, "poll_") {
			throw(true, "")
		}
		c, _ := strconv.Atoi(c[5:])
		if c < 1 || c > options {
			log.Println(c, options, a.Extras)
			throw(true, "")
		}
	}

	throw(dal.ModKV().Set(pollID, (&model.Article{
		ID:     pollID,
		Extras: map[string]string{"choice": strings.Join(newChoices, ",")},
	}).Marshal()), "")
	throw(common.Err2(dal.DoUpdateArticle(a.ID, func(a *model.Article) {
		for _, key := range append(newChoices, "poll_total") {
			v, _ := strconv.ParseInt(a.Extras[key], 10, 64)
			a.Extras[key] = strconv.FormatInt(v+1, 10)
		}
	})), "")

	{
		a2, _ := dal.GetArticle(a.ID)
		if a2 == nil {
			a2 = a // fallback
		}
		a2.Extras["poll_you_voted"] = strings.Join(newChoices, ",")
		okok(g, common.RevRenderTemplateString("poll.html", []interface{}{a2.ID, a2.Extras}))
	}
}

func APIUpload(g *gin.Context) {
	const IR = "ERROR"
	g.Set("error-as-500", true) // failure will be returned using 500 status code

	u := getUser(g)
	if u == nil {
		u, _ = dal.GetUserByToken(g.Query("api2_uid"), true)
	}

	throw(u, "INVALID_USER")
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
