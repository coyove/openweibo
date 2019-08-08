package main

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

func handleNewPostView(g *gin.Context) {
	var pl = struct {
		UUID      string
		Reply     string
		Abstract  string
		Challenge string

		RTitle, RContent, RTags, EError string
	}{
		UUID:      makeCSRFToken(g),
		Challenge: makeChallengeToken(),
		RTitle:    g.Query("title"),
		RContent:  g.Query("content"),
		RTags:     g.Query("tags"),
		EError:    g.Query("error"),
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

	g.HTML(200, "newpost.html", pl)
}

func handleNewPostAction(g *gin.Context) {
	var (
		reply     = displayIDToObejctID(g.PostForm("reply"))
		answer    = g.PostForm("answer")
		challenge = g.PostForm("challenge")
		uuid      = g.PostForm("uuid")
		content   = softTrunc(g.PostForm("content"), int(config.MaxContent))
		title     = softTrunc(g.PostForm("title"), 64)
		author    = softTrunc(g.PostForm("author"), 32)
		tags      = splitTags(softTrunc(g.PostForm("tags"), 128))
		redir     = func(a, b string) {
			q := encodeQuery(a, b, "content", content, "title", title, "tags", strings.Join(tags, " "))
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
		author = g.ClientIP() + config.Key
	}

	if len(content) < int(config.MinContent) {
		redir("error", "content too short")
		return
	}

	var err error
	if reply != "" {
		err = m.PostReply(reply, NewArticle("", content, authorNameToHash(author), nil, nil))
	} else {
		if len(title) < int(config.MinContent) {
			redir("error", "title too short")
			return
		}
		err = m.PostArticle(NewArticle(title, content, authorNameToHash(author), nil, tags))
	}
	if err != nil {
		redir("error", err.Error())
		return
	}
	if reply != "" {
		g.Redirect(302, "/p/by_create/parent:"+objectIDToDisplayID(reply)+"?p=-1")
	} else {
		g.Redirect(302, "/p/by_reply/index")
	}
}
