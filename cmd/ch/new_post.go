package main

import (
	"encoding/binary"
	"log"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

func handleNewPostView(g *gin.Context) {
	var pl = struct {
		UUID      string
		Reply     string
		Abstract  string
		Challenge string
		Tags      []string

		RTitle, RAuthor, RContent, RTags, EError string
	}{
		UUID:      makeCSRFToken(g),
		Challenge: makeChallengeToken(),
		RTitle:    g.Query("title"),
		RContent:  g.Query("content"),
		RTags:     g.Query("tags"),
		RAuthor:   g.Query("author"),
		EError:    g.Query("error"),
		Tags:      config.Tags,
	}

	if id := g.Param("id"); id != "0" {
		pl.Reply = id
		a, err := m.GetArticle(displayIDToObejctID(id))
		if err != nil {
			log.Println(err)
			g.AbortWithStatus(400)
			return
		}
		if a.Title != "" {
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

func hashIP(g *gin.Context) uint64 {
	buf := make([]byte, 8)
	ip := g.ClientIP()
	ip2 := net.ParseIP(ip)
	if len(ip2) == net.IPv4len {
		copy(buf, ip2[:3]) // first 3 bytes
	} else if len(ip2) == net.IPv6len {
		copy(buf, ip2) // first 8 byte
	} else {
		copy(buf, ip)
	}
	return binary.BigEndian.Uint64(buf)
}

func handleNewPostAction(g *gin.Context) {
	var (
		reply     = displayIDToObejctID(g.PostForm("reply"))
		answer    = g.PostForm("answer")
		challenge = g.PostForm("challenge")
		uuid      = g.PostForm("uuid")
		ip        = hashIP(g)
		content   = softTrunc(g.PostForm("content"), int(config.MaxContent))
		title     = softTrunc(g.PostForm("title"), 64)
		author    = softTrunc(g.PostForm("author"), 32)
		tags      = splitTags(softTrunc(g.PostForm("tags"), 128))
		redir     = func(a, b string) {
			q := encodeQuery(a, b, "author", author, "content", content, "title", title, "tags", strings.Join(tags, " "))
			if reply == "" {
				g.Redirect(302, "/new/0?"+q)
			} else {
				g.Redirect(302, "/new/"+objectIDToDisplayID(reply)+"?"+q)
			}
		}
	)

	if !g.GetBool("ip-ok") {
		redir("error", "cooling down")
		return
	}

	if g.PostForm("refresh") != "" {
		redir("", "")
		return
	}

	if !isCSRFTokenValid(g, uuid) {
		redir("", "")
		return
	}

	if !isChallengeTokenValid(challenge, answer) && author != config.AdminName {
		log.Println(g.ClientIP(), "challenge failed")
		redir("error", "wrong captcha")
		return
	}

	if author == "" {
		author = g.ClientIP()
	}

	if len(content) < int(config.MinContent) {
		redir("error", "content too short")
		return
	}

	var err error
	if reply != "" {
		err = m.PostReply(reply, NewArticle("", content, authorNameToHash(author), nil, nil, ip))
	} else {
		if len(title) < int(config.MinContent) {
			redir("error", "title too short")
			return
		}
		a := NewArticle(title, content, authorNameToHash(author), nil, tags, ip)
		err = m.PostArticle(a)
		reply = a.ID
	}
	if err != nil {
		redir("error", err.Error())
		return
	}

	g.Redirect(302, "/p/"+objectIDToDisplayID(reply)+"?p=-1")
}

func sanText(in string) string {
	firstImg := false
	return rxSan.ReplaceAllStringFunc(in, func(in string) string {
		if in == "<" {
			return "&lt;"
		}
		if strings.HasSuffix(in, ".jpg") || strings.HasSuffix(in, ".gif") || strings.HasSuffix(in, ".png") {
			if !firstImg {
				firstImg = true
				return "<a href='" + in + "' target=_blank><img src='" + in + "' class='image'></a>"
			}
		}
		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
}
