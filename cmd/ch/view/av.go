package view

import (
	"encoding/base64"
	"html/template"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/mv"
)

type ArticleView struct {
	ID          string
	IDDOM       string
	Parent      *ArticleView
	Author      *mv.User
	Cmd         string
	Replies     int
	Locked      bool
	NSFW        bool
	NoAvatar    bool
	Content     string
	ContentHTML template.HTML
	Media       string
	MediaType   string
	CreateTime  time.Time
	// HasPoll     bool
	// Polls       [8]struct {
	// 	Text  string
	// 	Votes int
	// 	Ratio byte // 100%
	// 	Voted bool
	// }
}

const (
	_ uint64 = 1 << iota
	_NoMoreParent
	_RichTime
	_ShowAvatar
)

func (a *ArticleView) from(a2 *mv.Article, opt uint64, u *mv.User) *ArticleView {
	if a2 == nil {
		return a
	}

	a.ID = a2.ID
	a.Replies = a2.Replies
	a.Locked = a2.Locked
	a.NSFW = a2.NSFW
	a.Cmd = string(a2.Cmd)
	a.CreateTime = a2.CreateTime
	a.Author, _ = m.GetUser(a2.Author)
	if a.Author == nil {
		a.Author = &mv.User{
			ID: "?",
		}
	}

	if p := strings.SplitN(a2.Media, ":", 2); len(p) == 2 {
		a.MediaType, a.Media = p[0], p[1]
		switch a.MediaType {
		case "IMG":
			a.Media = "/img/" + base64.StdEncoding.EncodeToString([]byte(a.Media)) + ".jpg"
		}
	}

	if img := mv.ExtractFirstImage(a2.Content); img != "" && a2.Media == "" {
		a.MediaType, a.Media = "IMG", img
	}

	a.Content = a2.Content
	a.ContentHTML = a2.ContentHTML()

	// if a2.Extras["poll"] == "true" {
	// 	for i := range a.Polls {
	// 		a.Polls[i].Text = a2.Extras["poll"+strconv.Itoa(i+1)]
	// 		if a.Polls[i].Text != "" {
	// 			a.HasPoll = true
	// 		}
	// 	}
	// }

	if a2.Parent != "" && opt&_NoMoreParent == 0 {
		p, _ := m.GetArticle(a2.Parent)
		a.Parent = &ArticleView{}
		a.Parent.from(p, opt|_NoMoreParent, u)
	}

	a.NoAvatar = opt&_NoMoreParent > 0
	if opt&_ShowAvatar > 0 {
		a.NoAvatar = false
	}

	switch a2.Cmd {
	case mv.CmdReply, mv.CmdMention:
		p, _ := m.GetArticle(a2.Extras["article_id"])
		a.from(p, opt, u)
		a.Cmd = string(a2.Cmd)
	}

	return a
}

func fromMultiple(a *[]ArticleView, a2 []*mv.Article, opt uint64, u *mv.User) {
	*a = make([]ArticleView, len(a2))
	for i, v := range a2 {
		(*a)[i].from(v, opt, u)
	}
}
