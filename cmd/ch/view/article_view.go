package view

import (
	"html/template"
	"time"

	"github.com/coyove/iis/cmd/ch/mv"
)

type ArticleView struct {
	ID          string
	Timeline    string
	Parent      *ArticleView
	Image       string
	Cmd         string
	Extras      map[string]string
	Index       int
	SubIndex    string
	Replies     int
	Forwards    int
	Upvotes     int
	Locked      bool
	NoAvatar    bool
	Title       string
	Content     string
	ContentHTML template.HTML
	Author      string
	IP          string
	CreateTime  time.Time
}

const (
	_ uint64 = 1 << iota
	_Abstract
	_NoMoreParent
	_RichTime
	_ShowAvatar
)

func (a *ArticleView) from(a2 *mv.Article, opt uint64) *ArticleView {
	if a2 == nil {
		return a
	}

	a.ID = a2.ID
	a.Replies = a2.Replies
	a.Locked = a2.Locked
	a.Cmd = string(a2.Cmd)
	a.Extras = a2.Extras
	a.Author = a2.Author
	a.IP = a2.IP
	a.CreateTime = a2.CreateTime

	if opt&_Abstract > 0 {
		a.Content = mv.SoftTrunc(a2.Content, 64)
		a.ContentHTML = template.HTML(template.HTMLEscapeString(a.Content))
	} else {
		a.Content = a2.Content
		a.ContentHTML = a2.ContentHTML()
	}

	if a2.Parent != "" && opt&_NoMoreParent == 0 {
		p, _ := m.GetArticle(a2.Parent)
		a.Parent = &ArticleView{}
		a.Parent.from(p, opt|_NoMoreParent)
	}

	a.NoAvatar = opt&_NoMoreParent > 0
	if opt&_ShowAvatar > 0 {
		a.NoAvatar = false
	}

	switch a2.Cmd {
	case mv.CmdReply, mv.CmdMention:
		p, _ := m.GetArticle(a2.Extras["article_id"])
		a.from(p, opt)
		a.Cmd = string(a2.Cmd)
	}

	return a
}

func fromMultiple(a *[]ArticleView, a2 []*mv.Article, opt uint64) {
	*a = make([]ArticleView, len(a2))
	for i, v := range a2 {
		(*a)[i].from(v, opt)
	}
}
