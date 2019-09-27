package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/dchest/captcha"
	"github.com/gin-gonic/gin"
)

var bytesPool = sync.Pool{New: func() interface{} { return &bytes.Buffer{} }}

func getAuthor(g *gin.Context) string {
	a := softTrunc(g.PostForm("author"), 32)
	if a == "" {
		a = "user/" + g.MustGet("ip").(net.IP).String()
	}
	return a
}

func captchaBase64(buf [6]byte) string {
	b := bytesPool.Get().(*bytes.Buffer)
	captcha.NewImage(config.Key, buf[:6], 180, 60).WriteTo(b)
	ret := base64.StdEncoding.EncodeToString(b.Bytes())
	b.Reset()
	bytesPool.Put(b)
	return ret
}

func handleNewPostView(g *gin.Context) {
	var pl = struct {
		UUID      string
		Reply     string
		Abstract  string
		Challenge string
		Tags      []string

		RTitle, RAuthor, RContent, RTags, EError string
	}{
		RTitle:   g.Query("title"),
		RContent: g.Query("content"),
		RTags:    g.Query("tags"),
		RAuthor:  g.Query("author"),
		EError:   g.Query("error"),
		Tags:     config.Tags,
	}

	var answer [6]byte
	pl.UUID, answer = makeCSRFToken(g)
	pl.Challenge = captchaBase64(answer)

	if pl.RAuthor == "" {
		pl.RAuthor, _ = g.Cookie("id")
	}

	g.HTML(200, "newpost.html", pl)
}

func generateNewReplyView(id int64, g *gin.Context) interface{} {
	var pl = struct {
		UUID      string
		Challenge string
		ShowReply bool
		RAuthor   string
		RContent  string
		EError    string
	}{}

	pl.RContent = g.Query("content")
	pl.RAuthor = g.Query("author")
	pl.EError = g.Query("error")
	pl.ShowReply = g.Query("refresh") == "1" || pl.EError != ""

	if pl.RAuthor == "" {
		pl.RAuthor, _ = g.Cookie("id")
	}

	var answer [6]byte
	pl.UUID, answer = makeCSRFToken(g)
	pl.Challenge = captchaBase64(answer)
	return pl
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
		answer            = softTrunc(g.PostForm("answer"), 6)
		uuid              = softTrunc(g.PostForm("uuid"), 32)
		tokenbuf, tokenok = extractCSRFToken(g, uuid)
		challengePassed   bool
	)
	if !g.GetBool("ip-ok") {
		return fmt.Sprintf("guard/cooling-down/%v", g.GetFloat64("ip-ok-remain"))
	}
	if !isAdmin(author) {
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

func handleNewPostAction(g *gin.Context) {
	var (
		ip       = hashIP(g)
		content  = softTrunc(g.PostForm("content"), int(config.MaxContent))
		title    = softTrunc(g.PostForm("title"), 100)
		author   = getAuthor(g)
		tags     = splitTags(softTrunc(g.PostForm("tags"), 128))
		announce = g.PostForm("announce") != ""
		redir    = func(a, b string) {
			q := encodeQuery(a, b, "author", author, "content", content, "title", title, "tags", strings.Join(tags, " "))
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

	if len(content) < int(config.MinContent) {
		redir("error", "content/too-short")
		return
	}

	if len(title) < int(config.MinContent) {
		redir("error", "title/too-short")
		return
	}

	a := m.NewPost(title, content, authorNameToHash(author), ip, tags)
	if isAdmin(author) && announce {
		a.Announce = true
		a.ID = newBigID()
	}

	if err := m.PostPost(a); err != nil {
		redir("error", "internal/error: "+err.Error())
		return
	}

	g.Redirect(302, "/p/"+a.DisplayID())
}

func handleNewReplyAction(g *gin.Context) {
	var (
		reply   = displayIDToObejctID(g.PostForm("reply"))
		ip      = hashIP(g)
		content = softTrunc(g.PostForm("content"), int(config.MaxContent))
		author  = getAuthor(g)
		redir   = func(a, b string) {
			g.Redirect(302, "/p/"+objectIDToDisplayID(reply)+"?p=-1&"+encodeQuery(a, b, "author", author, "content", content)+"#paging")
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

	if len(content) < int(config.MinContent) {
		redir("error", "content/too-short")
		return
	}

	if err := m.PostReply(reply, m.NewReply(content, authorNameToHash(author), ip)); err != nil {
		log.Println(err)
		redir("error", "internal/error")
		return
	}

	content, author = "", ""
	redir("", "")
}
