package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/dchest/captcha"
	"github.com/gin-gonic/gin"
)

var bytesPool = sync.Pool{New: func() interface{} { return &bytes.Buffer{} }}

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

	if !g.GetBool("ip-ok") {
		r := g.GetFloat64("ip-ok-remain")
		errorPage(403, "COOLING DOWN, please retry after "+strconv.Itoa(config.Cooldown-int(r))+"s", g)
		return
	}

	var answer [6]byte
	pl.UUID, answer = makeCSRFToken(g)
	pl.Challenge = captchaBase64(answer)

	if id := g.Param("id"); id != "0" {
		pl.Reply = id
		a, err := m.GetArticle(displayIDToObejctID(id))
		if err != nil {
			log.Println(err)
			g.AbortWithStatus(400)
			return
		}
		if a.Title != "" && !strings.HasPrefix(a.Title, "RE:") {
			pl.Abstract = softTrunc(a.Title, 50)
		} else {
			pl.Abstract = softTrunc(a.Content, 50)
		}
	}

	if id, _ := g.Cookie("id"); id != "" && pl.RAuthor == "" {
		pl.RAuthor = id
	}

	g.HTML(200, "newpost.html", pl)
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

func handleNewPostAction(g *gin.Context) {
	var (
		reply    = displayIDToObejctID(g.PostForm("reply"))
		ip       = hashIP(g)
		answer   = softTrunc(g.PostForm("answer"), 6)
		uuid     = softTrunc(g.PostForm("uuid"), 32)
		content  = softTrunc(g.PostForm("content"), int(config.MaxContent))
		title    = softTrunc(g.PostForm("title"), 100)
		author   = softTrunc(g.PostForm("author"), 32)
		tags     = splitTags(softTrunc(g.PostForm("tags"), 128))
		announce = g.PostForm("announce") != ""
		isAPI    = g.PostForm("api") == "1"
		image, _ = g.FormFile("image")
		redir    = func(a, b string) {
			if isAPI {
				g.Status(500)
				g.Header("Error", b)
				return
			}
			q := encodeQuery(a, b, "author", author, "content", content, "title", title, "tags", strings.Join(tags, " "))
			if reply == 0 {
				g.Redirect(302, "/new/0?"+q)
			} else {
				g.Redirect(302, "/new/"+objectIDToDisplayID(reply)+"?"+q)
			}
		}
	)

	if !g.GetBool("ip-ok") {
		redir("error", "guard/cooling-down")
		return
	}

	if g.PostForm("refresh") != "" {
		dedup.Remove(g.ClientIP())
		redir("", "")
		return
	}

	tokenbuf, tokenok := extractCSRFToken(g, uuid)

	if !isAdmin(author) {
		challengePassed := false
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
			log.Println(g.ClientIP(), "challenge failed")
			redir("error", "guard/failed-captcha")
			return
		}
	}

	if !tokenok {
		redir("error", "guard/token-expired")
		return
	}

	if author == "" {
		author = "user/" + g.ClientIP()
	}

	if image == nil && len(content) < int(config.MinContent) {
		redir("error", "content/too-short")
		return
	}

	var imagek string
	if image != nil {
		if config.ImageDisabled && !isAdmin(author) {
			redir("error", "image/disabled")
			return
		}

		f, err := image.Open()
		if err != nil {
			redir("error", "image/open-error: "+err.Error())
			return
		}
		buf, _ := ioutil.ReadAll(io.LimitReader(f, 1024*1024))
		f.Close()

		if !isValidImage(buf) {
			redir("error", "image/invalid-format")
			return
		}

		localpath, displaypath := getImageLocalTmpPath(image.Filename, buf)
		if err := ioutil.WriteFile(localpath, buf, 0700); err != nil {
			redir("error", "image/write-error: "+err.Error())
			return
		}
		imagek = displaypath
	}

	var err error
	if reply != 0 {
		err = m.PostReply(reply, m.NewArticle("", content, authorNameToHash(author), ip, imagek, nil))
	} else {
		if len(title) < int(config.MinContent) {
			redir("error", "title/too-short")
			return
		}
		a := m.NewArticle(title, content, authorNameToHash(author), ip, imagek, tags)
		if isAdmin(author) && announce {
			a.Announce = true
			a.ID = newBigID()
		}
		err = m.PostArticle(a)
		reply = a.ID
	}
	if err != nil {
		redir("error", "internal/error: "+err.Error())
		return
	}

	if isAPI {
		g.Status(http.StatusCreated)
	} else {
		g.Redirect(302, "/p/"+objectIDToDisplayID(reply)+"?p=-1")
	}
}
